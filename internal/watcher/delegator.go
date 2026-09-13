package watcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

// delegateTimeout bounds both outbound calls a delegation makes (minting
// a token, then the mutation) so a slow Linear response can't itself
// blow the watcher's webhook ack SLA (see docs/ARCHITECTURE.md "The watcher").
const delegateTimeout = 5 * time.Second

const (
	defaultTokenURL = "https://api.linear.app/oauth/token"
	defaultAPIURL   = "https://api.linear.app/graphql"
	// defaultScope matches the pair validated live in docs/ARCHITECTURE.md's
	// "Automating per-agent app creation" (app:mentionable + app:assignable minted
	// via client_credentials with zero browser interaction). Linear's
	// docs specify this field as a comma-separated list, not
	// space-separated (checked directly, 2026-09-10).
	defaultScope = "app:mentionable,app:assignable"
)

// Delegator hands an issue off to a specialist. Kept as an interface so
// gate routing can be tested without making real HTTP calls — see
// LinearDelegator for the concrete implementation.
type Delegator interface {
	Delegate(ctx context.Context, issueID string, specialist Specialist) error
}

// LinearDelegator delegates by minting a client_credentials token for
// the specialist's own OAuth Application — validated live 2026-09-10,
// see docs/ARCHITECTURE.md "Automating per-agent app creation": no browser step,
// no actor= parameter needed at mint time, since an app registered with
// the client_credentials grant enabled always mints an app-actor token
// (there's no user in that grant's flow to be the actor instead). The
// resulting token then calls agentSessionCreateOnIssue as that
// specialist, which is what actually starts its Agent Session on the
// issue — see docs/ARCHITECTURE.md's "Fabro" section for why the review logic
// itself is deliberately not pennon's job.
type LinearDelegator struct {
	// TokenURL and APIURL default to Linear's real endpoints when empty
	// — overridable so tests can point at an httptest.Server instead.
	TokenURL string
	APIURL   string
	// Scope defaults to defaultScope when empty.
	Scope      string
	HTTPClient *http.Client
	Log        *slog.Logger
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
}

type graphQLError struct {
	Message string `json:"message"`
}

type graphQLResponse struct {
	Errors []graphQLError  `json:"errors"`
	Data   json.RawMessage `json:"data"`
}

// Viewer is the identity a token resolves to on Linear's own side —
// exactly what VerifyToken exists to check before that token gets wired
// into anything.
type Viewer struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Delegate implements Delegator.
func (d LinearDelegator) Delegate(ctx context.Context, issueID string, specialist Specialist) error {
	ctx, cancel := context.WithTimeout(ctx, delegateTimeout)
	defer cancel()

	token, err := d.MintToken(ctx, specialist)
	if err != nil {
		return fmt.Errorf("watcher: mint token for %s: %w", specialist.Name, err)
	}

	if err := d.createSession(ctx, token, issueID); err != nil {
		return fmt.Errorf("watcher: agentSessionCreateOnIssue for %s on %s: %w", specialist.Name, issueID, err)
	}

	d.Log.Info("delegated issue to specialist", "specialist", specialist.Name, "issue_id", issueID)
	return nil
}

// MintToken mints a client_credentials app-actor token for the given
// identity — the same call Delegate makes internally, exported so other
// callers (see cmd/pennon's "mint" subcommand) can get a bare token for
// an identity without going through the gate/delegation path at all.
// Nothing about this token ties it to a review or a gate; it's just that
// identity's own credential, good for whatever it posts to Linear as
// itself.
func (d LinearDelegator) MintToken(ctx context.Context, specialist Specialist) (string, error) {
	tokenURL := d.TokenURL
	if tokenURL == "" {
		tokenURL = defaultTokenURL
	}
	scope := d.Scope
	if scope == "" {
		scope = defaultScope
	}

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"scope":         {scope},
		"client_id":     {specialist.ClientID},
		"client_secret": {specialist.ClientSecret},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := d.client().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, body)
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("token response carried no access_token: %s", body)
	}
	return tr.AccessToken, nil
}

// VerifyToken confirms a token actually works before it gets wired into
// anything — the mechanical form of the manual `curl .../graphql -H
// "Authorization: Bearer $TOKEN"` check that caught pennon mint's own
// narrow-scope bug live (see docs/ARCHITECTURE.md "Agent onboarding"). A minted
// token that can't answer a plain `viewer` query is a token nothing
// should be pointed at yet, so callers are meant to abort before
// touching any config on a non-nil error here.
func (d LinearDelegator) VerifyToken(ctx context.Context, token string) (Viewer, error) {
	apiURL := d.APIURL
	if apiURL == "" {
		apiURL = defaultAPIURL
	}

	const query = `{ viewer { id name email } }`
	payload, err := json.Marshal(map[string]any{"query": query})
	if err != nil {
		return Viewer{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return Viewer{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := d.client().Do(req)
	if err != nil {
		return Viewer{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Viewer{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Viewer{}, fmt.Errorf("graphql endpoint returned %d: %s", resp.StatusCode, body)
	}

	var gr graphQLResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		return Viewer{}, fmt.Errorf("decode graphql response: %w", err)
	}
	if len(gr.Errors) > 0 {
		return Viewer{}, fmt.Errorf("graphql error: %s", gr.Errors[0].Message)
	}

	var data struct {
		Viewer Viewer `json:"viewer"`
	}
	if err := json.Unmarshal(gr.Data, &data); err != nil {
		return Viewer{}, fmt.Errorf("decode viewer from graphql response: %w", err)
	}
	return data.Viewer, nil
}

func (d LinearDelegator) createSession(ctx context.Context, token, issueID string) error {
	apiURL := d.APIURL
	if apiURL == "" {
		apiURL = defaultAPIURL
	}

	const query = `mutation($issueId: String!) { agentSessionCreateOnIssue(input: {issueId: $issueId}) { success } }`
	payload, err := json.Marshal(map[string]any{
		"query":     query,
		"variables": map[string]string{"issueId": issueID},
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := d.client().Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("graphql endpoint returned %d: %s", resp.StatusCode, body)
	}

	var gr graphQLResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		return fmt.Errorf("decode graphql response: %w", err)
	}
	if len(gr.Errors) > 0 {
		return fmt.Errorf("graphql error: %s", gr.Errors[0].Message)
	}
	return nil
}

func (d LinearDelegator) client() *http.Client {
	if d.HTTPClient != nil {
		return d.HTTPClient
	}
	return http.DefaultClient
}
