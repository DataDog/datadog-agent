// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package backend

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetReleasedImageTags(t *testing.T) {
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"agent-package","tags":["7.85.2-2","7","7.85","7.85.2-1","7.54.0-crash-1","7.86.0-rc.1-1"]}`))
	}))
	t.Cleanup(registry.Close)
	tags, err := getReleasedImageTags(strings.TrimPrefix(registry.URL, "http://") + "/agent-package")
	require.NoError(t, err)
	require.Equal(t, []string{"7.85.2-2", "7.85.2-1"}, tags)
}

func TestCatalogPreviousMinorVersions(t *testing.T) {
	var catalog Catalog
	for _, version := range []string{
		"7.86.2-2", "7.84.0-1", "7.86.0-1", "v7.86.2-1", "7.86.2-1",
		"7.83.0-1", "v7.85.0-1", "7.90.0-1", "8.86.0-1",
		"7.86.3-rc.1-1", "7.86.3-devel.git.123-1",
	} {
		catalog.packages = append(catalog.packages, catalogEntry{
			Package: "datadog-agent", branch: BranchStable, Version: version,
		})
	}
	catalog.packages = append(catalog.packages,
		catalogEntry{Package: "other-package", branch: BranchStable, Version: "7.86.4-1"},
		catalogEntry{Package: "datadog-agent", branch: BranchTesting, Version: "7.86.5-1"},
	)
	for _, tc := range []struct {
		name     string
		offset   int
		versions []string
	}{
		{"N-1", 1, []string{"7.86.2-2", "7.86.2-1", "7.86.0-1"}},
		{"N-2", 2, []string{"v7.85.0-1"}},
		{"N-3", 3, []string{"7.84.0-1"}},
		{"N-4", 4, []string{"7.83.0-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			versions, err := catalog.PreviousMinorVersions(BranchStable, "datadog-agent", "v7.87.0-devel.git.123-1", tc.offset)
			require.NoError(t, err)
			require.Equal(t, tc.versions, versions)
		})
	}
}

func TestCatalogPreviousMinorVersionsUnavailable(t *testing.T) {
	for _, tc := range []struct {
		name        string
		packages    []catalogEntry
		unavailable bool
	}{
		{
			name: "missing minor does not substitute an older release",
			packages: []catalogEntry{
				{Package: "datadog-agent", branch: BranchStable, Version: "7.86.0-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: "7.84.0-1"},
			},
			unavailable: true,
		},
		{name: "empty catalog"},
		{
			name: "malformed catalog",
			packages: []catalogEntry{
				{Package: "datadog-agent", branch: BranchStable, Version: "7.84.0-1"},
				{Package: "datadog-agent", branch: BranchStable, Version: "unknown"},
			},
		},
		{
			name: "no stable releases",
			packages: []catalogEntry{
				{Package: "datadog-agent", branch: BranchStable, Version: "7.85.0-rc.1-1"},
				{Package: "datadog-agent", branch: BranchTesting, Version: "7.85.0-1"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalog := Catalog{packages: tc.packages}
			_, err := catalog.PreviousMinorVersions(BranchStable, "datadog-agent", "7.87.0-1", 2)
			require.Error(t, err)
			if tc.unavailable {
				require.ErrorIs(t, err, ErrNoReleasedVersions)
			} else {
				require.NotErrorIs(t, err, ErrNoReleasedVersions)
			}
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
