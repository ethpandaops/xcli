package release

import (
	"fmt"
	"regexp"
	"strconv"
)

// Version represents a parsed semantic version.
type Version struct {
	Major int
	Minor int
	Patch int
}

// semverRegex matches versions like "v1.2.3" or "1.2.3".
var semverRegex = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

// ParseVersion parses a version string like "v1.2.3" or "1.2.3".
func ParseVersion(v string) (*Version, error) {
	matches := semverRegex.FindStringSubmatch(v)
	if matches == nil {
		return nil, fmt.Errorf("invalid semver format: %s", v)
	}

	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return nil, fmt.Errorf("invalid major version: %w", err)
	}

	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return nil, fmt.Errorf("invalid minor version: %w", err)
	}

	patch, err := strconv.Atoi(matches[3])
	if err != nil {
		return nil, fmt.Errorf("invalid patch version: %w", err)
	}

	return &Version{
		Major: major,
		Minor: minor,
		Patch: patch,
	}, nil
}

// String returns the version as "vX.Y.Z" format.
func (v *Version) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Bump returns a new version with the specified bump applied.
func (v *Version) Bump(bumpType BumpType) *Version {
	switch bumpType {
	case BumpMajor:
		return &Version{
			Major: v.Major + 1,
			Minor: 0,
			Patch: 0,
		}
	case BumpMinor:
		return &Version{
			Major: v.Major,
			Minor: v.Minor + 1,
			Patch: 0,
		}
	case BumpPatch:
		return &Version{
			Major: v.Major,
			Minor: v.Minor,
			Patch: v.Patch + 1,
		}
	default:
		// Default to patch bump for unknown types
		return &Version{
			Major: v.Major,
			Minor: v.Minor,
			Patch: v.Patch + 1,
		}
	}
}

// Compare returns -1 if v < other, 0 if equal, 1 if v > other.
func (v *Version) Compare(other *Version) int {
	if v.Major != other.Major {
		if v.Major < other.Major {
			return -1
		}

		return 1
	}

	if v.Minor != other.Minor {
		if v.Minor < other.Minor {
			return -1
		}

		return 1
	}

	if v.Patch != other.Patch {
		if v.Patch < other.Patch {
			return -1
		}

		return 1
	}

	return 0
}
