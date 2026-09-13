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

func TestLinearIssueQuerier_IssuesInState(t *testing.T) {
	var authHeader, reqBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		reqBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"issues":{"nodes":[
			{"id":"issue-1","delegate":{"name":"FlintReviewer@justin"}},
			{"id":"issue-2","delegate":null}
		]}}}`))
	}))
	defer srv.Close()

	q := LinearIssueQuerier{APIURL: srv.URL, Token: "tok-xyz"}
	issues, err := q.IssuesInState(context.Background(), "team-1", "state-review")
	if err != nil {
		t.Fatalf("IssuesInState: %v", err)
	}

	if authHeader != "Bearer tok-xyz" {
		t.Errorf("Authorization header = %q, want Bearer tok-xyz", authHeader)
	}

	var req struct {
		Query     string            `json:"query"`
		Variables map[string]string `json:"variables"`
	}
	if err := json.Unmarshal([]byte(reqBody), &req); err != nil {
		t.Fatalf("request body isn't valid JSON: %v", err)
	}
	if !strings.Contains(req.Query, "issues") {
		t.Errorf("query = %q, missing an issues query", req.Query)
	}
	if req.Variables["teamId"] != "team-1" || req.Variables["stateId"] != "state-review" {
		t.Errorf("variables = %+v, want teamId=team-1 stateId=state-review", req.Variables)
	}

	want := []PolledIssue{
		{ID: "issue-1", DelegateName: "FlintReviewer@justin"},
		{ID: "issue-2", DelegateName: ""},
	}
	if len(issues) != len(want) {
		t.Fatalf("issues = %+v, want %+v", issues, want)
	}
	for i := range want {
		if issues[i] != want[i] {
			t.Errorf("issues[%d] = %+v, want %+v", i, issues[i], want[i])
		}
	}
}

func TestLinearIssueQuerier_GraphQLError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"Argument Validation Error"}]}`))
	}))
	defer srv.Close()

	q := LinearIssueQuerier{APIURL: srv.URL, Token: "tok-xyz"}
	_, err := q.IssuesInState(context.Background(), "team-1", "state-review")
	if err == nil {
		t.Fatal("expected an error when the graphql response carries errors")
	}
	if !strings.Contains(err.Error(), "Argument Validation Error") {
		t.Errorf("error = %q, want it to include the graphql error message", err)
	}
}

func TestLinearIssueQuerier_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer srv.Close()

	q := LinearIssueQuerier{APIURL: srv.URL, Token: "tok-bad"}
	_, err := q.IssuesInState(context.Background(), "team-1", "state-review")
	if err == nil {
		t.Fatal("expected an error on a non-200 response")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %q, want it to mention the 401 status", err)
	}
}
