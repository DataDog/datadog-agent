// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package backend

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCatalogLatestMinus(t *testing.T) {
	for _, prefix := range []string{"", "v"} {
		name := "unprefixed"
		if prefix != "" {
			name = "prefixed"
		}
		t.Run(name, func(t *testing.T) {
			// Match getCatalog's descending version order, including repeated
			// patches/builds and interleaved entries for other packages/branches.
			catalog := Catalog{packages: []catalogEntry{
				{Package: "datadog-agent", branch: BranchTesting, Version: prefix + "7.84.0-devel.git.123-1"},
				{Package: "other-package", branch: BranchStable, Version: prefix + "7.83.0-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: prefix + "7.82.3-2"},
				{Package: "datadog-agent", branch: BranchStable, Version: prefix + "7.82.3-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: prefix + "7.82.1-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: prefix + "7.81.4-2"},
				{Package: "datadog-agent", branch: BranchTesting, Version: prefix + "7.81.4-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: prefix + "7.81.3-1"},
				{Package: "other-package", branch: BranchStable, Version: prefix + "7.81.2-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: prefix + "7.81.0-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: prefix + "7.80.2-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: prefix + "7.80.1-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: prefix + "7.79.1-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: prefix + "7.79.0-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: prefix + "7.78.2-1"},
			}}
			for _, tc := range []struct {
				name    string
				offset  int
				version string
			}{
				{name: "latest", offset: 0, version: "7.82.3-2"},
				{name: "N-1", offset: 1, version: "7.81.4-2"},
				{name: "N-2", offset: 2, version: "7.80.2-1"},
				{name: "N-3", offset: 3, version: "7.79.1-1"},
				{name: "N-4", offset: 4, version: "7.78.2-1"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					var version string
					require.NotPanics(t, func() {
						version = catalog.LatestMinus(BranchStable, "datadog-agent", tc.offset)
					})
					require.Equal(t, prefix+tc.version, version)
				})
			}
			require.Equal(t, prefix+"7.82.3-2", catalog.Latest(BranchStable, "datadog-agent"))
			require.Equal(t, prefix+"7.84.0-devel.git.123-1", catalog.Latest(BranchTesting, "datadog-agent"))
		})
	}
}

func TestCatalogLatestMinusUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name     string
		packages []catalogEntry
		branch   Branch
		pkg      string
		offset   int
		message  string
	}{
		{
			name: "empty catalog", branch: BranchStable, pkg: "datadog-agent",
			message: "package datadog-agent stable 0 not found",
		},
		{
			name: "missing package", branch: BranchStable, pkg: "datadog-agent",
			packages: []catalogEntry{{Package: "other-package", branch: BranchStable, Version: "7.82.0-1"}},
			message:  "package datadog-agent stable 0 not found",
		},
		{
			name: "missing branch", branch: BranchTesting, pkg: "datadog-agent",
			packages: []catalogEntry{{Package: "datadog-agent", branch: BranchStable, Version: "7.82.0-1"}},
			message:  "package datadog-agent testing 0 not found",
		},
		{
			name: "no older minor", branch: BranchStable, pkg: "datadog-agent", offset: 1,
			packages: []catalogEntry{
				{Package: "datadog-agent", branch: BranchStable, Version: "7.82.3-2"},
				{Package: "datadog-agent", branch: BranchStable, Version: "7.82.3-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: "7.82.0-1"},
			},
			message: "package datadog-agent stable 1 not found",
		},
		{
			name: "exhausted distinct minors", branch: BranchStable, pkg: "datadog-agent", offset: 2,
			packages: []catalogEntry{
				{Package: "datadog-agent", branch: BranchStable, Version: "v7.82.0-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: "v7.81.3-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: "v7.81.0-1"},
			},
			message: "package datadog-agent stable 1 not found",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalog := Catalog{packages: tc.packages}
			require.PanicsWithError(t, tc.message, func() {
				catalog.LatestMinus(tc.branch, tc.pkg, tc.offset)
			})
		})
	}
}

