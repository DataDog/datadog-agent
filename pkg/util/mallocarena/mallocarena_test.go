// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package mallocarena

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentLimit(t *testing.T) {
	tests := []struct {
		name     string
		arenaMax string
		tunables string
		want     string // the override SetMax reports, empty when the environment pins nothing
	}{
		{name: "nothing set"},
		{name: "empty values are not a choice", arenaMax: "", tunables: ""},
		{name: "arena max", arenaMax: "4", want: "MALLOC_ARENA_MAX=4"},
		{name: "tunable alone", tunables: "glibc.malloc.arena_max=4", want: "GLIBC_TUNABLES glibc.malloc.arena_max=4"},
		{
			name:     "tunable among others",
			tunables: "glibc.malloc.tcache_count=0:glibc.malloc.arena_max=4",
			want:     "GLIBC_TUNABLES glibc.malloc.arena_max=4",
		},
		{name: "unrelated tunables", tunables: "glibc.malloc.tcache_count=0:glibc.elision.enable=1"},
		{name: "arena_test is a different knob", tunables: "glibc.malloc.arena_test=2"},
		{name: "name must match exactly", tunables: "glibc.malloc.arena_maximum=4"},
		{name: "valueless tunable", tunables: "glibc.malloc.arena_max"},
		// glibc discards these and falls back to 8*nproc, so they are not an operator's cap.
		{name: "zero is below the tunable minimum", arenaMax: "0"},
		{name: "negative is below the tunable minimum", arenaMax: "-1"},
		{name: "non-numeric", arenaMax: "unlimited"},
		{name: "zero tunable", tunables: "glibc.malloc.arena_max=0"},
		{name: "non-numeric tunable", tunables: "glibc.malloc.arena_max=unlimited"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(arenaMaxEnv, tc.arenaMax)
			t.Setenv(tunablesEnv, tc.tunables)

			override, ok := environmentLimit()
			assert.Equal(t, tc.want != "", ok)
			assert.Equal(t, tc.want, override)
		})
	}
}

func TestSetMaxDefersToEnvironment(t *testing.T) {
	t.Setenv(arenaMaxEnv, "4")
	t.Setenv(tunablesEnv, "")

	summary, err := SetMax()
	require.NoError(t, err)
	assert.Contains(t, summary, "left to the environment")
	assert.Contains(t, summary, "MALLOC_ARENA_MAX=4")
}

// A value glibc itself rejects is not an override, so the cap still applies.
func TestSetMaxIgnoresRejectedEnvironment(t *testing.T) {
	t.Setenv(arenaMaxEnv, "0")
	t.Setenv(tunablesEnv, "")

	summary, err := SetMax()
	require.NoError(t, err)
	assert.NotContains(t, summary, "left to the environment")
}

// SetMax must never fail the caller: an unsupported build reports the fact and returns nil.
// TestSetMaxLimitsArenas covers the glibc path, where the cap actually binds.
func TestSetMaxWithoutOverride(t *testing.T) {
	t.Setenv(arenaMaxEnv, "")
	t.Setenv(tunablesEnv, "")

	summary, err := SetMax()
	require.NoError(t, err)
	assert.NotEmpty(t, summary)
}
