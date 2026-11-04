// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package kubeinfo

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/version"

	"github.com/DataDog/datadog-agent/comp/core"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func newWorkloadmeta(t *testing.T) workloadmetamock.Mock {
	return fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
}

func kubeCapabilities(serverVersion *version.Info) *workloadmeta.KubeCapabilities {
	return &workloadmeta.KubeCapabilities{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubeCapabilities,
			ID:   workloadmeta.KubeCapabilitiesID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: workloadmeta.KubeCapabilitiesName,
		},
		Version: serverVersion,
	}
}

// newProvider returns a Provider with stubbed cluster information getters.
func newProvider(wmeta workloadmeta.Component, clusterName, clusterID, distribution string) Provider {
	hostname, _ := hostnamemock.NewMock("my-host")
	p := NewProvider(wmeta, hostname)
	p.getClusterName = func(_ context.Context, host string) string {
		if host != "my-host" {
			return ""
		}
		return clusterName
	}
	p.getClusterID = func() (string, error) { return clusterID, nil }
	p.getDistribution = func(context.Context) string { return distribution }
	return p
}

func TestProvider(t *testing.T) {
	tests := []struct {
		name          string
		serverVersion *version.Info
		wantDetected  string
		wantWarning   bool
	}{
		{name: "not collected yet"},
		{name: "supported", serverVersion: &version.Info{GitVersion: "v1.23.17-eks-a1b2c3"}, wantDetected: "v1.23.17-eks-a1b2c3"},
		{name: "unsupported", serverVersion: &version.Info{GitVersion: "v1.22.17-eks-a1b2c3"}, wantDetected: "v1.22.17-eks-a1b2c3", wantWarning: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wmeta := newWorkloadmeta(t)
			if tt.serverVersion != nil {
				wmeta.Set(kubeCapabilities(tt.serverVersion))
			}

			p := newProvider(wmeta, "", "", "")
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
			p := newProvider(newWorkloadmeta(t), tt.clusterName, tt.clusterID, tt.distribution)
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