func TestCatalogPreviousMinorVersions(t *testing.T) {
	for _, prefix := range []string{"", "v"} {
		t.Run("prefix="+prefix, func(t *testing.T) {
			var catalog Catalog
			// Deliberately unsorted: results must not depend on registry order.
			for _, version := range []string{
				"7.84.0-1", "7.86.0-1", "7.86.2-2", "7.83.1-1", "7.85.1-1",
				"7.86.2-10", "7.86.2-1", "7.85.0-1", "7.84.1-1", "7.83.0-1",
				"7.86.2-1", // An immutable tag and a moving alias can point at the same package.
				"7.90.0-1", "7.87.0-1", "7.82.0-1", "8.86.0-1", "6.86.0-1",
				"7.86.3-rc.1-1", "7.86.3-devel.git.123-1", "7.86.3-beta.1-1",
			} {
				catalog.packages = append(catalog.packages, catalogEntry{
					Package: "datadog-agent", branch: BranchStable, Version: prefix + version,
				})
			}
			catalog.packages = append(catalog.packages,
				catalogEntry{Package: "other-package", branch: BranchStable, Version: prefix + "7.86.4-1"},
				catalogEntry{Package: "datadog-agent", branch: BranchTesting, Version: prefix + "7.86.5-1"},
				catalogEntry{Package: "other-package", branch: BranchStable, Version: "unknown"},
			)
			for _, tc := range []struct {
				name     string
				current  string
				offset   int
				versions []string
			}{
				{"N-1 all patches and revisions", "7.87.0-devel.git.123-1", 1, []string{"7.86.2-10", "7.86.2-2", "7.86.2-1", "7.86.0-1"}},
				{"N-2", "7.87.0-devel.git.123-1", 2, []string{"7.85.1-1", "7.85.0-1"}},
				{"N-3", "7.87.0-devel.git.123-1", 3, []string{"7.84.1-1", "7.84.0-1"}},
				{"N-4", "7.87.0-devel.git.123-1", 4, []string{"7.83.1-1", "7.83.0-1"}},
				{"release candidate anchors to candidate minor", "7.85.0-rc.1-1", 1, []string{"7.84.1-1", "7.84.0-1"}},
				{"patch candidate excludes its own minor", "7.85.2-1", 1, []string{"7.84.1-1", "7.84.0-1"}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					versions, err := catalog.PreviousMinorVersions(BranchStable, "datadog-agent", prefix+tc.current, tc.offset)
					require.NoError(t, err)
					expected := make([]string, len(tc.versions))
					for i, version := range tc.versions {
						expected[i] = prefix + version
					}
					require.Equal(t, expected, versions)
				})
			}
			versions, err := catalog.PreviousMinorVersions(BranchTesting, "datadog-agent", "7.87.0-1", 1)
			require.NoError(t, err)
			require.Equal(t, []string{prefix + "7.86.5-1"}, versions)
		})
	}
}

func TestCatalogPreviousMinorVersionsDeduplicatesPrefixes(t *testing.T) {
	catalog := Catalog{packages: []catalogEntry{
		{Package: "datadog-agent", branch: BranchStable, Version: "v7.86.1-1"},
		{Package: "datadog-agent", branch: BranchStable, Version: "7.86.1-1"},
		{Package: "datadog-agent", branch: BranchStable, Version: "v7.86.0-1"},
	}}
	versions, err := catalog.PreviousMinorVersions(BranchStable, "datadog-agent", "7.87.0-1", 1)
	require.NoError(t, err)
	require.Equal(t, []string{"7.86.1-1", "v7.86.0-1"}, versions)
}

func TestCatalogPreviousMinorVersionsUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name        string
		current     string
		offset      int
		packages    []catalogEntry
		error       string
		unavailable bool
	}{
		{name: "invalid candidate", current: "not-a-version", offset: 1, error: "invalid current package version"},
		{name: "abbreviated candidate", current: "7.87", offset: 1, error: "invalid current package version"},
		{name: "zero offset", current: "7.87.0-1", error: "minor release offset must be positive"},
		{name: "negative offset", current: "7.87.0-1", offset: -1, error: "minor release offset must be positive"},
		{name: "major boundary", current: "8.2.0-1", offset: 3, error: "precedes major release v8"},
		{name: "empty catalog", current: "7.87.0-1", offset: 1, error: "catalog contains no released versions"},
		{
			name: "missing exact minor does not substitute older release", current: "7.87.0-1", offset: 2,
			packages: []catalogEntry{
				{Package: "datadog-agent", branch: BranchStable, Version: "7.86.0-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: "7.84.0-1"},
			},
			error: "for minor 7.85", unavailable: true,
		},
		{
			name: "only prereleases or wrong package or branch", current: "7.87.0-1", offset: 1,
			packages: []catalogEntry{
				{Package: "datadog-agent", branch: BranchStable, Version: "7.86.0-rc.1-1"},
				{Package: "other-package", branch: BranchStable, Version: "7.86.0-1"},
				{Package: "datadog-agent", branch: BranchTesting, Version: "7.86.0-1"},
			},
			error: "catalog contains no released versions",
		},
		{
			name: "unreleased minor in otherwise populated release catalog", current: "7.87.0-1", offset: 1,
			packages: []catalogEntry{
				{Package: "datadog-agent", branch: BranchStable, Version: "7.86.0-rc.1-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: "7.85.0-1"},
			},
			error: "for minor 7.86", unavailable: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalog := Catalog{packages: tc.packages}
			versions, err := catalog.PreviousMinorVersions(BranchStable, "datadog-agent", tc.current, tc.offset)
			require.ErrorContains(t, err, tc.error)
			if tc.unavailable {
				require.ErrorIs(t, err, ErrNoReleasedVersions)
			} else {
				require.NotErrorIs(t, err, ErrNoReleasedVersions)
			}
			require.Nil(t, versions)
		})
	}
}

