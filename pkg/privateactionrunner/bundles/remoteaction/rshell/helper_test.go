// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin || windows

package com_datadoghq_remoteaction_rshell

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestOnlyRshellPrefixedCommands pins the namespace-scoping applied to the
// signed backend command list before it is handed to rshell.
func TestOnlyRshellPrefixedCommands(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "nil",
			in:   nil,
			want: []string{},
		},
		{
			name: "empty",
			in:   []string{},
			want: []string{},
		},
		{
			name: "all in namespace",
			in:   []string{"rshell:cat", "rshell:ls"},
			want: []string{"rshell:cat", "rshell:ls"},
		},
		{
			name: "none in namespace",
			in:   []string{"evil:cat", "git:checkout"},
			want: []string{},
		},
		{
			name: "mixed: only namespaced kept",
			in:   []string{"rshell:cat", "evil:cat", "rshell:ls", "ls"},
			want: []string{"rshell:cat", "rshell:ls"},
		},
		{
			name: "the wildcard token itself is rshell-prefixed and would be admitted, note that this should never happen in practice",
			in:   []string{rShellCommandAllowAllWildcard},
			want: []string{},
		},
		{
			name: "bare 'rshell' without colon is not admitted (the colon is part of the prefix)",
			in:   []string{"rshell"},
			want: []string{},
		},
		{
			name: "'rshell:' alone (empty name after prefix) is admitted by the prefix check",
			in:   []string{"rshell:"},
			want: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := onlyRshellPrefixedCommands(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestCleanPathList covers the post-condition that every entry is
// path.Clean-normalized AND ends with a single separator. Empty input must
// not panic (this was a regression in an earlier draft).
func TestCleanPathList(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "nil input",
			in:   nil,
			want: []string{},
		},
		{
			name: "empty input",
			in:   []string{},
			want: []string{},
		},
		{
			name: "single path without trailing slash",
			in:   []string{"/var/log"},
			want: []string{"/var/log/"},
		},
		{
			name: "single path with trailing slash",
			in:   []string{"/var/log/"},
			want: []string{"/var/log/"},
		},
		{
			name: "root",
			in:   []string{"/"},
			want: []string{"/"},
		},
		{
			name: "redundant separators collapsed",
			in:   []string{"//var//log//"},
			want: []string{"/var/log/"},
		},
		{
			name: "multiple paths",
			in:   []string{"/var/log", "/etc/"},
			want: []string{"/var/log/", "/etc/"},
		},
		{
			name: "path.Clean resolves dot segments",
			in:   []string{"/var/./log/../log"},
			want: []string{"/var/log/"},
		},
		{
			name: "empty string becomes ./",
			in:   []string{""},
			want: []string{"./"},
		},
		{
			name: "parent (..) escape to root collapses to /",
			in:   []string{"/.."},
			want: []string{"/"},
		},
		{
			name: "interior parent segments are resolved",
			in:   []string{"/var/../etc/../usr/local"},
			want: []string{"/usr/local/"},
		},
		{
			name: "read-only suffix remains at end after path normalization",
			in:   []string{"/host/var/log:ro"},
			want: []string{"/host/var/log/:ro"},
		},
		{
			name: "read-write suffix remains at end after path normalization",
			in:   []string{"/host/datadog:rw"},
			want: []string{"/host/datadog/:rw"},
		},
		{
			name: "path cleanup happens before access suffix is reattached",
			in:   []string{"/host/./datadog/../datadog:rw"},
			want: []string{"/host/datadog/:rw"},
		},
		{
			name: "different access overlays are preserved independently",
			in:   []string{"/var/log:ro", "/var/log/datadog:rw"},
			want: []string{"/var/log/:ro", "/var/log/datadog/:rw"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanPathList(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestCleanPathListDoesNotMutateInput pins the API contract that cleanPathList
// allocates a fresh output slice rather than rewriting in place.
func TestCleanPathListDoesNotMutateInput(t *testing.T) {
	in := []string{"/var/log", "/etc"}
	original := slices.Clone(in)

	cleanPathList(in)

	assert.Equal(t, original, in, "input must not be mutated")
}

func TestPathSpecPathStripsAccessSuffixForLocalStat(t *testing.T) {
	assert.Equal(t, "/host/datadog/", pathSpecPath("/host/datadog/:rw"))
	assert.Equal(t, "/host/var/log/", pathSpecPath("/host/var/log/:ro"))
	assert.Equal(t, "/etc/", pathSpecPath("/etc/"))
}

func TestNarrowerPathWithSameAccessRootAllowsAbsolutePaths(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "posix absolute",
			path: "/var/log/:ro",
			want: true,
		},
		{
			name: "windows drive-rooted absolute",
			path: "C:/Users/ContainerAdministrator/AppData/Local/Temp/:rw",
			want: true,
		},
		{
			name: "windows drive-relative",
			path: "C:Users/ContainerAdministrator/AppData/Local/Temp/:rw",
			want: false,
		},
		{
			name: "relative",
			path: "var/log/:ro",
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pathToKeep, ok := narrowerPathWithSameAccess("/", tc.path)

			assert.Equal(t, tc.want, ok)
			if tc.want {
				assert.Equal(t, tc.path, pathToKeep)
			}
		})
	}
}

func TestIsWindowsAbsolutePathSpecPath(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "uppercase drive letter with slash separator",
			path: "C:/Windows/",
			want: true,
		},
		{
			name: "lowercase drive letter with slash separator",
			path: "z:/tmp/",
			want: true,
		},
		{
			name: "non-letter drive",
			path: "1:/tmp/",
			want: false,
		},
		{
			name: "drive-relative path",
			path: "C:Windows/",
			want: false,
		},
		{
			name: "backslash separators are not rshell path specs",
			path: `C:\Windows\`,
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isWindowsAbsolutePathSpecPath(tc.path))
		})
	}
}

func TestReducePathListToBroadest(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "nil input",
			in:   nil,
			want: []string{},
		},
		{
			name: "empty input",
			in:   []string{},
			want: []string{},
		},
		{
			name: "read-only paths reduce to broadest prefix",
			in:   []string{"/var/log/", "/var/log/datadog/", "/etc/"},
			want: []string{"/etc/", "/var/log/"},
		},
		{
			name: "read-only paths reduce when broadest appears last",
			in:   []string{"/var/log/datadog/", "/var/log/datadog/agent/", "/var/log/"},
			want: []string{"/var/log/"},
		},
		{
			name: "read-only paths reduce through multiple replacements",
			in:   []string{"/var/log/datadog/agent/", "/var/log/datadog/", "/var/log/"},
			want: []string{"/var/log/"},
		},
		{
			name: "unrelated read-only sibling prefixes are preserved",
			in:   []string{"/var/log/", "/var/logger/", "/var/logs/"},
			want: []string{"/var/log/", "/var/logger/", "/var/logs/"},
		},
		{
			name: "read-write paths reduce independently",
			in:   []string{"/var/log/:rw", "/var/log/datadog/:rw", "/etc/"},
			want: []string{"/etc/", "/var/log/:rw"},
		},
		{
			name: "read-write paths reduce when broadest appears last",
			in:   []string{"/var/log/datadog/agent/:rw", "/var/log/datadog/:rw", "/var/log/:rw"},
			want: []string{"/var/log/:rw"},
		},
		{
			name: "unrelated read-write sibling prefixes are preserved",
			in:   []string{"/var/log/:rw", "/var/logger/:rw", "/var/logs/:rw"},
			want: []string{"/var/log/:rw", "/var/logger/:rw", "/var/logs/:rw"},
		},
		{
			name: "read-only path does not swallow read-write descendant",
			in:   []string{"/var/log/", "/var/log/datadog/:rw"},
			want: []string{"/var/log/", "/var/log/datadog/:rw"},
		},
		{
			name: "read-write path does not swallow read-only descendant",
			in:   []string{"/var/log/:rw", "/var/log/datadog/"},
			want: []string{"/var/log/:rw", "/var/log/datadog/"},
		},
		{
			name: "read-write path replaces unsuffixed read-only path with same path",
			in:   []string{"/var/log/", "/var/log/:rw"},
			want: []string{"/var/log/:rw"},
		},
		{
			name: "read-write path replaces explicit read-only path with same path",
			in:   []string{"/var/log/:ro", "/var/log/:rw"},
			want: []string{"/var/log/:rw"},
		},
		{
			name: "explicit read-only suffix is preserved",
			in:   []string{"/var/log/:ro", "/var/log/datadog/:ro"},
			want: []string{"/var/log/:ro"},
		},
		{
			name: "explicit read-only suffix is preserved when broadest appears after unsuffixed descendant",
			in:   []string{"/var/log/datadog/", "/var/log/:ro"},
			want: []string{"/var/log/:ro"},
		},
		{
			name: "unsuffixed broadest path wins over explicit read-only descendant",
			in:   []string{"/var/log/datadog/:ro", "/var/log/"},
			want: []string{"/var/log/"},
		},
		{
			name: "duplicate read-only path keeps explicit read-only suffix",
			in:   []string{"/var/log/", "/var/log/:ro"},
			want: []string{"/var/log/:ro"},
		},
		{
			name: "duplicate read-only path keeps explicit read-only suffix regardless of order",
			in:   []string{"/var/log/:ro", "/var/log/"},
			want: []string{"/var/log/:ro"},
		},
		{
			name: "duplicates are removed across access buckets",
			in:   []string{"/var/log/:rw", "/var/log/:rw", "/var/log/", "/var/log/"},
			want: []string{"/var/log/:rw"},
		},
		{
			name: "root read-only reduces all read-only paths only",
			in:   []string{"/var/log/", "/etc/", "/"},
			want: []string{"/"},
		},
		{
			name: "root read-write replaces read-only root with same path",
			in:   []string{"/var/log/:rw", "/etc/:rw", "/:rw", "/"},
			want: []string{"/:rw"},
		},
		{
			name: "mixed access reductions stay isolated",
			in: []string{
				"/var/log/datadog/:rw",
				"/var/log/:rw",
				"/var/log/datadog/agent/",
				"/var/log/datadog/:ro",
				"/opt/datadog/:rw",
				"/opt/",
			},
			want: []string{
				"/opt/",
				"/opt/datadog/:rw",
				"/var/log/:rw",
				"/var/log/datadog/:ro",
			},
		},
		{
			name: "output is sorted after reduction",
			in:   []string{"/zeta/", "/alpha/:rw", "/alpha/beta/:rw", "/beta/"},
			want: []string{"/alpha/:rw", "/beta/", "/zeta/"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, reducePathListToBroadest(tc.in))
		})
	}
}
