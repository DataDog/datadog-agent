// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteconfig

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

var testProducts = []string{
	state.ProductContainerAutoscalingSettings,
	state.ProductContainerAutoscalingValues,
}

// NewClient starts nothing, so these tests never reach the configured endpoint.
// Start is what begins polling, and no unit test calls it.
func TestNewClient(t *testing.T) {
	runPath := t.TempDir()
	cfg := newTestConfig(t, "https://config.datad0g.com", testAPIKey)
	cfg.SetInTest("run_path", runPath)

	endpointStatus := NewEndpointStatus()
	built, err := NewClient(cfg, "test-hostname", "test-cluster", "test-cluster-id", testProducts, endpointStatus)
	require.NoError(t, err)
	require.NotNil(t, built.Client)
	t.Cleanup(built.Close)

	// Its own uptane cache: the default service uses remote-config.db in the
	// same run_path and the two must not collide.
	assert.FileExists(t, filepath.Join(runPath, DatabaseFileName))
	assert.NoFileExists(t, filepath.Join(runPath, "remote-config.db"))

	// Built but idle, so an update cannot be missed before the config retrievers
	// subscribe.
	assert.Same(t, endpointStatus, built.status)
	assert.False(t, endpointStatus.snapshot().started)
}

// TestClientCloseReleasesTheService covers Close stopping the service behind the
// client, not just the client. The uptane database is the observable: bbolt holds
// an exclusive lock on it, so a second client on the same run_path can only be
// built once the first service has released it.
func TestClientCloseReleasesTheService(t *testing.T) {
	runPath := t.TempDir()
	cfg := newTestConfig(t, "https://config.datad0g.com", testAPIKey)
	cfg.SetInTest("run_path", runPath)

	first, err := NewClient(cfg, "test-hostname", "test-cluster", "test-cluster-id", testProducts, NewEndpointStatus())
	require.NoError(t, err)
	first.Close()

	second, err := NewClient(cfg, "test-hostname", "test-cluster", "test-cluster-id", testProducts, NewEndpointStatus())
	require.NoError(t, err, "the first client's service still holds the database lock")
	t.Cleanup(second.Close)
}

func TestNewClientErrors(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		clusterID string
		products  []string
		wantError string
	}{
		{
			name:      "invalid endpoint",
			url:       "https://config.example.com",
			clusterID: "test-cluster-id",
			products:  testProducts,
			wantError: "does not point at a known Datadog site",
		},
		{
			// Targeting is by self-reported cluster ID, so without one the
			// backend can never match this client.
			name:      "no cluster id",
			url:       "https://config.datad0g.com",
			products:  testProducts,
			wantError: "cluster ID is required",
		},
		{
			name:      "no products",
			url:       "https://config.datad0g.com",
			clusterID: "test-cluster-id",
			wantError: "no autoscaling remote configuration products",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newTestConfig(t, tt.url, testAPIKey)
			cfg.SetInTest("run_path", t.TempDir())

			endpointStatus := NewEndpointStatus()
			built, err := NewClient(cfg, "test-hostname", "test-cluster", tt.clusterID, tt.products, endpointStatus)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantError)
			// No fallback to the default endpoint, and nothing left running.
			assert.Nil(t, built)
			// A service that failed to build must not report itself as started.
			assert.False(t, endpointStatus.snapshot().started)
		})
	}
}

// TestNewClientRejectsUnusableRemoteConfigKey covers the legacy per-organization
// key being malformed: the service refuses it, and the client must not be left
// running against an endpoint it cannot authenticate to.
func TestNewClientRejectsUnusableRemoteConfigKey(t *testing.T) {
	cfg := newTestConfig(t, "https://config.datad0g.com", testAPIKey)
	cfg.SetInTest("run_path", t.TempDir())
	cfg.SetInTest(KeySetting, "DDRCM_not-valid-base32-$$$")

	built, err := NewClient(cfg, "test-hostname", "test-cluster", "test-cluster-id", testProducts, NewEndpointStatus())
	require.Error(t, err)
	assert.Nil(t, built)
}

// TestNewClientToleratesNilStatus covers a caller passing no status: one is
// substituted, so start has something to mark. The service tolerates a nil
// instance, so Start has something to mark.
func TestNewClientToleratesNilStatus(t *testing.T) {
	cfg := newTestConfig(t, "https://config.datad0g.com", testAPIKey)
	cfg.SetInTest("run_path", t.TempDir())

	built, err := NewClient(cfg, "test-hostname", "test-cluster", "test-cluster-id", testProducts, nil)
	require.NoError(t, err)
	require.NotNil(t, built.Client)
	require.NotNil(t, built.status)
	t.Cleanup(built.Close)
}