func TestCatalogPreviousMinorVersionsRejectsMalformedMetadata(t *testing.T) {
	for _, malformed := range []string{"", "unknown", "7", "7.86", "7.86.01-1", "7.86.3-01", "7.86.3-"} {
		t.Run(malformed, func(t *testing.T) {
			catalog := Catalog{packages: []catalogEntry{
				{Package: "datadog-agent", branch: BranchStable, Version: "7.85.0-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: malformed},
			}}
			versions, err := catalog.PreviousMinorVersions(BranchStable, "datadog-agent", "7.87.0-1", 1)
			require.ErrorContains(t, err, "invalid catalog package version")
			require.NotErrorIs(t, err, ErrNoReleasedVersions)
			require.Nil(t, versions)
		})
	}
}

// crtDecode reimplements the Microsoft C runtime command-line parser
// (the rules CommandLineToArgvW implements) and splits a command line into its
// arguments. It is the inverse of windowsCRTEscape and is used to verify that an
// escaped argument round-trips back to its original bytes. It implements the
// backslash/quote toggle rules; the "" -> " in-quotes rule is intentionally not
// implemented because windowsCRTEscape never emits a bare "" sequence.
func crtDecode(cmdline string) []string {
	var args []string
	var cur strings.Builder
	inQuotes := false
	hasArg := false
	for i := 0; i < len(cmdline); {
		c := cmdline[i]
		switch {
		case c == '\\':
			j := i
			for j < len(cmdline) && cmdline[j] == '\\' {
				j++
			}
			nb := j - i
			if j < len(cmdline) && cmdline[j] == '"' {
				cur.WriteString(strings.Repeat(`\`, nb/2))
				hasArg = true
				if nb%2 == 1 {
					cur.WriteByte('"')
				} else {
					inQuotes = !inQuotes
				}
				i = j + 1
			} else {
				cur.WriteString(strings.Repeat(`\`, nb))
				hasArg = true
				i = j
			}
		case c == '"':
			inQuotes = !inQuotes
			hasArg = true
			i++
		case (c == ' ' || c == '\t') && !inQuotes:
			if hasArg {
				args = append(args, cur.String())
				cur.Reset()
				hasArg = false
			}
			i++
		default:
			cur.WriteByte(c)
			hasArg = true
			i++
		}
	}
	if hasArg {
		args = append(args, cur.String())
	}
	return args
}

func TestWindowsCRTEscapeRoundTrip(t *testing.T) {
	cases := []string{
		"datadog-agent",
		"7.82.0-devel.git.147.abc.pipeline.120173372-1",
		`{"deployment_id":"123-0","file_operations":[{"file_op":"merge-patch","file_path":"/datadog.yaml","patch":{"extra_tags":["debug:step-0"]}}]}`,
		`{"packages":[{"package":"datadog-agent","version":"7.80.2-1","url":"oci://install.datadoghq.com/agent-package:7.80"}]}`,
		`{"deployment_id":"jq-replace-tag","file_operations":[{"file_op":"jq","file_path":"/datadog.yaml","transform":".tags |= map(if . == \"env:staging\" then $new_env else . end)","arguments":{"new_env":"env:prod"}}]}`,
		`{"log_level": "debug"}`,
		`value with spaces and "quotes"`,
		`back\slash and \"escaped\" quote`,
		`trailing-backslash\`,
	}

	for _, arg := range cases {
		// windowsCRTEscape emits a fully-quoted argument that PowerShell passes
		// through to the native command line verbatim (it does not re-quote an
		// argument that already contains double quotes). The C runtime must decode
		// it back to exactly the original argument.
		require.Equal(t, []string{arg}, crtDecode(windowsCRTEscape(arg)),
			"argument should round-trip through PowerShell + C runtime for %q", arg)
	}
}
