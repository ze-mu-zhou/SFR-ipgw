package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseVersion(t *testing.T) {
	check := assert.New(t)
	cases := []struct {
		version string
		parsed  *Semver
	}{
		{version: "v1.0.1", parsed: &Semver{
			Major:      1,
			Minor:      0,
			Patch:      1,
			Prerelease: "",
		}},
		{version: "v1.0.1-beta", parsed: &Semver{
			Major:      1,
			Minor:      0,
			Patch:      1,
			Prerelease: "beta",
		}},
		{version: "0.2.0-alpha.3", parsed: &Semver{
			Major:      0,
			Minor:      2,
			Patch:      0,
			Prerelease: "alpha.3",
		}},
		{version: "0.1", parsed: nil},
	}
	for _, tc := range cases {
		check.Equal(ParseVersion(tc.version), tc.parsed)
	}
}

func TestCompareVersion(t *testing.T) {
	check := assert.New(t)
	cases := []struct {
		left     string
		right    string
		expected bool
	}{
		{left: "v1.0.0", right: "v0.2.2", expected: true},
		{left: "v1.0.0-alpha", right: "v1.2.2", expected: false},
		{left: "v1.0.0-alpha", right: "v1.0.0", expected: false},
		{left: "v1.0.0", right: "v1.0.0-beta.2", expected: true},
		{left: "v1.0.0-beta", right: "v1.0.0-alpha", expected: true},
		{left: "v1", right: "v1.0.0-alpha", expected: false},
		{left: "1.0.0-beta", right: "", expected: true},
	}
	for _, tc := range cases {
		check.Equal(CompareVersion(ParseVersion(tc.left), ParseVersion(tc.right)), tc.expected)
	}
}
