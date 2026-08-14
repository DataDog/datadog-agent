// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !serverless

package hostname

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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

func TestCheckHostnameDriftEmitsTelemetry(t *testing.T) {
	cfg := configmock.New(t)
	cacheHostnameKey := cache.BuildAgentKey("hostname_check")

	// Seed the cache the way scheduleHostnameDriftChecks does at agent startup: the currently
	// resolved hostname/provider, before any check has run.
	cfg.SetInTest("hostname", "host-a")
	cache.Cache.Set(cacheHostnameKey, Data{Hostname: "host-a", Provider: configProviderName}, cache.NoExpiration)

	ds := driftService{}
	ds.checkHostnameDrift(context.Background(), cacheHostnameKey)

	// Prometheus's text exposition format (which the diagnose CLI ultimately renders) sorts label
	// names alphabetically, and telemetry.Options.NameWithSeparator inserts a leading "_" that
	// becomes a second underscore once joined with the "hostname" subsystem — hence
	// "hostname__drift_..." with labels ordered provider-then-state, not the declaration order.
	body := scrapeTelemetry(t)
	noDriftLabels := fmt.Sprintf(`provider="%s",state="%s"`, configProviderName, noDrift)
	assert.Contains(t, body, "hostname__drift_resolution_time_ms_count{"+noDriftLabels+"}",
		"steady state should record a resolution-time sample with state=no_drift")
	assert.NotContains(t, body, fmt.Sprintf(`hostname__drift_detected{provider="%s",state="%s"`, configProviderName, hostnameChanged),
		"steady state must not increment drift_detected")

	// Now change the config-provided hostname between checks: this is the drift transition the
	// E2E suite (test/new-e2e/tests/agent-runtimes/hostname_drift_*_test.go) never exercised.
	cfg.SetInTest("hostname", "host-b")
	ds.checkHostnameDrift(context.Background(), cacheHostnameKey)

	body = scrapeTelemetry(t)
	driftLabels := fmt.Sprintf(`provider="%s",state="%s"`, configProviderName, hostnameChanged)
	assert.Contains(t, body, "hostname__drift_resolution_time_ms_count{"+driftLabels+"} 1",
		"hostname change should record a resolution-time sample with state=hostname_drift")
	assert.Contains(t, body, "hostname__drift_detected{"+driftLabels+"} 1",
		"hostname change should increment drift_detected exactly once")

	cachedData, found := cache.Cache.Get(cacheHostnameKey)
	require.True(t, found)
	assert.Equal(t, Data{Hostname: "host-b", Provider: configProviderName}, cachedData,
		"cache should be updated to the newly detected hostname after drift")
}

func TestDriftServiceConfigOverrides(t *testing.T) {
	ds := driftService{
		initialDelay:      defaultInitialDelay,
		recurringInterval: defaultRecurringInterval,
	}

	t.Run("falls back to struct defaults when unset", func(t *testing.T) {
		configmock.New(t)
		assert.Equal(t, defaultInitialDelay, ds.getInitialDelay())
		assert.Equal(t, defaultRecurringInterval, ds.getRecurringInterval())
	})

	t.Run("config value overrides the struct default", func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.SetInTest("hostname_drift_initial_delay", "45s")
		cfg.SetInTest("hostname_drift_recurring_interval", "2h")

		assert.Equal(t, 45*time.Second, ds.getInitialDelay())
		assert.Equal(t, 2*time.Hour, ds.getRecurringInterval())
	})
}
