// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !serverless && test

package hostnameimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hostnamedef "github.com/DataDog/datadog-agent/comp/core/hostname/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

func TestDetermineDriftState(t *testing.T) {
	tests := []struct {
		name     string
		oldData  hostnamedef.Data
		newData  hostnamedef.Data
		expected driftInfo
	}{
		{
			name:     "no drift",
			oldData:  hostnamedef.Data{Hostname: "host1", Provider: "provider1"},
			newData:  hostnamedef.Data{Hostname: "host1", Provider: "provider1"},
			expected: driftInfo{state: driftStateNone, hasDrift: false},
		},
		{
			name:     "hostname changed only",
			oldData:  hostnamedef.Data{Hostname: "host1", Provider: "provider1"},
			newData:  hostnamedef.Data{Hostname: "host2", Provider: "provider1"},
			expected: driftInfo{state: driftStateHostname, hasDrift: true},
		},
		{
			name:     "provider changed only",
			oldData:  hostnamedef.Data{Hostname: "host1", Provider: "provider1"},
			newData:  hostnamedef.Data{Hostname: "host1", Provider: "provider2"},
			expected: driftInfo{state: driftStateProvider, hasDrift: true},
		},
		{
			name:     "both hostname and provider changed",
			oldData:  hostnamedef.Data{Hostname: "host1", Provider: "provider1"},
			newData:  hostnamedef.Data{Hostname: "host2", Provider: "provider2"},
			expected: driftInfo{state: driftStateHostnameProvider, hasDrift: true},
		},
		{
			name:     "empty hostnames",
			oldData:  hostnamedef.Data{Hostname: "", Provider: "provider1"},
			newData:  hostnamedef.Data{Hostname: "", Provider: "provider1"},
			expected: driftInfo{state: driftStateNone, hasDrift: false},
		},
		{
			name:     "empty providers",
			oldData:  hostnamedef.Data{Hostname: "host1", Provider: ""},
			newData:  hostnamedef.Data{Hostname: "host1", Provider: ""},
			expected: driftInfo{state: driftStateNone, hasDrift: false},
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

func TestDriftServiceStart(t *testing.T) {
	cacheHostnameKey := cache.BuildAgentKey("hostname_check")
	cache.Cache.Delete(cacheHostnameKey)

	hostnameData := hostnamedef.Data{
		Hostname: "test-hostname",
		Provider: "test-provider",
	}

	cfg := configmock.New(t)
	telMock := telemetryimpl.NewMock(t)
	ds := newDriftService(cfg, telMock)
	ds.initialDelay = 10 * time.Millisecond
	ds.recurringInterval = 50 * time.Millisecond

	ds.start(hostnameData)
	defer ds.stop()

	// Verify that the initial data was cached immediately on start
	cachedData, found := cache.Cache.Get(cacheHostnameKey)
	require.True(t, found, "Expected hostname data to be cached")

	cachedHostnameData, ok := cachedData.(hostnamedef.Data)
	require.True(t, ok, "Expected cached data to be of type hostnamedef.Data")
	assert.Equal(t, hostnameData.Hostname, cachedHostnameData.Hostname)
	assert.Equal(t, hostnameData.Provider, cachedHostnameData.Provider)

	// Verify that telemetry metrics were created on the service instance
	assert.NotNil(t, ds.driftDetected, "Expected drift_detected telemetry metric to be created")
	assert.NotNil(t, ds.driftResolutionTime, "Expected drift_resolution_time_ms telemetry metric to be created")
}
