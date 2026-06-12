package ddr

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestClientInjectsAuthHeaders(t *testing.T) {
	wantToken := base64.StdEncoding.EncodeToString([]byte("serval:test-token"))
	client := NewClient(&Config{
		URL:       "https://example.test",
		APIKey:    "test-token",
		CompanyID: "configured-company",
	}, nil, false)
	var seenPaths []string
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seenPaths = append(seenPaths, req.URL.Path)
			if got := req.Header.Get("Authorization"); got != "Serval "+wantToken {
				t.Fatalf("Authorization = %q, want %q", got, "Serval "+wantToken)
			}

			body := `{"code":0,"msg":"ok","data":{"items":[]}}`
			switch req.URL.Path {
			case "/qzh/api/auth/v1/ns/attributes":
				if got := req.Header.Get("X-CS-Header-Company"); got != "" {
					t.Fatalf("fetch company X-CS-Header-Company = %q, want empty", got)
				}
				body = `{"data":{"attributes":[{"k":"corp_name","v":"fetched-company"}]}}`
			case "/health":
				if got := req.Header.Get("X-CS-Header-Company"); got != "fetched-company" {
					t.Fatalf("X-CS-Header-Company = %q, want %q", got, "fetched-company")
				}
			default:
				t.Fatalf("unexpected path: %s", req.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		}),
	}

	var result map[string]interface{}
	if err := client.Do(context.Background(), http.MethodGet, "/health", nil, nil, nil, &result); err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if got := strings.Join(seenPaths, ","); got != "/qzh/api/auth/v1/ns/attributes,/health" {
		t.Fatalf("paths = %q", got)
	}
}

func TestBuildAuthorizationHeader(t *testing.T) {
	encodedServalToken := base64.StdEncoding.EncodeToString([]byte("serval:69eb402497c242046a88e64e"))
	encodedAPIKey := base64.StdEncoding.EncodeToString([]byte("serval:eeff7c64507045828b93fbcc57db7771"))

	tests := []struct {
		name   string
		apiKey string
		want   string
	}{
		{
			name:   "encoded serval token",
			apiKey: encodedServalToken,
			want:   "Serval " + encodedServalToken,
		},
		{
			name:   "raw api key",
			apiKey: "eeff7c64507045828b93fbcc57db7771",
			want:   "Serval " + encodedAPIKey,
		},
		{
			name:   "existing serval header",
			apiKey: "Serval " + encodedServalToken,
			want:   "Serval " + encodedServalToken,
		},
		{
			name:   "bearer header",
			apiKey: "Bearer jwt-token",
			want:   "Bearer jwt-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := buildAuthorizationHeader(tt.apiKey); got != tt.want {
				t.Fatalf("buildAuthorizationHeader() = %q, want %q", got, tt.want)
			}
		})
	}
}
