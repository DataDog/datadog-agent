// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build nodefilter

package nodefilter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfigenv "github.com/DataDog/datadog-agent/pkg/config/env"
	pkgerrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

// TestLocalNodeName verifies that this collector only applies to otel-agent
// running on Kubernetes in DDOT standalone mode, without the kubelet
// collector opt-out, and with its node-name env var set, and that Enabled
// (which the kubelet collector steps aside on) agrees.
func TestLocalNodeName(t *testing.T) {
	tests := []struct {
		name       string
		flavor     string
		standalone bool
		useKubelet bool
		kubernetes bool
		nodeName   string
		// wantNodeName is empty when the collector must be disabled.
		wantNodeName string
	}{
		{
			name:   "standalone otel-agent, defaults to nodefilter",
			flavor: flavor.OTelAgent, standalone: true, kubernetes: true, nodeName: "test-node",
			wantNodeName: "test-node",
		},
		{
			name:   "not standalone",
			flavor: flavor.OTelAgent, standalone: false, kubernetes: true, nodeName: "test-node",
		},
		{
			name:   "not otel-agent",
			flavor: flavor.DefaultAgent, standalone: true, kubernetes: true, nodeName: "test-node",
		},
		{
			name:   "opted back out to kubelet",
			flavor: flavor.OTelAgent, standalone: true, useKubelet: true, kubernetes: true, nodeName: "test-node",
		},
		{
			name:   "not on Kubernetes",
			flavor: flavor.OTelAgent, standalone: true, kubernetes: false, nodeName: "test-node",
		},
		{
			name:   "node name env var not set",
			flavor: flavor.OTelAgent, standalone: true, kubernetes: true, nodeName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flavor.SetTestFlavor(t, tt.flavor)
			if tt.kubernetes {
				pkgconfigenv.SetFeatures(t, pkgconfigenv.Kubernetes)
			} else {
				pkgconfigenv.SetFeatures(t)
			}
			// K8S_NODE_NAME is the otelcollector.standalone.node_from_env_var
			// default.
			t.Setenv("K8S_NODE_NAME", tt.nodeName)
			cfg := config.NewMockWithOverrides(t, map[string]interface{}{
				"otel_standalone": tt.standalone,
				"otelcollector.standalone.use_kubelet_collector": tt.useKubelet,
			})

			nodeName, err := localNodeName(cfg)
			if tt.wantNodeName == "" {
				require.Error(t, err)
				assert.True(t, pkgerrors.IsDisabled(err))
				assert.False(t, Enabled(cfg))
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantNodeName, nodeName)
				assert.True(t, Enabled(cfg))
			}
		})
	}
}

// TestLocalNodeName_CustomEnvVar verifies that the node name is read from
// whichever environment variable otelcollector.standalone.node_from_env_var
// names, mirroring k8sattributesprocessor's configurable node_from_env_var
// filter rather than hardcoding a single env var.
func TestLocalNodeName_CustomEnvVar(t *testing.T) {
	flavor.SetTestFlavor(t, flavor.OTelAgent)
	pkgconfigenv.SetFeatures(t, pkgconfigenv.Kubernetes)
	t.Setenv("K8S_NODE_NAME", "")
	t.Setenv("MY_CUSTOM_NODE_NAME_VAR", "test-node")
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"otel_standalone": true,
		"otelcollector.standalone.node_from_env_var": "MY_CUSTOM_NODE_NAME_VAR",
	})

	nodeName, err := localNodeName(cfg)
	require.NoError(t, err)
	assert.Equal(t, "test-node", nodeName)
}
