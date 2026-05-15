package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestIsParamError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "invalid_param error",
			err:  fmt.Errorf("API error [invalid_param]: some field is invalid"),
			want: true,
		},
		{
			name: "invalid_parameter error",
			err:  fmt.Errorf("API error [invalid_parameter]: unknown field"),
			want: true,
		},
		{
			name: "invalid_filter_key error",
			err:  fmt.Errorf("API error [invalid_filter_key]: filter not supported"),
			want: true,
		},
		{
			name: "unknown parameter error",
			err:  fmt.Errorf("API error [err]: unknown parameter 'foo'"),
			want: true,
		},
		{
			name: "unexpected parameter error",
			err:  fmt.Errorf("API error [err]: unexpected parameter 'bar'"),
			want: true,
		},
		{
			name: "non-param API error",
			err:  fmt.Errorf("API error [permission_denied]: no access"),
			want: false,
		},
		{
			name: "network error",
			err:  fmt.Errorf("request failed: connection refused"),
			want: false,
		},
		{
			name: "HTTP status error",
			err:  fmt.Errorf("HTTP 500: internal server error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsParamError(tt.err); got != tt.want {
				t.Errorf("IsParamError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDoWithFallback_NoError(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		resp := Envelope{Data: json.RawMessage(`"ok"`)}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	env, err := c.DoWithFallback("GET", "/test", nil,
		map[string]string{"new_param": "value"},
		map[string]string{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 call, got %d", calls.Load())
	}
	if env == nil {
		t.Fatal("expected non-nil envelope")
	}
}

func TestDoWithFallback_ParamErrorRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			// First call: return parameter error
			errCode := "invalid_param"
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Envelope{
				Err: &errCode,
				Msg: json.RawMessage(`"unsupported parameter"`),
			})
			return
		}
		// Second call: success
		resp := Envelope{Data: json.RawMessage(`"ok"`)}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	env, err := c.DoWithFallback("GET", "/test", nil,
		map[string]string{"new_param": "value"},
		map[string]string{},
	)
	if err != nil {
		t.Fatalf("unexpected error after fallback: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls, got %d", calls.Load())
	}
	if env == nil {
		t.Fatal("expected non-nil envelope")
	}
}

func TestDoWithFallback_NonParamErrorNoRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		errCode := "permission_denied"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Envelope{
			Err: &errCode,
			Msg: json.RawMessage(`"no access"`),
		})
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	_, err := c.DoWithFallback("GET", "/test", nil,
		map[string]string{"new_param": "value"},
		map[string]string{},
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 call (no retry), got %d", calls.Load())
	}
}

func TestDoWithFallback_ParamErrorNilFallbackNoRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		errCode := "invalid_param"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Envelope{
			Err: &errCode,
			Msg: json.RawMessage(`"unsupported parameter"`),
		})
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	_, err := c.DoWithFallback("GET", "/test", nil,
		map[string]string{"new_param": "value"},
		nil,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 call (no retry with nil fallback), got %d", calls.Load())
	}
}

func TestDoMultiWithFallback_ParamErrorRetries(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			// First call: return parameter error
			errCode := "invalid_filter_key"
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(Envelope{
				Err: &errCode,
				Msg: json.RawMessage(`"filter key not supported"`),
			})
			return
		}
		// Second call: success
		resp := Envelope{Data: json.RawMessage(`"ok"`)}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	env, err := c.DoMultiWithFallback("GET", "/test", nil,
		map[string]string{"new_param": "value"},
		map[string][]string{"sort": {"asc", "name"}},
		map[string]string{},
		map[string][]string{},
	)
	if err != nil {
		t.Fatalf("unexpected error after fallback: %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls, got %d", calls.Load())
	}
	if env == nil {
		t.Fatal("expected non-nil envelope")
	}
}

func TestDoMultiWithFallback_NonParamErrorNoRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		errCode := "internal_error"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Envelope{
			Err: &errCode,
			Msg: json.RawMessage(`"something went wrong"`),
		})
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	_, err := c.DoMultiWithFallback("GET", "/test", nil,
		map[string]string{"new_param": "value"},
		map[string][]string{"sort": {"asc"}},
		map[string]string{},
		map[string][]string{},
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 call (no retry), got %d", calls.Load())
	}
}

func TestDoMultiWithFallback_ParamErrorNilFallbackNoRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		errCode := "invalid_param"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Envelope{
			Err: &errCode,
			Msg: json.RawMessage(`"unsupported parameter"`),
		})
	}))
	defer srv.Close()

	c := New(srv.URL, srv.Client())
	_, err := c.DoMultiWithFallback("GET", "/test", nil,
		map[string]string{"new_param": "value"},
		map[string][]string{"sort": {"asc"}},
		nil, nil,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 call (no retry with nil fallback), got %d", calls.Load())
	}
}
