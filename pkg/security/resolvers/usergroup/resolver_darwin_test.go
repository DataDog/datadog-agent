// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package usergroup

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRoot(t *testing.T) {
	r, err := NewResolver()
	require.NoError(t, err)

	user, err := r.ResolveUser(0)
	require.NoError(t, err)
	assert.Equal(t, "root", user)

	group, err := r.ResolveGroup(0)
	require.NoError(t, err)
	assert.Equal(t, "wheel", group, "gid 0 is wheel on macOS, not root")
}

// TestResolveCurrentUser is the real check. macOS keeps console users in Open
// Directory, not /etc/passwd, so the Linux resolver's /etc/passwd parsing would
// find nothing useful here.
func TestResolveCurrentUser(t *testing.T) {
	r, err := NewResolver()
	require.NoError(t, err)

	user, err := r.ResolveUser(os.Getuid())
	require.NoError(t, err)
	assert.NotEmpty(t, user, "the current user must resolve via Open Directory")

	group, err := r.ResolveGroup(os.Getgid())
	require.NoError(t, err)
	assert.NotEmpty(t, group)
}

func TestResolveUnknownUID(t *testing.T) {
	r, err := NewResolver()
	require.NoError(t, err)

	_, err = r.ResolveUser(999999)
	assert.Error(t, err, "an unknown uid must error rather than return empty")

	_, err = r.ResolveGroup(999999)
	assert.Error(t, err, "an unknown gid must error rather than return empty")
}

// TestResolveIsCached guards the reason the cache exists: every exec event
// resolves a uid, so an uncached lookup would mean a getpwuid_r per event.
func TestResolveIsCached(t *testing.T) {
	r, err := NewResolver()
	require.NoError(t, err)

	first, err := r.ResolveUser(0)
	require.NoError(t, err)

	// Poison the cache to prove the second call does not re-query.
	r.usersCache.Add(0, "sentinel-from-cache")

	second, err := r.ResolveUser(0)
	require.NoError(t, err)

	assert.Equal(t, "root", first)
	assert.Equal(t, "sentinel-from-cache", second, "a second lookup must come from the cache")
}
