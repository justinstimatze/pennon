package watcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// issuesInStateQuery follows Linear's published IssueFilter shape —
// team/state filtered by { id: { eq: $id } }, the same nested eq-filter
// convention Linear's schema uses throughout. Verified live 2026-09-13
// against a real workspace: the filter genuinely narrows to matching
// issues (checked every returned node's own team/state id against the
// query, not just a non-error response), and `delegate` decodes as nil
// when unset rather than erroring.
const issuesInStateQuery = `query($teamId: ID!, $stateId: ID!) {
  issues(filter: { team: { id: { eq: $teamId } }, state: { id: { eq: $stateId } } }) {
    nodes {
      id
      delegate { name }
    }
  }
}`

// LinearIssueQuerier implements IssueQuerier against Linear's real
// GraphQL API using one fixed bearer token — deliberately not a
// per-specialist client_credentials mint the way LinearDelegator's
// Delegate call is. Listing every issue in a team's state is a read
// across the whole team, not an action taken as any one identity, so it
// runs as whatever control-plane token the deployment configures for
// polling (see cmd/pennon's "gate-poll" subcommand).
type LinearIssueQuerier struct {
	// APIURL defaults to Linear's real endpoint when empty.
	APIURL     string
	Token      string
	HTTPClient *http.Client
}

type issuesInStateResponse struct {
	Issues struct {
		Nodes []struct {
			ID       string `json:"id"`
			Delegate *struct {
				Name string `json:"name"`
			} `json:"delegate"`
		} `json:"nodes"`
	} `json:"issues"`
}

// IssuesInState implements IssueQuerier.
func (q LinearIssueQuerier) IssuesInState(ctx context.Context, teamID, stateID string) ([]PolledIssue, error) {
	apiURL := q.APIURL
	if apiURL == "" {
		apiURL = defaultAPIURL
	}

	payload, err := json.Marshal(map[string]any{
		"query":     issuesInStateQuery,
		"variables": map[string]string{"teamId": teamID, "stateId": stateID},
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+q.Token)

	client := q.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("graphql endpoint returned %d: %s", resp.StatusCode, body)
	}

	var gr graphQLResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		return nil, fmt.Errorf("decode graphql response: %w", err)
	}
	if len(gr.Errors) > 0 {
		return nil, fmt.Errorf("graphql error: %s", gr.Errors[0].Message)
	}

	var data issuesInStateResponse
	if err := json.Unmarshal(gr.Data, &data); err != nil {
		return nil, fmt.Errorf("decode issues from graphql response: %w", err)
	}

	issues := make([]PolledIssue, 0, len(data.Issues.Nodes))
	for _, n := range data.Issues.Nodes {
		pi := PolledIssue{ID: n.ID}
		if n.Delegate != nil {
			pi.DelegateName = n.Delegate.Name
		}
		issues = append(issues, pi)
	}
	return issues, nil
}
