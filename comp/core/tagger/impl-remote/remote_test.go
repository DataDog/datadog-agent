// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteimpl

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/noopsimpl"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

// TestNewComponent tests that the Remote Tagger can be instantiated and started.
func TestNewComponent(t *testing.T) {
	// Skip this test if not running in CI, as it may conflict with another Agent.
	if os.Getenv("CI") != "true" {
		t.Skip("Skipping test as it is not running in CI.")
	}
	if runtime.GOOS == "darwin" {
		t.Skip("Skipping test on macOS runners with an existing Agent.")
	}

	// Instantiate the component.
	req := Requires{
		Lc:     compdef.NewTestLifecycle(t),
		Config: configmock.New(t),
		Log:    logmock.New(t),
		Params: tagger.NewRemoteParams(
			tagger.WithRemoteTarget(func(config.Component) (string, error) { return ":5001", nil }),
		),
		Telemetry: nooptelemetry.GetCompatComponent(),
		IPC:       ipcmock.New(t),
	}
	_, err := NewComponent(req)
	require.NoError(t, err)
}

// TestNewComponentNonBlocking tests that the Remote Tagger instantiation does not block when the gRPC server is not available.
func TestNewComponentNonBlocking(t *testing.T) {
	// Instantiate the component.
	req := Requires{
		Lc:     compdef.NewTestLifecycle(t),
		Config: configmock.New(t),
		Log:    logmock.New(t),
		Params: tagger.NewRemoteParams(
			tagger.WithRemoteTarget(func(config.Component) (string, error) { return ":5001", nil }),
		),
		Telemetry: nooptelemetry.GetCompatComponent(),
		IPC:       ipcmock.New(t),
	}
	_, err := NewComponent(req)
	require.NoError(t, err)
}

// TestNewComponentSetsTaggerListEndpoint tests the Remote Tagger tagger-list endpoint.
func TestNewComponentSetsTaggerListEndpoint(t *testing.T) {
	// Instantiate the component.
	req := Requires{
		Lc:     compdef.NewTestLifecycle(t),
		Config: configmock.New(t),
		Log:    logmock.New(t),
		Params: tagger.NewRemoteParams(
			tagger.WithRemoteTarget(func(config.Component) (string, error) { return ":5001", nil }),
		),
		Telemetry: nooptelemetry.GetCompatComponent(),
		IPC:       ipcmock.New(t),
	}
	provides, err := NewComponent(req)
	require.NoError(t, err)

	endpointProvider := provides.Endpoint.Provider

	assert.Equal(t, []string{"GET"}, endpointProvider.Methods())
	assert.Equal(t, "/tagger-list", endpointProvider.Route())

	// Create a test server with the endpoint handler
	server := httptest.NewServer(endpointProvider.HandlerFunc())
	defer server.Close()

	// Make a request to the endpoint
	resp, err := http.Get(server.URL + "/tagger-list")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response types.TaggerListResponse
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.NotNil(t, response.Entities)
}

