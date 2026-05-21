package version

import (
	"testing"
)

func TestFilterParams(t *testing.T) {
	tests := []struct {
		name          string
		commandKey    string
		params        map[string]string
		serverVersion string
		want          map[string]string
	}{
		{
			name:          "old version strips exclude_body from log/detect/list",
			commandKey:    "log/detect/list",
			params:        map[string]string{"exclude_body": "true", "page": "1"},
			serverVersion: "23.01.014",
			want:          map[string]string{"page": "1"},
		},
		{
			name:          "new version keeps exclude_body in log/detect/list",
			commandKey:    "log/detect/list",
			params:        map[string]string{"exclude_body": "true", "page": "1"},
			serverVersion: "23.02.001",
			want:          map[string]string{"exclude_body": "true", "page": "1"},
		},
		{
			name:          "old version strips exclude_body from log/access/list",
			commandKey:    "log/access/list",
			params:        map[string]string{"exclude_body": "true", "limit": "10"},
			serverVersion: "23.01.999",
			want:          map[string]string{"limit": "10"},
		},
		{
			name:          "new version keeps exclude_body in log/access/list",
			commandKey:    "log/access/list",
			params:        map[string]string{"exclude_body": "true", "limit": "10"},
			serverVersion: "24.01.000",
			want:          map[string]string{"exclude_body": "true", "limit": "10"},
		},
		{
			name:          "unknown command keeps all params",
			commandKey:    "unknown/command",
			params:        map[string]string{"exclude_body": "true", "foo": "bar"},
			serverVersion: "23.01.014",
			want:          map[string]string{"exclude_body": "true", "foo": "bar"},
		},
		{
			name:          "empty version keeps all params",
			commandKey:    "log/detect/list",
			params:        map[string]string{"exclude_body": "true", "page": "1"},
			serverVersion: "",
			want:          map[string]string{"exclude_body": "true", "page": "1"},
		},
		{
			name:          "invalid version keeps all params",
			commandKey:    "log/detect/list",
			params:        map[string]string{"exclude_body": "true", "page": "1"},
			serverVersion: "not.a.version",
			want:          map[string]string{"exclude_body": "true", "page": "1"},
		},
		{
			name:          "no constrained params in request",
			commandKey:    "log/detect/list",
			params:        map[string]string{"page": "1", "size": "10"},
			serverVersion: "23.01.014",
			want:          map[string]string{"page": "1", "size": "10"},
		},
		{
			name:          "version at exact min keeps param",
			commandKey:    "log/detect/list",
			params:        map[string]string{"exclude_body": "true", "page": "1"},
			serverVersion: "23.02.001",
			want:          map[string]string{"exclude_body": "true", "page": "1"},
		},
		{
			name:          "version one patch below min strips param",
			commandKey:    "log/detect/list",
			params:        map[string]string{"exclude_body": "true", "page": "1"},
			serverVersion: "23.02.000",
			want:          map[string]string{"page": "1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FilterParams(tt.commandKey, tt.params, tt.serverVersion)
			if len(got) != len(tt.want) {
				t.Errorf("FilterParams() returned %d params, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("FilterParams()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestFilterParamsDoesNotMutateInput(t *testing.T) {
	params := map[string]string{"exclude_body": "true", "page": "1"}
	original := map[string]string{"exclude_body": "true", "page": "1"}

	_ = FilterParams("log/detect/list", params, "23.01.014")

	for k, v := range original {
		if params[k] != v {
			t.Errorf("FilterParams mutated input: params[%q] = %q, want %q", k, params[k], v)
		}
	}
}
