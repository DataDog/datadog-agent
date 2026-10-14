// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeversion

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/version"
)

func TestProvider(t *testing.T) {
	tests := []struct {
		name          string
		serverVersion *version.Info
		wantHidden    bool
	}{
		{name: "unknown", serverVersion: nil, wantHidden: true},
		{name: "supported", serverVersion: &version.Info{GitVersion: "v1.23.17-eks-a1b2c3"}, wantHidden: true},
		{name: "unsupported", serverVersion: &version.Info{GitVersion: "v1.22.17-eks-a1b2c3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unsupportedVersion = ""
			t.Cleanup(func() { unsupportedVersion = "" })
			setServerVersion(tt.serverVersion)

			p := Provider{}
			stats := map[string]interface{}{}
			require.NoError(t, p.JSON(false, stats))
			var text bytes.Buffer
			require.NoError(t, p.Text(false, &text))

			if tt.wantHidden {
				assert.Empty(t, p.Name())
				assert.NotContains(t, stats, "kubernetesVersion")
				assert.Empty(t, text.String())
				return
			}

			assert.Equal(t, "Kubernetes Version", p.Name())
			require.Contains(t, stats, "kubernetesVersion")
			info := stats["kubernetesVersion"].(map[string]interface{})
			assert.Equal(t, "v1.22.17-eks-a1b2c3", info["detectedVersion"])
			assert.Equal(t, "v1.23", info["minSupportedVersion"])
			assert.Equal(t, false, info["supported"])
			assert.Contains(t, text.String(), "Detected version: v1.22.17-eks-a1b2c3")
			assert.Contains(t, text.String(), "Minimum supported version: v1.23")
			assert.Contains(t, text.String(), "WARNING: Kubernetes v1.22.17-eks-a1b2c3 is older than v1.23")
			assert.Contains(t, text.String(), "Datadog Cluster Agent 7.86.0")
		})
	}
}
