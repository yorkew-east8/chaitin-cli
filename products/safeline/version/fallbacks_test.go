package version

import (
	"testing"
)

func TestStripFallbackParams(t *testing.T) {
	tests := []struct {
		name       string
		commandKey string
		params     map[string]string
		want       map[string]string
	}{
		{
			name:       "known command strips listed params",
			commandKey: "log/detect/list",
			params:     map[string]string{"exclude_body": "true", "page": "1"},
			want:       map[string]string{"page": "1"},
		},
		{
			name:       "known command log/access/list strips listed params",
			commandKey: "log/access/list",
			params:     map[string]string{"exclude_body": "true", "limit": "10"},
			want:       map[string]string{"limit": "10"},
		},
		{
			name:       "unknown command keeps all params",
			commandKey: "unknown/command",
			params:     map[string]string{"exclude_body": "true", "foo": "bar"},
			want:       map[string]string{"exclude_body": "true", "foo": "bar"},
		},
		{
			name:       "no fallback params present in input",
			commandKey: "log/detect/list",
			params:     map[string]string{"page": "1", "size": "10"},
			want:       map[string]string{"page": "1", "size": "10"},
		},
		{
			name:       "empty params",
			commandKey: "log/detect/list",
			params:     map[string]string{},
			want:       map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripFallbackParams(tt.commandKey, tt.params)
			if len(got) != len(tt.want) {
				t.Errorf("StripFallbackParams() returned %d params, want %d", len(got), len(tt.want))
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("StripFallbackParams()[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestStripFallbackParamsDoesNotMutateInput(t *testing.T) {
	params := map[string]string{"exclude_body": "true", "page": "1"}
	original := map[string]string{"exclude_body": "true", "page": "1"}

	_ = StripFallbackParams("log/detect/list", params)

	for k, v := range original {
		if params[k] != v {
			t.Errorf("StripFallbackParams mutated input: params[%q] = %q, want %q", k, params[k], v)
		}
	}
}
