package gateway

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchOrgIDFrom(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		fmt.Fprint(w, `{"providers":{"opencode":{"headers":{"x-opencode-org-id":"org_01TEST"}}}}`)
	}))
	defer server.Close()

	org, err := fetchOrgIDFrom(context.Background(), server.Client(), server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	if org != "org_01TEST" {
		t.Fatalf("org = %q", org)
	}
}

func TestFetchOrgIDFromMissingHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"providers":{"opencode":{"headers":{}}}}`)
	}))
	defer server.Close()

	if _, err := fetchOrgIDFrom(context.Background(), server.Client(), server.URL, "test-key"); err == nil {
		t.Fatal("expected an error when the console reports no workspace")
	}
}

func TestSetOrgHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://example.invalid", nil)
	setOrgHeader(req, "https://opencode.ai/inference", "org_01TEST")
	if got := req.Header.Get("x-opencode-org-id"); got != "org_01TEST" {
		t.Fatalf("v2 header = %q", got)
	}

	legacy := httptest.NewRequest(http.MethodPost, "http://example.invalid", nil)
	setOrgHeader(legacy, "https://opencode.ai/zen", "org_01TEST")
	if got := legacy.Header.Get("x-opencode-org-id"); got != "" {
		t.Fatalf("v1 header = %q", got)
	}

	empty := httptest.NewRequest(http.MethodPost, "http://example.invalid", nil)
	setOrgHeader(empty, "https://opencode.ai/inference", "")
	if got := empty.Header.Get("x-opencode-org-id"); got != "" {
		t.Fatalf("empty org header = %q", got)
	}
}
