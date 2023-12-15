// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package checks

import (
	"os/user"
	"testing"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestLookupUserWithId(t *testing.T) {
	cfg := config.Mock(t)
	cfg.SetWithoutSource("process_config.cache_lookupid", true)

	for _, tc := range []struct {
		name          string
		expectedUser  *user.User
		expectedError error
		ttl           time.Duration
	}{
		{
			name:         "user found",
			expectedUser: &user.User{Name: "steve"},
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
					assert.Equal(t, tc.expectedUser.Name, u.Name)
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
					assert.Equal(t, tc.expectedUser.Name, v.Name)
				case error:
					assert.ErrorIs(t, v, tc.expectedError)
				}
			}

			var timesCalled int
			p.lookupId = func(inputUid string) (*user.User, error) {
				// Make sure this function is called once despite the fact that we call `lookupIdWithCache`.
				// This should simulate a cache hit vs a miss.
				timesCalled++
				assert.Equal(t, 1, timesCalled)

				assert.Equal(t, testUID, inputUid)
				if tc.expectedError != nil {
					return nil, tc.expectedError
				}
				return tc.expectedUser, nil
			}

			checkResult(p.LookupId(testUID))
			checkCacheResult(p.lookupIdCache.Get(testUID))
			checkResult(p.LookupId(testUID))
		})
	}
}

func TestLookupIdConfigSetting(t *testing.T) {
	//nolint:revive // TODO(PROC) Fix revive linter
	testLookupIdFunc := func(uid string) (*user.User, error) { return &user.User{Name: "jojo"}, nil }

	t.Run("enabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.SetWithoutSource("process_config.cache_lookupid", true)

		p := NewLookupIDProbe(cfg)
		p.lookupId = testLookupIdFunc

		_, _ = p.LookupId("1234") // testLookupIdFunc should be called and "1234" added to the cache
		u, ok := p.lookupIdCache.Get("1234")
		assert.Equal(t, "jojo", u.(*user.User).Name)
		assert.True(t, ok)
	})

	t.Run("disabled", func(t *testing.T) {
		cfg := config.Mock(t)
		cfg.SetWithoutSource("process_config.cache_lookupid", false)

		p := NewLookupIDProbe(cfg)
		p.lookupId = testLookupIdFunc

		_, _ = p.LookupId("1234")
		_, ok := p.lookupIdCache.Get("1234")
		assert.False(t, ok)
	})
}
