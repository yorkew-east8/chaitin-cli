package apisec

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestClientSetsAPITokenHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("API-TOKEN"); got != "token-1" {
			t.Fatalf("API-TOKEN = %q, want token-1", got)
		}
		if got := r.URL.Query().Get("page"); got != "2" {
			t.Fatalf("page query = %q, want 2", got)
		}
		_, _ = w.Write([]byte(`{"err":null,"data":{"ok":true},"msg":null}`))
	}))
	defer server.Close()

	client := NewClient(&Config{URL: server.URL, APIToken: "token-1"}, false)
	var result any
	err := client.Do(context.Background(), http.MethodGet, "/api/ApplicationAPI", url.Values{"page": []string{"2"}}, nil, &result)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	got := result.(map[string]any)
	if got["ok"] != true {
		t.Fatalf("result = %#v, want data payload", result)
	}
}

func TestClientSendsJSONBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode body error = %v", err)
		}
		if body["name"] != "app" {
			t.Fatalf("body name = %q, want app", body["name"])
		}
		_, _ = w.Write([]byte(`{"err":null,"data":{"id":1},"msg":null}`))
	}))
	defer server.Close()

	client := NewClient(&Config{URL: server.URL, APIToken: "token-1"}, false)
	var result any
	if err := client.Do(context.Background(), http.MethodPost, "/api/ApplicationAPI", nil, map[string]string{"name": "app"}, &result); err != nil {
		t.Fatalf("Do() error = %v", err)
	}
}

func TestClientReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"err":"bad-request","msg":"invalid"}`))
	}))
	defer server.Close()

	client := NewClient(&Config{URL: server.URL, APIToken: "token-1"}, false)
	var result any
	err := client.Do(context.Background(), http.MethodGet, "/api/ApplicationAPI", nil, nil, &result)
	if err == nil || !strings.Contains(err.Error(), "bad-request") {
		t.Fatalf("Do() error = %v, want bad-request", err)
	}
}

func TestDryRunMasksAPIToken(t *testing.T) {
	oldDryRun := dryRun
	oldVerboseSensitive := verboseSensitive
	dryRun = true
	verboseSensitive = false
	t.Cleanup(func() {
		dryRun = oldDryRun
		verboseSensitive = oldVerboseSensitive
	})

	stderr := captureStderr(t, func() {
		client := NewClient(&Config{URL: "https://apisec.example", APIToken: "HJn812345678a4cb"}, false)
		var result any
		_ = client.Do(context.Background(), http.MethodPost, "/api/RiskStrategyAPI", nil, map[string]any{"name": "x"}, &result)
	})

	if strings.Contains(stderr, "HJn812345678a4cb") {
		t.Fatalf("dry-run output leaked full token:\n%s", stderr)
	}
	if !strings.Contains(stderr, "HJn8...a4cb") {
		t.Fatalf("dry-run output missing masked token:\n%s", stderr)
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}
	os.Stderr = w
	fn()
	_ = w.Close()
	os.Stderr = old
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	return string(data)
}
