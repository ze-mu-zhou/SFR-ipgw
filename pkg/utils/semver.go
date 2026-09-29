package utils

import (
	"regexp"
	"strconv"
	"strings"
)

type Semver struct {
	Major, Minor, Patch int
	Prerelease          string
}

var versionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)

func ParseVersion(value string) *Semver {
	match := versionPattern.FindStringSubmatch(value)
	if match == nil {
		return nil
	}
	numbers := make([]int, 3)
	for i := range numbers {
		number, err := strconv.Atoi(match[i+1])
		if err != nil {
			return nil
		}
		numbers[i] = number
	}
	for _, id := range strings.Split(match[4], ".") {
		if numeric(id) && len(id) > 1 && id[0] == '0' {
			return nil
		}
	}
	return &Semver{numbers[0], numbers[1], numbers[2], match[4]}
}

func numeric(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func CompareVersion(a, b *Semver) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	if a.Major != b.Major {
		return a.Major > b.Major
	}
	if a.Minor != b.Minor {
		return a.Minor > b.Minor
	}
	if a.Patch != b.Patch {
		return a.Patch > b.Patch
	}
	if a.Prerelease == b.Prerelease {
		return false
	}
	if a.Prerelease == "" {
		return true
	}
	if b.Prerelease == "" {
		return false
	}
	aParts, bParts := strings.Split(a.Prerelease, "."), strings.Split(b.Prerelease, ".")
	for i := 0; i < len(aParts) && i < len(bParts); i++ {
		if aParts[i] == bParts[i] {
			continue
		}
		aNumeric, bNumeric := numeric(aParts[i]), numeric(bParts[i])
		if aNumeric != bNumeric {
			return !aNumeric
		}
		if aNumeric && len(aParts[i]) != len(bParts[i]) {
			return len(aParts[i]) > len(bParts[i])
		}
		return aParts[i] > bParts[i]
	}
	return len(aParts) > len(bParts)
}
