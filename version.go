package svcforge

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Version is a parsed Semantic Version 2.0.0 value.
type Version struct {
	Major      uint64
	Minor      uint64
	Patch      uint64
	Prerelease []string
	Build      []string
}

// ParseVersion parses Semantic Version 2.0.0. A leading "v" is accepted for Go-style release strings.
func ParseVersion(raw string) (Version, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "v") {
		raw = raw[1:]
	}
	if raw == "" {
		return Version{}, errors.New("version is empty")
	}
	var v Version
	mainAndBuild := strings.SplitN(raw, "+", 2)
	if len(mainAndBuild) == 2 {
		ids, err := parseIdentifiers(mainAndBuild[1], false)
		if err != nil {
			return Version{}, fmt.Errorf("build metadata: %w", err)
		}
		v.Build = ids
	}
	mainAndPre := strings.SplitN(mainAndBuild[0], "-", 2)
	if len(mainAndPre) == 2 {
		ids, err := parseIdentifiers(mainAndPre[1], true)
		if err != nil {
			return Version{}, fmt.Errorf("prerelease: %w", err)
		}
		v.Prerelease = ids
	}
	parts := strings.Split(mainAndPre[0], ".")
	if len(parts) != 3 {
		return Version{}, errors.New("semantic version core must be MAJOR.MINOR.PATCH")
	}
	values := []*uint64{&v.Major, &v.Minor, &v.Patch}
	for i, part := range parts {
		n, err := parseCoreNumber(part)
		if err != nil {
			return Version{}, err
		}
		*values[i] = n
	}
	return v, nil
}

// CompareVersions returns -1 when a < b, 0 when equal in precedence and 1 when a > b.
// Build metadata does not affect precedence, as required by Semantic Versioning.
func CompareVersions(a, b string) (int, error) {
	va, err := ParseVersion(a)
	if err != nil {
		return 0, fmt.Errorf("left version: %w", err)
	}
	vb, err := ParseVersion(b)
	if err != nil {
		return 0, fmt.Errorf("right version: %w", err)
	}
	return va.Compare(vb), nil
}

// Compare compares two parsed semantic versions by precedence.
func (v Version) Compare(other Version) int {
	if c := compareUint(v.Major, other.Major); c != 0 {
		return c
	}
	if c := compareUint(v.Minor, other.Minor); c != 0 {
		return c
	}
	if c := compareUint(v.Patch, other.Patch); c != 0 {
		return c
	}
	if len(v.Prerelease) == 0 && len(other.Prerelease) == 0 {
		return 0
	}
	if len(v.Prerelease) == 0 {
		return 1
	}
	if len(other.Prerelease) == 0 {
		return -1
	}
	limit := min(len(v.Prerelease), len(other.Prerelease))
	for i := 0; i < limit; i++ {
		a, b := v.Prerelease[i], other.Prerelease[i]
		an, aNum := numericIdentifier(a)
		bn, bNum := numericIdentifier(b)
		switch {
		case aNum && bNum:
			if c := compareUint(an, bn); c != 0 {
				return c
			}
		case aNum:
			return -1
		case bNum:
			return 1
		default:
			if a < b {
				return -1
			}
			if a > b {
				return 1
			}
		}
	}
	if len(v.Prerelease) < len(other.Prerelease) {
		return -1
	}
	if len(v.Prerelease) > len(other.Prerelease) {
		return 1
	}
	return 0
}

func parseCoreNumber(raw string) (uint64, error) {
	if raw == "" {
		return 0, errors.New("version contains an empty numeric component")
	}
	if len(raw) > 1 && raw[0] == '0' {
		return 0, fmt.Errorf("numeric component %q has a leading zero", raw)
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("numeric component %q is invalid", raw)
		}
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("numeric component %q is too large", raw)
	}
	return n, nil
}

func parseIdentifiers(raw string, rejectNumericLeadingZero bool) ([]string, error) {
	if raw == "" {
		return nil, errors.New("identifier list is empty")
	}
	parts := strings.Split(raw, ".")
	for _, part := range parts {
		if part == "" {
			return nil, errors.New("identifier is empty")
		}
		for _, r := range part {
			if !(r == '-' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
				return nil, fmt.Errorf("identifier %q contains an invalid character", part)
			}
		}
		if rejectNumericLeadingZero && len(part) > 1 && part[0] == '0' {
			if _, ok := numericIdentifier(part); ok {
				return nil, fmt.Errorf("numeric prerelease identifier %q has a leading zero", part)
			}
		}
	}
	return parts, nil
}

func numericIdentifier(raw string) (uint64, bool) {
	if raw == "" {
		return 0, false
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	return n, err == nil
}

func compareUint(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
