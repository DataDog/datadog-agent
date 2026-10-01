// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package checks

import (
	"errors"
	"os"
	"os/user"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestLookupIDCacheSetting(t *testing.T) {
	t.Setenv("HOST_ETC", "")
	lookupErr := errors.New("unknown user")
	for _, tc := range []struct {
		name       string
		cache      bool
		initialErr error
	}{
		{name: "cached user", cache: true},
		{name: "uncached user"},
		{name: "cached error", cache: true, initialErr: lookupErr},
		{name: "uncached error", initialErr: lookupErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.SetInTest("process_config.cache_lookupid", tc.cache)
			probe := NewLookupIDProbe(cfg)
			initialUser := &user.User{Username: "before"}
			if tc.initialErr != nil {
				initialUser = nil
			}
			updatedUser := &user.User{Username: "after"}
			calls := 0
			probe.lookupID = func(string) (*user.User, error) {
				calls++
				if calls == 1 {
					return initialUser, tc.initialErr
				}
				return updatedUser, nil
			}

			u, err := probe.LookupID("123")
			require.ErrorIs(t, err, tc.initialErr)
			require.Equal(t, initialUser, u)

			u, err = probe.LookupID("123")
			if tc.cache {
				require.ErrorIs(t, err, tc.initialErr)
				assert.Equal(t, initialUser, u)
			} else {
				require.NoError(t, err)
				assert.Equal(t, updatedUser, u)
			}
		})
	}
}

func TestLookupIDHostPasswdCacheSetting(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cache    bool
		expected string
	}{
		{name: "cached", cache: true, expected: "before"},
		{name: "uncached", expected: "after"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writePasswd(t, dir, "before:x:42:4::/:/bin/sh\n")
			t.Setenv("HOST_ETC", dir)
			cfg := configmock.New(t)
			cfg.SetInTest("process_config.cache_lookupid", tc.cache)
			probe := NewLookupIDProbe(cfg)
			probe.lookupID = func(string) (*user.User, error) {
				return &user.User{Username: "local-user"}, nil
			}

			u, err := probe.LookupID("42")
			require.NoError(t, err)
			require.Equal(t, "before", u.Username)

			writePasswd(t, dir, "after:x:42:4::/:/bin/sh\n")
			u, err = probe.LookupID("42")
			require.NoError(t, err)
			assert.Equal(t, tc.expected, u.Username)
		})
	}
}

func TestLookupIDCachedFallbackPrecedesNewHostEntry(t *testing.T) {
	for _, tc := range []struct {
		name string
		user *user.User
		err  error
	}{
		{name: "cached username", user: &user.User{Username: "local-user"}},
		{name: "cached error", err: errors.New("unknown user")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOST_ETC", dir)
			cfg := configmock.New(t)
			cfg.SetInTest("process_config.cache_lookupid", true)
			probe := NewLookupIDProbe(cfg)
			probe.lookupID = func(string) (*user.User, error) {
				return tc.user, tc.err
			}

			u, err := probe.LookupID("123")
			require.ErrorIs(t, err, tc.err)
			require.Equal(t, tc.user, u)

			writePasswd(t, dir, "host-user:x:123:123::/:/bin/sh\n")
			u, err = probe.LookupID("123")
			require.ErrorIs(t, err, tc.err)
			assert.Equal(t, tc.user, u)

			// Disabling the existing cache must bypass cached results and errors.
			cfg.SetInTest("process_config.cache_lookupid", false)
			u, err = probe.LookupID("123")
			require.NoError(t, err)
			assert.Equal(t, "host-user", u.Username)
		})
	}
}

func TestLookupIDHostReadFailureFallsBackAndRecovers(t *testing.T) {
	dir := t.TempDir()
	writePasswd(t, dir, "before:x:42:4::/:/bin/sh\n")
	t.Setenv("HOST_ETC", dir)
	cfg := configmock.New(t)
	cfg.SetInTest("process_config.cache_lookupid", false)
	probe := NewLookupIDProbe(cfg)
	probe.lookupID = func(string) (*user.User, error) {
		return &user.User{Username: "local-user"}, nil
	}

	u, err := probe.LookupID("42")
	require.NoError(t, err)
	require.Equal(t, "before", u.Username)

	passwdPath := filepath.Join(dir, "passwd")
	require.NoError(t, os.Remove(passwdPath))
	require.NoError(t, os.Mkdir(passwdPath, 0o700))
	u, err = probe.LookupID("42")
	require.NoError(t, err)
	assert.Equal(t, "local-user", u.Username)

	require.NoError(t, os.Remove(passwdPath))
	writePasswd(t, dir, "recovered:x:42:4::/:/bin/sh\n")
	u, err = probe.LookupID("42")
	require.NoError(t, err)
	assert.Equal(t, "recovered", u.Username)
}

func TestLookupIDReadsHostEtcAfterConstruction(t *testing.T) {
	t.Setenv("HOST_ETC", "")
	probe := NewLookupIDProbe(configmock.New(t))

	dir := t.TempDir()
	writePasswd(t, dir, "host-user:x:42:4::/:/bin/sh\n")
	t.Setenv("HOST_ETC", dir)

	u, err := probe.LookupID("42")
	require.NoError(t, err)
	assert.Equal(t, "host-user", u.Username)
}
