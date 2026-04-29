// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_rshell

import (
	"os"
	"slices"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/stretchr/testify/assert"
)

// TestOnlyRshellPrefixedCommands pins the namespace-scoping the wildcard
// branch of filterAllowedCommands relies on. Without it, the wildcard
// sentinel would either admit arbitrary backend entries (security
// weakness) or admit nothing (kill-switch).
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

// TestCommonPath pins every shape of the (a, b) pair commonPath can see.
// Pre-condition: both inputs are cleaned and end with "/". Tests follow that
// contract, since callers always pass cleaned forms.
func TestCommonPath(t *testing.T) {
	cases := []struct {
		name         string
		a            string
		b            string
		wantDeepest  string
		wantBroadest string
	}{
		{
			name:         "equal paths",
			a:            "/var/log/",
			b:            "/var/log/",
			wantDeepest:  "/var/log/",
			wantBroadest: "/var/log/",
		},
		{
			name:         "a deeper than b",
			a:            "/var/log/nginx/",
			b:            "/var/log/",
			wantDeepest:  "/var/log/nginx/",
			wantBroadest: "/var/log/",
		},
		{
			name:         "b deeper than a",
			a:            "/var/log/",
			b:            "/var/log/nginx/",
			wantDeepest:  "/var/log/nginx/",
			wantBroadest: "/var/log/",
		},
		{
			name:         "root contains everything",
			a:            "/",
			b:            "/var/log/",
			wantDeepest:  "/var/log/",
			wantBroadest: "/",
		},
		{
			name:         "everything is contained by root",
			a:            "/var/log/",
			b:            "/",
			wantDeepest:  "/var/log/",
			wantBroadest: "/",
		},
		{
			name:         "both root",
			a:            "/",
			b:            "/",
			wantDeepest:  "/",
			wantBroadest: "/",
		},
		{
			name:         "no relation: prefix siblings (var/log vs var/logger)",
			a:            "/var/log/",
			b:            "/var/logger/",
			wantDeepest:  "",
			wantBroadest: "",
		},
		{
			name:         "no relation: prefix siblings reversed",
			a:            "/var/logger/",
			b:            "/var/log/",
			wantDeepest:  "",
			wantBroadest: "",
		},
		{
			name:         "no relation: disjoint paths",
			a:            "/var/log/",
			b:            "/etc/",
			wantDeepest:  "",
			wantBroadest: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			deepest, broadest := commonPath(tc.a, tc.b)
			assert.Equal(t, tc.wantDeepest, deepest, "deepest")
			assert.Equal(t, tc.wantBroadest, broadest, "broadest")
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

// TestReducePathListToBroadest covers single-input cases, full-domination
// scenarios (one entry absorbs others), prefix-sibling preservation, and
// the multi-domination case (regression: an earlier draft only absorbed
// the first dominated entry).
//
// Pre-condition: inputs are already cleaned (path.Clean + trailing "/").
//
// Post-condition: output is deduplicated, sorted (the implementation uses
// slices.Sort + slices.Compact), and contains no two entries where one
// contains the other.
func TestReducePathListToBroadest(t *testing.T) {
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
			name: "single entry",
			in:   []string{"/var/log/"},
			want: []string{"/var/log/"},
		},
		{
			name: "exact duplicates collapse",
			in:   []string{"/var/log/", "/var/log/"},
			want: []string{"/var/log/"},
		},
		{
			name: "broader entry first absorbs deeper",
			in:   []string{"/var/", "/var/log/"},
			want: []string{"/var/"},
		},
		{
			name: "deeper entry first is absorbed by broader",
			in:   []string{"/var/log/", "/var/"},
			want: []string{"/var/"},
		},
		{
			name: "broader at the end absorbs MULTIPLE deeper entries",
			in:   []string{"/var/log/a/", "/var/log/b/", "/var/"},
			want: []string{"/var/"},
		},
		{
			name: "broader at the beginning absorbs MULTIPLE deeper entries",
			in:   []string{"/var/", "/var/log/a/", "/var/log/b/"},
			want: []string{"/var/"},
		},
		{
			name: "broader in the middle absorbs MULTIPLE deeper entries",
			in:   []string{"/var/log/a/", "/var/", "/var/log/b/"},
			want: []string{"/var/"},
		},
		{
			name: "root absorbs everything",
			in:   []string{"/var/log/", "/etc/", "/"},
			want: []string{"/"},
		},
		{
			name: "prefix siblings are kept separately",
			in:   []string{"/var/log/", "/var/logger/"},
			want: []string{"/var/log/", "/var/logger/"},
		},
		{
			name: "disjoint paths kept and sorted",
			in:   []string{"/var/log/", "/etc/", "/tmp/"},
			want: []string{"/etc/", "/tmp/", "/var/log/"},
		},
		{
			name: "three siblings under one parent collapse to parent",
			in:   []string{"/var/a/", "/var/b/", "/var/c/", "/var/"},
			want: []string{"/var/"},
		},
		{
			name: "interleaved related and unrelated",
			in:   []string{"/var/log/", "/etc/foo/", "/var/", "/etc/"},
			want: []string{"/etc/", "/var/"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := reducePathListToBroadest(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestReducePathListToBroadestIsIdempotent pins the property that running
// reduce on an already-reduced list is a no-op. The post-condition implies
// this, but a direct check guards against subtle bugs where a second pass
// drops or reorders entries.
func TestReducePathListToBroadestIsIdempotent(t *testing.T) {
	in := []string{"/var/log/a/", "/var/log/b/", "/var/", "/etc/foo/"}
	once := reducePathListToBroadest(in)
	twice := reducePathListToBroadest(slices.Clone(once))

	assert.Equal(t, once, twice, "reduce(reduce(x)) must equal reduce(x)")
}

// TestReducePathListToBroadestIsOrderIndependent pins the property that
// the same set of input paths yields the same reduced output regardless of
// iteration order. The implementation sorts at the end, but order can still
// matter inside the loop (e.g. if an early reduction would have absorbed a
// later entry differently).
func TestReducePathListToBroadestIsOrderIndependent(t *testing.T) {
	permutations := [][]string{
		{"/var/", "/var/log/a/", "/var/log/b/", "/etc/"},
		{"/var/log/a/", "/var/", "/var/log/b/", "/etc/"},
		{"/var/log/a/", "/var/log/b/", "/var/", "/etc/"},
		{"/var/log/b/", "/var/log/a/", "/etc/", "/var/"},
		{"/etc/", "/var/log/a/", "/var/log/b/", "/var/"},
	}
	expected := reducePathListToBroadest(permutations[0])
	for i := 1; i < len(permutations); i++ {
		got := reducePathListToBroadest(permutations[i])
		assert.Equal(t, expected, got, "permutation #%d must match the canonical reduction", i)
	}
}

// TestIntersectPathLists pins the "narrower side wins" semantics of the
// containment-based intersection. Pre-condition: both inputs are cleaned
// AND reduced (no two entries on the same side dominate each other).
//
// The matrix below covers: empty inputs, equal lists, single-entry
// containment in either direction, multiple narrower entries under one
// broader entry (regression: an earlier draft broke too eagerly out of the
// inner loop), prefix siblings, fully disjoint, and a multi-element
// real-world-shaped case.
func TestIntersectPathLists(t *testing.T) {
	cases := []struct {
		name         string
		list1, list2 []string
		want         []string
	}{
		{
			name:  "both empty",
			list1: []string{},
			list2: []string{},
			want:  []string{},
		},
		{
			name:  "list1 empty",
			list1: []string{},
			list2: []string{"/var/log/"},
			want:  []string{},
		},
		{
			name:  "list2 empty",
			list1: []string{"/var/log/"},
			list2: []string{},
			want:  []string{},
		},
		{
			name:  "equal single-entry lists",
			list1: []string{"/var/log/"},
			list2: []string{"/var/log/"},
			want:  []string{"/var/log/"},
		},
		{
			name:  "list1 contains list2 (narrower wins; list2 admitted)",
			list1: []string{"/var/"},
			list2: []string{"/var/log/"},
			want:  []string{"/var/log/"},
		},
		{
			name:  "list2 contains list1 (narrower wins; list1 admitted)",
			list1: []string{"/var/log/"},
			list2: []string{"/var/"},
			want:  []string{"/var/log/"},
		},
		{
			name:  "list1 broader admits MULTIPLE list2 narrower entries",
			list1: []string{"/var/"},
			list2: []string{"/var/log/", "/var/spool/"},
			want:  []string{"/var/log/", "/var/spool/"},
		},
		{
			name:  "list2 broader admits MULTIPLE list1 narrower entries",
			list1: []string{"/var/log/", "/var/spool/"},
			list2: []string{"/var/"},
			want:  []string{"/var/log/", "/var/spool/"},
		},
		{
			name:  "prefix siblings produce no intersection",
			list1: []string{"/var/log/"},
			list2: []string{"/var/logger/"},
			want:  []string{},
		},
		{
			name:  "disjoint paths produce no intersection",
			list1: []string{"/var/log/"},
			list2: []string{"/etc/"},
			want:  []string{},
		},
		{
			name:  "root in list1 admits all list2 entries",
			list1: []string{"/"},
			list2: []string{"/var/log/", "/etc/"},
			want:  []string{"/var/log/", "/etc/"},
		},
		{
			name:  "two-by-two: each side has unrelated entries; only related pair admits",
			list1: []string{"/var/", "/etc/"},
			list2: []string{"/var/log/", "/opt/"},
			want:  []string{"/var/log/"},
		},
		{
			name:  "root in list2 admits all list1 entries (symmetry)",
			list1: []string{"/var/log/", "/etc/"},
			list2: []string{"/"},
			want:  []string{"/var/log/", "/etc/"},
		},
		{
			name:  "two-by-two: each list1 entry has a containing list2 entry",
			list1: []string{"/var/log/", "/etc/foo/"},
			list2: []string{"/var/", "/etc/"},
			want:  []string{"/var/log/", "/etc/foo/"},
		},
		{
			name:  "three-by-three with mixed disjoint, contained, and matching pairs",
			list1: []string{"/var/", "/etc/", "/opt/"},
			list2: []string{"/var/log/", "/etc/", "/srv/"},
			want:  []string{"/var/log/", "/etc/"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := intersectPathLists(tc.list1, tc.list2)
			assert.ElementsMatch(t, tc.want, got, "set of admitted paths")
		})
	}
}

// TestBackendPathsForEnv pins the env-driven dispatch and the failure
// modes when the relevant key is missing.
func TestBackendPathsForEnv(t *testing.T) {
	cases := []struct {
		name          string
		containerized bool
		in            map[string][]string
		want          []string
	}{
		{
			name:          "nil map → nil slice (kill-switch downstream)",
			containerized: false,
			in:            nil,
			want:          nil,
		},
		{
			name:          "empty map → nil slice (kill-switch downstream)",
			containerized: false,
			in:            map[string][]string{},
			want:          nil,
		},
		{
			name:          "bare-metal runner picks the default key",
			containerized: false,
			in: map[string][]string{
				setup.RShellPathAllowMapDefaultKey:       {"/var/log", "/etc"},
				setup.RShellPathAllowMapContainerizedKey: {"/host/var/log"},
			},
			want: []string{"/var/log", "/etc"},
		},
		{
			name:          "containerized runner picks the containerized key",
			containerized: true,
			in: map[string][]string{
				setup.RShellPathAllowMapDefaultKey:       {"/var/log", "/etc"},
				setup.RShellPathAllowMapContainerizedKey: {"/host/var/log"},
			},
			want: []string{"/host/var/log"},
		},
		{
			name:          "bare-metal runner with only the containerized key → nil (kill-switch)",
			containerized: false,
			in: map[string][]string{
				setup.RShellPathAllowMapContainerizedKey: {"/host/var/log"},
			},
			want: nil,
		},
		{
			name:          "containerized runner with only the default key → nil (kill-switch)",
			containerized: true,
			in: map[string][]string{
				setup.RShellPathAllowMapDefaultKey: {"/var/log"},
			},
			want: nil,
		},
		{
			name:          "unknown keys are ignored",
			containerized: false,
			in: map[string][]string{
				setup.RShellPathAllowMapDefaultKey: {"/var/log"},
				"some_future_env":                  {"/some/path"},
			},
			want: []string{"/var/log"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.containerized {
				t.Setenv("DOCKER_DD_AGENT", "true")
			} else {
				os.Unsetenv("DOCKER_DD_AGENT")
			}
			got := selectBackendPathsFromEnv(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}
