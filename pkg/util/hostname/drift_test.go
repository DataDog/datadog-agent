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

// TestCheckHostnameDriftEmitsTelemetry replaces
// test/new-e2e/tests/agent-runtimes/hostname_drift_{common,nix,win}_test.go, which provisioned a
// real EC2 Linux/Windows host and asserted this same steady-state telemetry line via
// `agent diagnose show-metadata agent-full-telemetry`.
//
// It goes through the real production entrypoint, GetWithProvider, rather than calling
// checkHostnameDrift directly: driftCalculator.scheduleHostnameDriftChecks is only reached from
// getHostname's fallthrough path (providers.go:233-238), which early-stop providers such as the
// "hostname" config value never reach (they return at providers.go:228, before that call). Only
// the "coupled" chain (fqdn/container/os/aws) that a real, unconfigured EC2 host falls through
// schedules drift checks at all, so the replacement has to drive that same path to catch a
// regression in the scheduling wiring itself.
//
// setupHostnameTest's "hostname from EC2 with default system name" fixture (providers_test.go)
// reproduces exactly that: a default AWS-pattern OS hostname with no config/file/fargate/GCE/azure
// override, so resolution falls through to the EC2 provider — the same real path the deleted E2E
// suite exercised on an actual EC2 instance.
func TestCheckHostnameDriftEmitsTelemetry(t *testing.T) {
	setupHostnameTest(t, testCase{
		name:             "hostname from EC2 with default system name",
		FQDNEC2:          true,
		OSEC2:            true,
		EC2:              true,
		expectedHostname: "hostname-from-ec2",
		expectedProvider: "aws",
	})

	// configmock.New(t) is idempotent within a test (see pkg/config/mock/mock.go), so this just
	// gets a handle to the same mock setupHostnameTest already installed.
	cfg := configmock.New(t)
	cfg.SetInTest("hostname_drift_initial_delay", "1ms")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	t.Cleanup(func() { cache.Cache.Delete(cache.BuildAgentKey("hostname_check")) })

	_, err := GetWithProvider(ctx)
	require.NoError(t, err)

	// Prometheus's text exposition format (which the diagnose CLI ultimately renders) sorts label
	// names alphabetically, and telemetry.Options.NameWithSeparator inserts a leading "_" that
	// becomes a second underscore once joined with the "hostname" subsystem — hence
	// "hostname__drift_..." with labels ordered provider-then-state, not the declaration order.
	require.Eventually(t, func() bool {
		body := scrapeTelemetry(t)
		return strings.Contains(body, "hostname__drift_resolution_time_ms_count{") &&
			strings.Contains(body, `provider="aws"`) &&
			strings.Contains(body, `state="`+noDrift+`"`)
	}, time.Second, 5*time.Millisecond,
		"drift telemetry with provider=aws, state=no_drift should be scheduled and emitted")
}
