// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package imageresolver

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type mockRoundTripper struct {
	callCount int
	responses map[string]*http.Response
	mu        sync.RWMutex
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount++
	resp, ok := m.responses[req.URL.String()]
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{},
			Body:       io.NopCloser(bytes.NewBufferString("")),
			Request:    req,
		}, nil
	}
	return resp, nil
}

func (m *mockRoundTripper) addImage(registry string, repository string, tag string, digest string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	url := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repository, tag)
	m.responses[url] = &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Docker-Content-Digest": []string{digest},
		},
		Body:    io.NopCloser(bytes.NewBufferString("")),
		Request: &http.Request{},
	}
}

func (m *mockRoundTripper) CallCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.callCount
}

func mockHTTPDigestCache(ttl time.Duration) (*httpDigestCache, *mockRoundTripper) {
	transport := &mockRoundTripper{
		responses: make(map[string]*http.Response),
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}
	cache := registryCache{
		"registry":      make(repositoryCache),
		"registry1":     make(repositoryCache),
		"registry2":     make(repositoryCache),
		"test-registry": make(repositoryCache),
	}
	c := httpDigestCache{
		cache: cache,
		ttl:   ttl,
		mu:    sync.RWMutex{},
		fetcher: &httpDigestFetcher{
			client: client,
		},
	}
	return &c, transport
}

func TestHttpDigestCache_Get_Success(t *testing.T) {
	tests := []struct {
		name              string
		ttl               time.Duration
		setupCache        func(*httpDigestCache)
		setupMock         func(*mockRoundTripper)
		repository        string
		tag               string
		expectedDigest    string
		expectedCallCount int
	}{
		{
			name: "cache_hit_unexpired",
			ttl:  100 * time.Minute,
			setupCache: func(cc *httpDigestCache) {
				cc.cache["test-registry"] = repositoryCache{
					"dd-lib-python-init": tagCache{
						"v1": {
							digest:     "sha256:aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000",
							whenCached: time.Now(),
						},
					},
				}
			},
			setupMock:         func(_ *mockRoundTripper) {},
			repository:        "dd-lib-python-init",
			tag:               "v1",
			expectedDigest:    "sha256:aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000",
			expectedCallCount: 0,
		},
		{
			name: "cache_hit_expired",
			ttl:  0 * time.Minute,
			setupCache: func(cc *httpDigestCache) {
				cc.cache["test-registry"] = repositoryCache{
					"dd-lib-python-init": tagCache{
						"v1": {
							digest:     "sha256:0000000000000000000000000000000000000000000000000000000000000000",
							whenCached: time.Now().Add(-1 * time.Minute),
						},
					},
				}
			},
			setupMock: func(m *mockRoundTripper) {
				m.addImage("test-registry", "dd-lib-python-init", "v1", "sha256:eeee1111eeee1111eeee1111eeee1111eeee1111eeee1111eeee1111eeee1111")
			},
			repository:        "dd-lib-python-init",
			tag:               "v1",
			expectedDigest:    "sha256:eeee1111eeee1111eeee1111eeee1111eeee1111eeee1111eeee1111eeee1111",
			expectedCallCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc, transport := mockHTTPDigestCache(tt.ttl)
			tt.setupCache(cc)
			tt.setupMock(transport)

			resolved, err := cc.get("test-registry", tt.repository, tt.tag)

			require.NoError(t, err, "Expected successful get")
			require.NotNil(t, resolved, "Expected non-nil resolved image")
			require.Equal(t, tt.expectedDigest, resolved)
			require.Equal(t, tt.expectedCallCount, transport.CallCount())
		})
	}
}

func TestHttpDigestCache_Get_Failure(t *testing.T) {
	cc, transport := mockHTTPDigestCache(1 * time.Minute)
	resolved, err := cc.get("test-registry", "dd-lib-python-init", "v1")

	require.Error(t, err, "Expected failed get")
	require.Empty(t, resolved, "Expected empty digest")
	require.Equal(t, 1, transport.CallCount())
}

func TestHttpDigestCache_Get_MultipleRepositories(t *testing.T) {
	cc, transport := mockHTTPDigestCache(5 * time.Minute)
	transport.addImage("registry1", "dd-lib-python-init", "v1", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	transport.addImage("registry2", "dd-lib-java-init", "v2", "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	resolved1, err1 := cc.get("registry1", "dd-lib-python-init", "v1")
	resolved2, err2 := cc.get("registry2", "dd-lib-java-init", "v2")

	require.NoError(t, err1, "Should fetch python lib")
	require.NoError(t, err2, "Should fetch java lib")
	require.Equal(t, "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", resolved1)
	require.Equal(t, "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", resolved2)
	require.Equal(t, 2, transport.CallCount(), "Should have fetched digest twice")
}

func TestHttpDigestCache_Get_SameRepoMultipleTags(t *testing.T) {
	cc, transport := mockHTTPDigestCache(5 * time.Minute)
	transport.addImage("registry", "dd-lib-python-init", "v1", "sha256:1111111111111111111111111111111111111111111111111111111111111111")
	transport.addImage("registry", "dd-lib-python-init", "v2", "sha256:2222222222222222222222222222222222222222222222222222222222222222")
	transport.addImage("registry", "dd-lib-python-init", "latest", "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")

	resolved1, err1 := cc.get("registry", "dd-lib-python-init", "v1")
	resolved2, err2 := cc.get("registry", "dd-lib-python-init", "v2")
	resolved3, err3 := cc.get("registry", "dd-lib-python-init", "latest")

	require.NoError(t, err1)
	require.NoError(t, err2)
	require.NoError(t, err3)
	require.Equal(t, "sha256:1111111111111111111111111111111111111111111111111111111111111111", resolved1)
	require.Equal(t, "sha256:2222222222222222222222222222222222222222222222222222222222222222", resolved2)
	require.Equal(t, "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", resolved3)
	require.Equal(t, 3, transport.CallCount(), "Should have fetched digest three times")
}

func TestHttpDigestCache_Get_ConcurrentCacheHit(t *testing.T) {
	cc, transport := mockHTTPDigestCache(5 * time.Minute)
	transport.addImage("registry", "repo", "v1", "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	resolved, err := cc.get("registry", "repo", "v1")
	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.Equal(t, 1, transport.CallCount(), "Should have made exactly one fetch to warm cache")

	var wg sync.WaitGroup
	const concurrency = 100
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resolved, err := cc.get("registry", "repo", "v1")
			require.NoError(t, err)
			require.NotNil(t, resolved)
			require.Equal(t, "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890", resolved)
		}()
	}
	wg.Wait()

	require.Equal(t, 1, transport.CallCount(), "Should still have only 1 fetch (all cache hits)")
}
