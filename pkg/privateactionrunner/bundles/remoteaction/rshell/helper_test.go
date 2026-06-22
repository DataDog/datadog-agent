// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_rshell

import (
	"slices"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
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
			in:   []string{setup.RShellCommandAllowAllWildcard},
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
