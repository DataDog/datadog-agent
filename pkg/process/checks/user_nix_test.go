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
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestLookupUserWithId(t *testing.T) {
	t.Setenv("HOST_ETC", "")
	cfg := configmock.New(t)
	cfg.SetInTest("process_config.cache_lookupid", true)

	for _, tc := range []struct {
		name          string
		expectedUser  *user.User
		expectedError error
		ttl           time.Duration
	}{
		{
			name:         "user found",
			expectedUser: &user.User{Username: "steve"},
			ttl:          cache.NoExpiration,
		},
		{
			name:          "user not found",
			expectedError: user.UnknownUserIdError(0),
			ttl:           cache.NoExpiration,
		},
	} {
		const testUID = "0"
		t.Run(tc.name, func(t *testing.T) {
			p := NewLookupIDProbe(cfg)

			checkResult := func(u *user.User, err error) {
				t.Helper()

				if tc.expectedUser != nil {
					require.NoError(t, err)
					require.NotNil(t, u)
					assert.Equal(t, tc.expectedUser.Username, u.Username)
				} else {
					assert.Nil(t, u)
				}

				assert.ErrorIs(t, err, tc.expectedError)
			}

			checkCacheResult := func(res interface{}, ok bool) {
				t.Helper()

				assert.True(t, ok)
				switch v := res.(type) {
				case *user.User:
					assert.Equal(t, tc.expectedUser.Username, v.Username)
				case error:
					assert.ErrorIs(t, v, tc.expectedError)
				}
			}

			var timesCalled int
			p.lookupID = func(inputUID string) (*user.User, error) {
				// Make sure this function is called once despite the fact that we call `lookupIDWithCache`.
				// This should simulate a cache hit vs a miss.
				timesCalled++
				assert.Equal(t, 1, timesCalled)

				assert.Equal(t, testUID, inputUID)
				if tc.expectedError != nil {
					return nil, tc.expectedError
				}
				return tc.expectedUser, nil
			}

			checkResult(p.LookupID(testUID))
			checkCacheResult(p.lookupIDCache.Get(testUID))
			checkResult(p.LookupID(testUID))
		})
	}
}

func TestLookupIDPrefersHostPasswdBeforeCachedFallback(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "passwd"), []byte("host-user:x:123:123::/:/bin/sh\n"), 0o600)
	require.NoError(t, err)
	t.Setenv("HOST_ETC", dir)

	cfg := configmock.New(t)
	cfg.SetInTest("process_config.cache_lookupid", true)
	probe := NewLookupIDProbe(cfg)
	fallbackCalls := 0
	probe.lookupID = func(uid string) (*user.User, error) {
		fallbackCalls++
		return &user.User{Username: "fallback-user", Uid: uid}, nil
	}

	hostUser, err := probe.LookupID("123")
	require.NoError(t, err)
	assert.Equal(t, "host-user", hostUser.Username)
	assert.Equal(t, 0, fallbackCalls)

	for range 2 {
		fallbackUser, err := probe.LookupID("124")
		require.NoError(t, err)
		assert.Equal(t, "fallback-user", fallbackUser.Username)
	}
	assert.Equal(t, 1, fallbackCalls)
}

func TestLookupIDHostEntrySupersedesCachedFallback(t *testing.T) {
	for _, failedLookup := range []bool{false, true} {
		name := "cached username"
		if failedLookup {
			name = "cached error"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("HOST_ETC", dir)
			cfg := configmock.New(t)
			cfg.SetInTest("process_config.cache_lookupid", true)
			probe := NewLookupIDProbe(cfg)
			clkCache, clk := newTestHostPasswdCache()
			probe.hostPasswd = clkCache
			fallbackCalls := 0
			fallbackErr := errors.New("unknown user")
			probe.lookupID = func(string) (*user.User, error) {
				fallbackCalls++
				if failedLookup {
					return nil, fallbackErr
				}
				return &user.User{Username: "image-user"}, nil
			}

			for range 2 {
				u, err := probe.LookupID("123")
				if failedLookup {
					require.ErrorIs(t, err, fallbackErr)
				} else {
					require.NoError(t, err)
					assert.Equal(t, "image-user", u.Username)
				}
			}
			require.Equal(t, 1, fallbackCalls)

			writePasswd(t, dir, "host-user:x:123:123::/:/bin/sh\n")
			clk.Add(hostPasswdRefreshInterval)
			u, err := probe.LookupID("123")
			require.NoError(t, err)
			assert.Equal(t, "host-user", u.Username)
			assert.Equal(t, 1, fallbackCalls)
		})
	}
}

func TestLookupIDConfigSetting(t *testing.T) {
	t.Setenv("HOST_ETC", "")
	testLookupIDFunc := func(_ string) (*user.User, error) { return &user.User{Username: "jojo"}, nil }

	t.Run("enabled", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.SetInTest("process_config.cache_lookupid", true)

		p := NewLookupIDProbe(cfg)
		p.lookupID = testLookupIDFunc

		_, _ = p.LookupID("1234") // testLookupIDFunc should be called and "1234" added to the cache
		u, ok := p.lookupIDCache.Get("1234")
		require.True(t, ok)
		assert.Equal(t, "jojo", u.(*user.User).Username)
	})

	t.Run("disabled", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.SetInTest("process_config.cache_lookupid", false)

		p := NewLookupIDProbe(cfg)
		p.lookupID = testLookupIDFunc

		_, _ = p.LookupID("1234")
		_, ok := p.lookupIDCache.Get("1234")
		assert.False(t, ok)
	})
}
