// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package checks

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestLookupUserWithId(t *testing.T) {
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
					assert.Equal(t, tc.expectedUser.Username, u.Username)
				} else {
					assert.Nil(t, tc.expectedUser)
				}

				assert.ErrorIs(t, tc.expectedError, err)
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
	assert.NoError(t, err)
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
	assert.NoError(t, err)
	assert.Equal(t, "host-user", hostUser.Username)
	assert.Equal(t, 0, fallbackCalls)

	for range 2 {
		fallbackUser, err := probe.LookupID("124")
		assert.NoError(t, err)
		assert.Equal(t, "fallback-user", fallbackUser.Username)
	}
	assert.Equal(t, 1, fallbackCalls)
}

func TestLookupIDConfigSetting(t *testing.T) {
	testLookupIDFunc := func(_ string) (*user.User, error) { return &user.User{Username: "jojo"}, nil }

	t.Run("enabled", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.SetInTest("process_config.cache_lookupid", true)

		p := NewLookupIDProbe(cfg)
		p.lookupID = testLookupIDFunc

		_, _ = p.LookupID("1234") // testLookupIDFunc should be called and "1234" added to the cache
		u, ok := p.lookupIDCache.Get("1234")
		assert.Equal(t, "jojo", u.(*user.User).Username)
		assert.True(t, ok)
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
