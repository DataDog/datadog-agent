// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !serverless

package hostname

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

func TestDetermineDriftState(t *testing.T) {
	tests := []struct {
		name     string
		oldData  Data
		newData  Data
		expected driftInfo
	}{
		{
			name: "no drift",
			oldData: Data{
				Hostname: "host1",
				Provider: "provider1",
			},
			newData: Data{
				Hostname: "host1",
				Provider: "provider1",
			},
			expected: driftInfo{
				state:    noDrift,
				hasDrift: false,
			},
		},
		{
			name: "hostname changed only",
			oldData: Data{
				Hostname: "host1",
				Provider: "provider1",
			},
			newData: Data{
				Hostname: "host2",
				Provider: "provider1",
			},
			expected: driftInfo{
				state:    hostnameChanged,
				hasDrift: true,
			},
		},
		{
			name: "provider changed only",
			oldData: Data{
				Hostname: "host1",
				Provider: "provider1",
			},
			newData: Data{
				Hostname: "host1",
				Provider: "provider2",
			},
			expected: driftInfo{
				state:    providerChanged,
				hasDrift: true,
			},
		},
		{
			name: "both hostname and provider changed",
			oldData: Data{
				Hostname: "host1",
				Provider: "provider1",
			},
			newData: Data{
				Hostname: "host2",
				Provider: "provider2",
			},
			expected: driftInfo{
				state:    hostnameProviderChanged,
				hasDrift: true,
			},
		},
		{
			name: "empty hostnames",
			oldData: Data{
				Hostname: "",
				Provider: "provider1",
			},
			newData: Data{
				Hostname: "",
				Provider: "provider1",
			},
			expected: driftInfo{
				state:    noDrift,
				hasDrift: false,
			},
		},
		{
			name: "empty providers",
			oldData: Data{
				Hostname: "host1",
				Provider: "",
			},
			newData: Data{
				Hostname: "host1",
				Provider: "",
			},
			expected: driftInfo{
				state:    noDrift,
				hasDrift: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineDriftState(tt.oldData, tt.newData)
			assert.Equal(t, tt.expected.state, result.state)
			assert.Equal(t, tt.expected.hasDrift, result.hasDrift)
		})
	}
}

func TestScheduleHostnameDriftChecks(t *testing.T) {
	// Clear cache before test
	cacheHostnameKey := cache.BuildAgentKey("hostname_check")
	cache.Cache.Delete(cacheHostnameKey)

	// Create test data
	hostnameData := Data{
		Hostname: "test-hostname",
		Provider: "test-provider",
	}

	// Create a drift service with shorter intervals for testing
	ds := driftService{
		initialDelay:      10 * time.Millisecond,
		recurringInterval: 50 * time.Millisecond,
	}

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Schedule the drift checks
	ds.scheduleHostnameDriftChecks(ctx, hostnameData)

	// Verify that the initial data was cached
	cachedData, found := cache.Cache.Get(cacheHostnameKey)
	require.True(t, found, "Expected hostname data to be cached")

	cachedHostnameData, ok := cachedData.(Data)
	require.True(t, ok, "Expected cached data to be of type Data")
	assert.Equal(t, hostnameData.Hostname, cachedHostnameData.Hostname)
	assert.Equal(t, hostnameData.Provider, cachedHostnameData.Provider)

	// Verify that telemetry metrics were created (they should exist even if we can't access them directly in tests)
	// The telemetry metrics are created as global variables in drift.go, so they should be available
	assert.NotNil(t, tlmDriftDetected, "Expected drift_detected telemetry metric to be created")
	assert.NotNil(t, tlmDriftResolutionTime, "Expected drift_resolution_time_ms telemetry metric to be created")

	// Cancel the context to stop the goroutine
	cancel()

	// Give some time for the goroutine to clean up
	time.Sleep(10 * time.Millisecond)
}

// scrapeTelemetry renders the process-wide telemetry registry as Prometheus text, the same way
// `agent diagnose show-metadata agent-full-telemetry` does, so tests can assert on emitted labels.
func scrapeTelemetry(t *testing.T) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	telemetryimpl.GetCompatComponent().Handler().ServeHTTP(rec, req)
	return rec.Body.String()
}

// TestCheckHostnameDriftEmitsTelemetry replaces the deleted hostname_drift E2E suite. It goes
// through GetWithProvider (not checkHostnameDrift directly) because scheduleHostnameDriftChecks is
// only reached via getHostname's fallthrough/"coupled" provider chain (fqdn/container/os/aws) —
// early-stop providers return before that call. The EC2-fallthrough fixture below reproduces that
// path deterministically.
func TestCheckHostnameDriftEmitsTelemetry(t *testing.T) {
	setupHostnameTest(t, testCase{
		name:             "hostname from EC2 with default system name",
		FQDNEC2:          true,
		OSEC2:            true,
		EC2:              true,
		expectedHostname: "hostname-from-ec2",
		expectedProvider: "aws",
	})

	cfg := configmock.New(t) // idempotent; reuses setupHostnameTest's mock
	cfg.SetInTest("hostname_drift_initial_delay", "1ms")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(func() { cache.Cache.Delete(cache.BuildAgentKey("hostname_check")) })

	_, err := GetWithProvider(ctx)
	require.NoError(t, err)

	// Metric name has a double underscore and labels are alphabetical (provider before state) —
	// see telemetry.Options.NameWithSeparator and Prometheus's exposition format.
	require.Eventually(t, func() bool {
		body := scrapeTelemetry(t)
		return strings.Contains(body, "hostname__drift_resolution_time_ms_count{") &&
			strings.Contains(body, `provider="aws"`) &&
			strings.Contains(body, `state="`+noDrift+`"`)
	}, time.Second, 5*time.Millisecond,
		"drift telemetry with provider=aws, state=no_drift should be scheduled and emitted")
}

// TestCheckHostnameDriftDetectsHostnameChange covers the actual drift path: EC2 metadata reports a
// different hostname on the next check. checkHostnameDrift is called synchronously here (the
// sibling test above already proves the scheduler reaches it) so there's no timer to race and no
// window between the telemetry Inc and the cache update to observe mid-flight.
func TestCheckHostnameDriftDetectsHostnameChange(t *testing.T) {
	setupHostnameTest(t, testCase{
		name:             "hostname from EC2 with default system name",
		FQDNEC2:          true,
		OSEC2:            true,
		EC2:              true,
		expectedHostname: "hostname-from-ec2",
		expectedProvider: "aws",
	})

	cacheHostnameKey := cache.BuildAgentKey("hostname_check")
	t.Cleanup(func() { cache.Cache.Delete(cacheHostnameKey) })

	initial, err := GetWithProvider(context.Background())
	require.NoError(t, err)
	require.Equal(t, "aws", initial.Provider)

	prevEC2GetInstanceID := ec2GetInstanceID
	t.Cleanup(func() { ec2GetInstanceID = prevEC2GetInstanceID })
	ec2GetInstanceID = func(context.Context) (string, error) { return "hostname-from-ec2-v2", nil }

	(&driftService{}).checkHostnameDrift(context.Background(), cacheHostnameKey)

	body := scrapeTelemetry(t)
	assert.Contains(t, body, `hostname__drift_detected{provider="aws",state="`+hostnameChanged+`"} 1`,
		"drift_detected should increment once the re-resolved hostname disagrees with the cached one")

	cachedData, found := cache.Cache.Get(cacheHostnameKey)
	require.True(t, found)
	assert.Equal(t, Data{Hostname: "hostname-from-ec2-v2", Provider: "aws"}, cachedData,
		"cache should be updated to the newly detected hostname after drift")
}
