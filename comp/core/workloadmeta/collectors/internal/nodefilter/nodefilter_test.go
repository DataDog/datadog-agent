// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build nodefilter

package nodefilter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigenv "github.com/DataDog/datadog-agent/pkg/config/env"
	pkgerrors "github.com/DataDog/datadog-agent/pkg/errors"
)

// TestDisabledStandalone verifies that this collector only applies to
// otel-agent running in DDOT standalone mode, and defers to the kubelet
// collector when that mode has opted back out via useKubelet — mirroring
// the kubelet collector's own mutual-exclusivity test.
func TestDisabledStandalone(t *testing.T) {
	tests := []struct {
		name       string
		standalone bool
		useKubelet bool
		disabled   bool
	}{
		{name: "not standalone, kubelet not opted out", standalone: false, useKubelet: false, disabled: true},
		{name: "not standalone, kubelet opted out is a no-op", standalone: false, useKubelet: true, disabled: true},
		{name: "standalone, defaults to nodefilter", standalone: true, useKubelet: false, disabled: false},
		{name: "standalone, opted back out to kubelet", standalone: true, useKubelet: true, disabled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Get past the Kubernetes-feature and node-name guards for every
			// case, so a non-"disabled" error below can only come from the
			// mutual-exclusivity check having let the collector through.
			pkgconfigenv.SetFeatures(t, pkgconfigenv.Kubernetes)
			t.Setenv("K8S_NODE_NAME", "test-node")
			cfg := config.NewMock(t)

			c := &collector{
				id:             collectorID,
				catalog:        workloadmeta.NodeAgent,
				config:         cfg,
				standalone:     tt.standalone,
				useKubelet:     tt.useKubelet,
				nodeFromEnvVar: "K8S_NODE_NAME",
			}

			err := c.Start(context.Background(), nil)
			require.Error(t, err)
			if tt.disabled {
				assert.True(t, pkgerrors.IsDisabled(err))
			} else {
				// Mutual exclusivity let it through; it now fails building a
				// real Kubernetes API client, which is not a "disabled" error.
				assert.False(t, pkgerrors.IsDisabled(err))
			}
		})
	}
}

// TestDisabledNoNodeName verifies that the collector refuses to start when
// its configured node-name environment variable isn't set, since the
// node-scoped field selector has nothing to filter on.
func TestDisabledNoNodeName(t *testing.T) {
	pkgconfigenv.SetFeatures(t, pkgconfigenv.Kubernetes)
	cfg := config.NewMock(t)

	c := &collector{
		id:             collectorID,
		catalog:        workloadmeta.NodeAgent,
		config:         cfg,
		standalone:     true,
		useKubelet:     false,
		nodeFromEnvVar: "K8S_NODE_NAME",
	}

	err := c.Start(context.Background(), nil)
	require.Error(t, err)
	assert.True(t, pkgerrors.IsDisabled(err))
}

// TestNodeFromEnvVar_CustomName verifies that the collector reads the node
// name from whichever environment variable nodeFromEnvVar names, mirroring
// k8sattributesprocessor's configurable node_from_env_var filter rather than
// hardcoding a single env var.
func TestNodeFromEnvVar_CustomName(t *testing.T) {
	pkgconfigenv.SetFeatures(t, pkgconfigenv.Kubernetes)
	t.Setenv("MY_CUSTOM_NODE_NAME_VAR", "test-node")
	cfg := config.NewMock(t)

	c := &collector{
		id:             collectorID,
		catalog:        workloadmeta.NodeAgent,
		config:         cfg,
		standalone:     true,
		useKubelet:     false,
		nodeFromEnvVar: "MY_CUSTOM_NODE_NAME_VAR",
	}

	err := c.Start(context.Background(), nil)
	require.Error(t, err)
	// Node name resolved successfully; it now fails building a real
	// Kubernetes API client, which is not a "disabled" error.
	assert.False(t, pkgerrors.IsDisabled(err))
}

// TestNewCollector_NodeFromEnvVarDefault verifies that NewCollector caches
// nodeFromEnvVar from the otelcollector.standalone.node_from_env_var config
// key, whose DD agent schema default is K8S_NODE_NAME.
func TestNewCollector_NodeFromEnvVarDefault(t *testing.T) {
	cfg := config.NewMock(t)
	provider, err := NewCollector(dependencies{Config: cfg})
	require.NoError(t, err)
	c := provider.Collector.(*collector)
	assert.Equal(t, "K8S_NODE_NAME", c.nodeFromEnvVar)
}
