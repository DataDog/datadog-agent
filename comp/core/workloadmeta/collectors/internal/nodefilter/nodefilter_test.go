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
			cfg := config.NewMockWithOverrides(t, map[string]interface{}{
				"kubernetes_kubelet_nodename": "test-node",
			})

			c := &collector{
				id:         collectorID,
				catalog:    workloadmeta.NodeAgent,
				config:     cfg,
				standalone: tt.standalone,
				useKubelet: tt.useKubelet,
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
// kubernetes_kubelet_nodename isn't set, since the node-scoped field
// selector has nothing to filter on.
func TestDisabledNoNodeName(t *testing.T) {
	pkgconfigenv.SetFeatures(t, pkgconfigenv.Kubernetes)
	cfg := config.NewMock(t)

	c := &collector{
		id:         collectorID,
		catalog:    workloadmeta.NodeAgent,
		config:     cfg,
		standalone: true,
		useKubelet: false,
	}

	err := c.Start(context.Background(), nil)
	require.Error(t, err)
	assert.True(t, pkgerrors.IsDisabled(err))
}