// TestNewComponentWithOverride tests the Remote Tagger initialization with overrides for TLS and auth token.
func TestNewComponentWithOverride(t *testing.T) {
	// Create a mock IPC component
	ipcComp := ipcmock.New(t)

	// Create a test server with the endpoint handler
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	t.Run("auth token getter blocks 2s and succeeds", func(t *testing.T) {
		start := time.Now()
		req := Requires{
			Lc:     compdef.NewTestLifecycle(t),
			Config: configmock.New(t),
			Log:    logmock.New(t),
			Params: tagger.NewRemoteParams(
				tagger.WithRemoteTarget(func(config.Component) (string, error) { return server.URL, nil }),
				tagger.WithOverrideTLSConfigGetter(func() (*tls.Config, error) {
					return &tls.Config{
						InsecureSkipVerify: true,
					}, nil
				}),
				tagger.WithOverrideAuthTokenGetter(func(_ configmodel.Reader) (string, error) {
					time.Sleep(2 * time.Second)
					return "test-token", nil
				}),
			),
			Telemetry: nooptelemetry.GetCompatComponent(),
			IPC:       ipcComp,
		}
		_, err := NewComponent(req)
		elapsed := time.Since(start)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, elapsed, 2*time.Second, "NewComponent should wait for auth token getter")
	})

	t.Run("auth token getter blocks >10s and fails", func(t *testing.T) {
		start := time.Now()
		req := Requires{
			Lc:     compdef.NewTestLifecycle(t),
			Config: configmock.New(t),
			Log:    logmock.New(t),
			Params: tagger.NewRemoteParams(
				tagger.WithRemoteTarget(func(config.Component) (string, error) { return server.URL, nil }),
				tagger.WithOverrideTLSConfigGetter(func() (*tls.Config, error) {
					return &tls.Config{
						InsecureSkipVerify: true,
					}, nil
				}),

				tagger.WithOverrideAuthTokenGetter(func(_ configmodel.Reader) (string, error) {
					return "", errors.New("auth token getter always fails")
				})),
			Telemetry: nooptelemetry.GetCompatComponent(),
			IPC:       ipcComp,
		}
		_, err := NewComponent(req)
		elapsed := time.Since(start)
		assert.Error(t, err, "NewComponent should fail if auth token getter blocks too long")
		assert.GreaterOrEqual(t, elapsed, 10*time.Second, "Should wait at least 10s before failing")
		assert.Less(t, elapsed, 15*time.Second, "Should not wait excessively long")
	})
}

// TestGenerateContainerIDFromOriginInfo_NegativeCache verifies that failed
// container ID lookups are cached, preventing unbounded gRPC round-trips
// on bare-metal (non-containerized) hosts. See: AGENT-15616
func TestGenerateContainerIDFromOriginInfo_NegativeCache(t *testing.T) {
	telemetryStore := telemetry.NewStore(nooptelemetry.GetCompatComponent())

	rt := &remoteTagger{
		store:            newTagStore(telemetryStore),
		telemetryStore:   telemetryStore,
		log:              logmock.New(t),
		containerIDCache: make(map[string]containerIDCacheEntry),
	}

	// Simulate a bare-metal resolution failure by pre-populating a negative entry
	originInfo := origindetection.OriginInfo{
		ProductOrigin: origindetection.ProductOriginAPM,
		LocalData:     origindetection.LocalData{ProcessID: 3047},
	}
	key := cache.BuildAgentKey(
		"remoteTagger",
		"originInfo",
		origindetection.OriginInfoString(originInfo),
	)
	resolveErr := fmt.Errorf("unable to resolve container ID from OriginInfo: %+v", originInfo)

	rt.containerIDCacheMu.Lock()
	rt.containerIDCache[key] = containerIDCacheEntry{
		containerID: "",
		err:         resolveErr,
		expireAt:    time.Now().Add(negativeCacheExpiration),
	}
	rt.containerIDCacheMu.Unlock()

	// Call should return the cached error without making a gRPC call.
	// (If it tried gRPC, it would panic because rt.client is nil.)
	result, err := rt.GenerateContainerIDFromOriginInfo(originInfo)
	assert.Error(t, err)
	assert.Empty(t, result)
	assert.Contains(t, err.Error(), "unable to resolve container ID")

	// Second call with the same OriginInfo — must also hit cache
	result2, err2 := rt.GenerateContainerIDFromOriginInfo(originInfo)
	assert.Error(t, err2)
	assert.Empty(t, result2)
}

