package version

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents a parsed semantic version in YY.MM.PPP format.
type Version struct {
	Major int
	Minor int
	Patch int
}

// ParseVersion parses a version string in YY.MM.PPP format.
// Build suffixes like "_r9" are stripped before parsing.
func ParseVersion(s string) (Version, error) {
	if s == "" {
		return Version{}, fmt.Errorf("empty version string")
	}

	// Strip build suffix (e.g., "_r9")
	if idx := strings.Index(s, "_"); idx >= 0 {
		s = s[:idx]
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid version format %q: expected 3 dot-separated parts", s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major version %q: %w", parts[0], err)
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Version{}, fmt.Errorf("invalid minor version %q: %w", parts[1], err)
	}

	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return Version{}, fmt.Errorf("invalid patch version %q: %w", parts[2], err)
	}

	return Version{Major: major, Minor: minor, Patch: patch}, nil
}

// LessThan returns true if v is semantically less than other.
func (v Version) LessThan(other Version) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	return v.Patch < other.Patch
}

// String returns the version formatted as YY.MM.PPP.
func (v Version) String() string {
	return fmt.Sprintf("%02d.%02d.%03d", v.Major, v.Minor, v.Patch)
}
