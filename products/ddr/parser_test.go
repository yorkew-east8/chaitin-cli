package ddr

import "testing"

func TestClassifyPathKeepsParameterContextForNestedList(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "top level list",
			path: "/softwaremanager/list",
			want: "list",
		},
		{
			name: "nested list under path parameter",
			path: "/softwaremanager/{software_hash}/list",
			want: "software-hash-list",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyOperationName(tt.path, "POST"); got != tt.want {
				t.Fatalf("classifyOperationName(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
