// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package imageresolver

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// To mock the crane calls
type mockDigestFetcher struct {
	digestMap map[string]string
	callCount int
}

func newMockDigestFetcher() *mockDigestFetcher {
	return &mockDigestFetcher{
		digestMap: make(map[string]string),
		callCount: 0,
	}
}

func (m *mockDigestFetcher) Digest(ref string) (string, error) {
	m.callCount++
	if digest, exists := m.digestMap[ref]; exists {
		return digest, nil
	}
	return "", fmt.Errorf("image not found: %s", ref)
}

func (m *mockDigestFetcher) addImage(ref, digest string) {
	m.digestMap[ref] = digest
}

func mockCraneCache(ttl time.Duration) (*craneCache, *mockDigestFetcher) {
	fetcher := newMockDigestFetcher()
	cc := &craneCache{
		cache:   make(map[string]map[string]CacheEntry),
		ttl:     ttl,
		mu:      sync.RWMutex{},
		fetcher: fetcher,
	}
	return cc, fetcher
}

func TestCraneCache_Get_Success(t *testing.T) {
	tests := []struct {
		name              string
		ttl               time.Duration
		setupCache        func(*craneCache)
		setupMock         func(*mockDigestFetcher)
		repository        string
		tag               string
		expectedDigest    string
		expectedCallCount int
	}{
		{
			name: "cache_hit_unexpired",
			ttl:  100 * time.Minute,
			setupCache: func(cc *craneCache) {
				cc.cache["dd-lib-python-init"] = map[string]CacheEntry{
					"v1": {
						ResolvedImage: &ResolvedImage{
							FullImageRef:     "test-registry/dd-lib-python-init@sha256:cacheddigest",
							CanonicalVersion: "v1",
						},
						WhenCached: time.Now(),
					},
				}
			},
			setupMock:         func(m *mockDigestFetcher) {},
			repository:        "dd-lib-python-init",
			tag:               "v1",
			expectedDigest:    "test-registry/dd-lib-python-init@sha256:cacheddigest",
			expectedCallCount: 0,
		},
		{
			name: "cache_hit_expired",
			ttl:  0 * time.Minute,
			setupCache: func(cc *craneCache) {
				cc.cache["dd-lib-python-init"] = map[string]CacheEntry{
					"v1": {
						ResolvedImage: &ResolvedImage{
							FullImageRef:     "test-registry/dd-lib-python-init@sha256:olddigest",
							CanonicalVersion: "v1",
						},
						WhenCached: time.Now().Add(-1 * time.Minute),
					},
				}
			},
			setupMock: func(m *mockDigestFetcher) {
				m.addImage("test-registry/dd-lib-python-init:v1", "sha256:newdigest")
			},
			repository:        "dd-lib-python-init",
			tag:               "v1",
			expectedDigest:    "test-registry/dd-lib-python-init@sha256:newdigest",
			expectedCallCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc, fetcher := mockCraneCache(tt.ttl)
			tt.setupCache(cc)
			tt.setupMock(fetcher)

			resolved, ok := cc.Get("test-registry", tt.repository, tt.tag)

			require.True(t, ok, "Expected successful Get")
			require.NotNil(t, resolved, "Expected non-nil resolved image")
			require.Equal(t, tt.expectedDigest, resolved.FullImageRef)
			require.Equal(t, tt.tag, resolved.CanonicalVersion)
			require.Equal(t, tt.expectedCallCount, fetcher.callCount)
		})
	}
}

func TestCraneCache_Get_Failure(t *testing.T) {
	cc, fetcher := mockCraneCache(1 * time.Minute)
	resolved, ok := cc.Get("test-registry", "dd-lib-python-init", "v1")

	require.False(t, ok, "Expected failed Get")
	require.Nil(t, resolved, "Expected nil resolved image")
	require.Equal(t, 1, fetcher.callCount)
}

func TestCraneCache_Get_MultipleRepositories(t *testing.T) {
	cc, fetcher := mockCraneCache(5 * time.Minute)
	fetcher.addImage("registry1/dd-lib-python-init:v1", "sha256:pythondigest")
	fetcher.addImage("registry2/dd-lib-java-init:v2", "sha256:javadigest")

	resolved1, ok1 := cc.Get("registry1", "dd-lib-python-init", "v1")
	resolved2, ok2 := cc.Get("registry2", "dd-lib-java-init", "v2")

	require.True(t, ok1, "Should fetch python lib")
	require.True(t, ok2, "Should fetch java lib")
	require.Equal(t, "registry1/dd-lib-python-init@sha256:pythondigest", resolved1.FullImageRef)
	require.Equal(t, "registry2/dd-lib-java-init@sha256:javadigest", resolved2.FullImageRef)
	require.Equal(t, 2, fetcher.callCount, "Should have called Digest twice")
}

func TestCraneCache_Get_SameRepoMultipleTags(t *testing.T) {
	cc, fetcher := mockCraneCache(5 * time.Minute)
	fetcher.addImage("registry/dd-lib-python-init:v1", "sha256:digestv1")
	fetcher.addImage("registry/dd-lib-python-init:v2", "sha256:digestv2")
	fetcher.addImage("registry/dd-lib-python-init:latest", "sha256:digestlatest")

	resolved1, ok1 := cc.Get("registry", "dd-lib-python-init", "v1")
	resolved2, ok2 := cc.Get("registry", "dd-lib-python-init", "v2")
	resolved3, ok3 := cc.Get("registry", "dd-lib-python-init", "latest")

	require.True(t, ok1)
	require.True(t, ok2)
	require.True(t, ok3)
	require.Equal(t, "registry/dd-lib-python-init@sha256:digestv1", resolved1.FullImageRef)
	require.Equal(t, "registry/dd-lib-python-init@sha256:digestv2", resolved2.FullImageRef)
	require.Equal(t, "registry/dd-lib-python-init@sha256:digestlatest", resolved3.FullImageRef)
	require.Equal(t, 3, fetcher.callCount, "Should have called Digest three times")
}

func TestCraneCache_Get_Concurrent(t *testing.T) {
	cc, fetcher := mockCraneCache(5 * time.Minute)
	fetcher.addImage("registry/repo:v1", "sha256:digest")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cc.Get("registry", "repo", "v1")
		}()
	}
	wg.Wait()
	require.Equal(t, 1, fetcher.callCount)
}
