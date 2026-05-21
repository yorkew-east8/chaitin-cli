package version

import (
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Version
		wantErr bool
	}{
		{
			name:    "standard format",
			input:   "23.01.014",
			want:    Version{Major: 23, Minor: 1, Patch: 14},
			wantErr: false,
		},
		{
			name:    "with build suffix",
			input:   "25.03.001_r9",
			want:    Version{Major: 25, Minor: 3, Patch: 1},
			wantErr: false,
		},
		{
			name:    "older version",
			input:   "21.07.005",
			want:    Version{Major: 21, Minor: 7, Patch: 5},
			wantErr: false,
		},
		{
			name:    "zero patch",
			input:   "23.02.000",
			want:    Version{Major: 23, Minor: 2, Patch: 0},
			wantErr: false,
		},
		{
			name:    "large patch number",
			input:   "25.12.099",
			want:    Version{Major: 25, Minor: 12, Patch: 99},
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "only two parts",
			input:   "23.01",
			wantErr: true,
		},
		{
			name:    "non-numeric major",
			input:   "ab.01.014",
			wantErr: true,
		},
		{
			name:    "non-numeric minor",
			input:   "23.xx.014",
			wantErr: true,
		},
		{
			name:    "non-numeric patch",
			input:   "23.01.xxx",
			wantErr: true,
		},
		{
			name:    "too many parts",
			input:   "23.01.014.5",
			wantErr: true,
		},
		{
			name:    "build suffix with underscore only",
			input:   "23.01.014_",
			want:    Version{Major: 23, Minor: 1, Patch: 14},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseVersion(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got != tt.want {
				t.Errorf("ParseVersion(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestVersionLessThan(t *testing.T) {
	tests := []struct {
		name  string
		v     Version
		other Version
		want  bool
	}{
		{
			name:  "less by major",
			v:     Version{Major: 22, Minor: 12, Patch: 999},
			other: Version{Major: 23, Minor: 0, Patch: 0},
			want:  true,
		},
		{
			name:  "less by minor",
			v:     Version{Major: 23, Minor: 1, Patch: 999},
			other: Version{Major: 23, Minor: 2, Patch: 0},
			want:  true,
		},
		{
			name:  "less by patch",
			v:     Version{Major: 23, Minor: 2, Patch: 0},
			other: Version{Major: 23, Minor: 2, Patch: 1},
			want:  true,
		},
		{
			name:  "equal versions",
			v:     Version{Major: 23, Minor: 2, Patch: 1},
			other: Version{Major: 23, Minor: 2, Patch: 1},
			want:  false,
		},
		{
			name:  "greater by major",
			v:     Version{Major: 24, Minor: 0, Patch: 0},
			other: Version{Major: 23, Minor: 12, Patch: 999},
			want:  false,
		},
		{
			name:  "greater by minor",
			v:     Version{Major: 23, Minor: 3, Patch: 0},
			other: Version{Major: 23, Minor: 2, Patch: 999},
			want:  false,
		},
		{
			name:  "greater by patch",
			v:     Version{Major: 23, Minor: 2, Patch: 2},
			other: Version{Major: 23, Minor: 2, Patch: 1},
			want:  false,
		},
		{
			name:  "zero versions equal",
			v:     Version{Major: 0, Minor: 0, Patch: 0},
			other: Version{Major: 0, Minor: 0, Patch: 0},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.v.LessThan(tt.other); got != tt.want {
				t.Errorf("Version(%v).LessThan(%v) = %v, want %v", tt.v, tt.other, got, tt.want)
			}
		})
	}
}

func TestVersionString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "standard format",
			input: "23.01.014",
			want:  "23.01.014",
		},
		{
			name:  "strips build suffix",
			input: "25.03.001_r9",
			want:  "25.03.001",
		},
		{
			name:  "zero patch",
			input: "23.02.000",
			want:  "23.02.000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := ParseVersion(tt.input)
			if err != nil {
				t.Fatalf("ParseVersion(%q) unexpected error: %v", tt.input, err)
			}
			if got := v.String(); got != tt.want {
				t.Errorf("Version.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
