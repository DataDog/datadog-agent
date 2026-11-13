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
