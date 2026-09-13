package watcher

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLinearDelegator_Delegate(t *testing.T) {
	var (
		tokenCalled  bool
		mutationBody string
		authHeader   string
	)

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalled = true
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse token request form: %v", err)
		}
		if got := r.FormValue("grant_type"); got != "client_credentials" {
			t.Errorf("grant_type = %q, want client_credentials", got)
		}
		if got := r.FormValue("client_id"); got != "client-1" {
			t.Errorf("client_id = %q, want client-1", got)
		}
		if got := r.FormValue("client_secret"); got != "secret-1" {
			t.Errorf("client_secret = %q, want secret-1", got)
		}
		if got := r.FormValue("scope"); got != "app:mentionable,app:assignable" {
			t.Errorf("scope = %q, want app:mentionable,app:assignable", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-abc","token_type":"Bearer","expires_in":2591999,"scope":"app:mentionable,app:assignable"}`))
	}))
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read mutation body: %v", err)
		}
		mutationBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"agentSessionCreateOnIssue":{"success":true}}}`))
	}))
	defer apiSrv.Close()

	d := LinearDelegator{
		TokenURL: tokenSrv.URL,
		APIURL:   apiSrv.URL,
		Log:      discardLogger(),
	}

	err := d.Delegate(context.Background(), "issue-1", Specialist{
		Name:         "Code Reviewer Flint",
		ClientID:     "client-1",
		ClientSecret: "secret-1",
	})
	if err != nil {
		t.Fatalf("Delegate: %v", err)
	}
	if !tokenCalled {
		t.Fatal("token endpoint was never called")
	}
	if authHeader != "Bearer tok-abc" {
		t.Errorf("Authorization header = %q, want Bearer tok-abc", authHeader)
	}

	var mutation struct {
		Query     string            `json:"query"`
		Variables map[string]string `json:"variables"`
	}
	if err := json.Unmarshal([]byte(mutationBody), &mutation); err != nil {
		t.Fatalf("mutation body isn't valid JSON: %v", err)
	}
	if !strings.Contains(mutation.Query, "agentSessionCreateOnIssue") {
		t.Errorf("mutation query = %q, missing agentSessionCreateOnIssue", mutation.Query)
	}
	if mutation.Variables["issueId"] != "issue-1" {
		t.Errorf("mutation variables[issueId] = %q, want issue-1", mutation.Variables["issueId"])
	}
}

func TestLinearDelegator_Delegate_TokenEndpointFails(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"Error","error_description":"Client does not support the client_credentials grant type"}`))
	}))
	defer tokenSrv.Close()

	d := LinearDelegator{TokenURL: tokenSrv.URL, Log: discardLogger()}
	err := d.Delegate(context.Background(), "issue-1", Specialist{ClientID: "c", ClientSecret: "s"})
	if err == nil {
		t.Fatal("expected an error when the token endpoint rejects the request")
	}
}

func TestLinearDelegator_Delegate_MutationFails(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-abc"}`))
	}))
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"Issue not found"}]}`))
	}))
	defer apiSrv.Close()

	d := LinearDelegator{TokenURL: tokenSrv.URL, APIURL: apiSrv.URL, Log: discardLogger()}
	err := d.Delegate(context.Background(), "issue-missing", Specialist{ClientID: "c", ClientSecret: "s"})
	if err == nil {
		t.Fatal("expected an error when the mutation returns a graphql error")
	}
	if !strings.Contains(err.Error(), "Issue not found") {
		t.Errorf("error = %q, want it to include the graphql error message", err)
	}
}

func TestLinearDelegator_VerifyToken(t *testing.T) {
	var authHeader, queryBody string

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		queryBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"viewer":{"id":"user-1","name":"mercury@justin","email":"mercury@justin.invalid"}}}`))
	}))
	defer apiSrv.Close()

	d := LinearDelegator{APIURL: apiSrv.URL, Log: discardLogger()}
	viewer, err := d.VerifyToken(context.Background(), "tok-xyz")
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if viewer.Name != "mercury@justin" || viewer.ID != "user-1" {
		t.Errorf("viewer = %+v, want name mercury@justin, id user-1", viewer)
	}
	if authHeader != "Bearer tok-xyz" {
		t.Errorf("Authorization header = %q, want Bearer tok-xyz", authHeader)
	}
	if !strings.Contains(queryBody, "viewer") {
		t.Errorf("request body = %q, missing a viewer query", queryBody)
	}
}

func TestLinearDelegator_VerifyToken_ScopeError(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":[{"message":"Invalid scope: read required"}]}`))
	}))
	defer apiSrv.Close()

	d := LinearDelegator{APIURL: apiSrv.URL, Log: discardLogger()}
	_, err := d.VerifyToken(context.Background(), "tok-narrow")
	if err == nil {
		t.Fatal("expected an error when the token lacks the scope to answer a viewer query")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error = %q, want it to mention the 403 status", err)
	}
}
