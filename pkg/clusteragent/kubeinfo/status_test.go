// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubeinfo

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/version"
)

func reset(t *testing.T) {
	resetState := func() {
		mu.Lock()
		defer mu.Unlock()
		state = info{}
	}
	resetState()
	t.Cleanup(resetState)
}

func TestProvider(t *testing.T) {
	tests := []struct {
		name          string
		serverVersion *version.Info
		wantDetected  string
		wantWarning   bool
	}{
		{name: "unknown", serverVersion: nil},
		{name: "supported", serverVersion: &version.Info{GitVersion: "v1.23.17-eks-a1b2c3"}, wantDetected: "v1.23.17-eks-a1b2c3"},
		{name: "unparsable", serverVersion: &version.Info{GitVersion: "foo"}, wantDetected: "foo"},
		{name: "unsupported", serverVersion: &version.Info{GitVersion: "v1.22.17-eks-a1b2c3"}, wantDetected: "v1.22.17-eks-a1b2c3", wantWarning: true},
		{name: "unsupported without git version", serverVersion: &version.Info{Major: "1", Minor: "22+"}, wantDetected: "v1.22+", wantWarning: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reset(t)
			setServerVersion(tt.serverVersion)

			p := Provider{}
			assert.Equal(t, "Kubernetes Information", p.Name())

			stats := map[string]interface{}{}
			require.NoError(t, p.JSON(false, stats))
			require.Contains(t, stats, "kubernetesInfo")
			versionInfo := stats["kubernetesInfo"].(map[string]interface{})["version"].(map[string]interface{})
			assert.Equal(t, tt.wantDetected, versionInfo["detected"])
			assert.Equal(t, "v1.23", versionInfo["minSupported"])

			var text bytes.Buffer
			require.NoError(t, p.Text(false, &text))
			if tt.wantDetected == "" {
				assert.Contains(t, text.String(), "Server Version: unknown")
			} else {
				assert.Contains(t, text.String(), "Server Version: "+tt.wantDetected)
			}

			if !tt.wantWarning {
				assert.NotContains(t, versionInfo, "warning")
				assert.NotContains(t, text.String(), "WARNING")
				return
			}
			assert.Contains(t, versionInfo["warning"], "Kubernetes "+tt.wantDetected+" is older than v1.23")
			assert.Contains(t, text.String(), "WARNING: Kubernetes "+tt.wantDetected+" is older than v1.23")
			assert.Contains(t, text.String(), "Datadog Cluster Agent 7.86.0")
		})
	}
}

func TestProviderClusterInfo(t *testing.T) {
	tests := []struct {
		name         string
		clusterName  string
		clusterID    string
		distribution string
		wantText     []string
		wantNoText   []string
	}{
		{
			name:       "unknown",
			wantText:   []string{"Cluster Name: unknown", "Cluster ID: unknown"},
			wantNoText: []string{"Distribution"},
		},
		{
			name:         "known",
			clusterName:  "my-cluster",
			clusterID:    "5f2e8c1a-0000-0000-0000-000000000000",
			distribution: "eks",
			wantText:     []string{"Cluster Name: my-cluster", "Cluster ID: 5f2e8c1a-0000-0000-0000-000000000000", "Distribution: eks"},
		},
		{
			name:        "no distribution",
			clusterName: "my-cluster",
			clusterID:   "5f2e8c1a-0000-0000-0000-000000000000",
			wantText:    []string{"Cluster Name: my-cluster"},
			wantNoText:  []string{"Distribution"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reset(t)
			SetClusterInfo(tt.clusterName, tt.clusterID, tt.distribution)

			p := Provider{}
			stats := map[string]interface{}{}
			require.NoError(t, p.JSON(false, stats))
			kubeInfo := stats["kubernetesInfo"].(map[string]interface{})
			assert.Equal(t, tt.clusterName, kubeInfo["clusterName"])
			assert.Equal(t, tt.clusterID, kubeInfo["clusterID"])
			assert.Equal(t, tt.distribution, kubeInfo["distribution"])

			var text bytes.Buffer
			require.NoError(t, p.Text(false, &text))
			for _, s := range tt.wantText {
				assert.Contains(t, text.String(), s)
			}
			for _, s := range tt.wantNoText {
				assert.NotContains(t, text.String(), s)
			}
		})
	}
}
