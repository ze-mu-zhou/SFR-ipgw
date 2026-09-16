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

func ParseVersion(s string) *Semver {
	m := versionPattern.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	n := make([]int, 3)
	for i := range n {
		v, e := strconv.Atoi(m[i+1])
		if e != nil {
			return nil
		}
		n[i] = v
	}
	for _, id := range strings.Split(m[4], ".") {
		if numeric(id) && len(id) > 1 && id[0] == '0' {
			return nil
		}
	}
	return &Semver{n[0], n[1], n[2], m[4]}
}
func numeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
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
	aa, bb := strings.Split(a.Prerelease, "."), strings.Split(b.Prerelease, ".")
	for i := 0; i < len(aa) && i < len(bb); i++ {
		if aa[i] == bb[i] {
			continue
		}
		an, bn := numeric(aa[i]), numeric(bb[i])
		if an != bn {
			return !an
		}
		if an && len(aa[i]) != len(bb[i]) {
			return len(aa[i]) > len(bb[i])
		}
		return aa[i] > bb[i]
	}
	return len(aa) > len(bb)
}