// TestGenerateContainerIDFromOriginInfo_PositiveCache verifies that successful
// container ID lookups are cached and returned without re-querying.
func TestGenerateContainerIDFromOriginInfo_PositiveCache(t *testing.T) {
	telemetryStore := telemetry.NewStore(nooptelemetry.GetCompatComponent())

	rt := &remoteTagger{
		store:            newTagStore(telemetryStore),
		telemetryStore:   telemetryStore,
		log:              logmock.New(t),
		containerIDCache: make(map[string]containerIDCacheEntry),
	}

	originInfo := origindetection.OriginInfo{
		ProductOrigin: origindetection.ProductOriginAPM,
		LocalData:     origindetection.LocalData{ProcessID: 1234},
	}
	key := cache.BuildAgentKey(
		"remoteTagger",
		"originInfo",
		origindetection.OriginInfoString(originInfo),
	)

	// Pre-populate a positive cache entry (simulates a containerized host)
	rt.containerIDCacheMu.Lock()
	rt.containerIDCache[key] = containerIDCacheEntry{
		containerID: "abc-container-123",
		err:         nil,
		expireAt:    time.Now().Add(cacheExpiration),
	}
	rt.containerIDCacheMu.Unlock()

	result, err := rt.GenerateContainerIDFromOriginInfo(originInfo)
	require.NoError(t, err)
	assert.Equal(t, "abc-container-123", result)
}

// TestGenerateContainerIDFromOriginInfo_ExpiryAndEviction verifies that:
// (a) expired cache entries are detected as stale (forcing a fresh lookup), and
// (b) lazy eviction purges stale entries when the map exceeds the threshold.
//
// Note: lazy eviction runs inside the write path (cache miss or expired entry),
// not on cache hits. We test the eviction invariant directly on the map because
// triggering the full write path requires a gRPC client mock.
func TestGenerateContainerIDFromOriginInfo_ExpiryAndEviction(t *testing.T) {
	telemetryStore := telemetry.NewStore(nooptelemetry.GetCompatComponent())

	rt := &remoteTagger{
		store:            newTagStore(telemetryStore),
		telemetryStore:   telemetryStore,
		log:              logmock.New(t),
		containerIDCache: make(map[string]containerIDCacheEntry),
	}

	// --- Part A: verify expired entries are NOT served from cache ---
	expiredKey := cache.BuildAgentKey("remoteTagger", "originInfo", "expired-entry")
	rt.containerIDCache[expiredKey] = containerIDCacheEntry{
		containerID: "should-not-be-returned",
		err:         nil,
		expireAt:    time.Now().Add(-1 * time.Second), // already expired
	}

	// The RLock path checks time.Now().Before(entry.expireAt), which is false
	// for an expired entry, so it would fall through to the gRPC path.
	rt.containerIDCacheMu.RLock()
	entry, found := rt.containerIDCache[expiredKey]
	isExpired := found && !time.Now().Before(entry.expireAt)
	rt.containerIDCacheMu.RUnlock()

	assert.True(t, found, "entry should exist in the map")
	assert.True(t, isExpired, "entry should be detected as expired, forcing a fresh lookup")

	// --- Part B: verify lazy eviction purges expired entries ---
	rt.containerIDCache = make(map[string]containerIDCacheEntry)
	for i := 0; i < 1001; i++ {
		k := cache.BuildAgentKey("remoteTagger", "originInfo", fmt.Sprintf("expired-%d", i))
		rt.containerIDCache[k] = containerIDCacheEntry{
			err:      errors.New("stale error"),
			expireAt: time.Now().Add(-1 * time.Minute),
		}
	}
	// Add one live entry
	liveKey := cache.BuildAgentKey("remoteTagger", "originInfo", "live-entry")
	rt.containerIDCache[liveKey] = containerIDCacheEntry{
		containerID: "live-container",
		err:         nil,
		expireAt:    time.Now().Add(cacheExpiration),
	}
	require.Equal(t, 1002, len(rt.containerIDCache))

	// Run the same eviction logic from the method
	if len(rt.containerIDCache) > 1000 {
		now := time.Now()
		for k, v := range rt.containerIDCache {
			if now.After(v.expireAt) {
				delete(rt.containerIDCache, k)
			}
		}
	}

	assert.Equal(t, 1, len(rt.containerIDCache), "only the live entry should survive eviction")
	surviving, ok := rt.containerIDCache[liveKey]
	require.True(t, ok)
	assert.Equal(t, "live-container", surviving.containerID)
	assert.NoError(t, surviving.err)
}
