// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patch

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/stretchr/testify/require"
)

func TestPatchRequestValidate(t *testing.T) {
	tests := []struct {
		name        string
		LibConfig   common.LibConfig
		K8sTarget   K8sTarget
		clusterName string
		valid       bool
	}{
		{
			name:        "valid",
			LibConfig:   common.LibConfig{Language: "lang", Version: "latest"},
			K8sTarget:   K8sTarget{Cluster: "cluster", Kind: "deployment", Name: "name", Namespace: "ns"},
			clusterName: "cluster",
			valid:       true,
		},
		{
			name:        "empty version",
			LibConfig:   common.LibConfig{Language: "lang"},
			K8sTarget:   K8sTarget{Cluster: "cluster", Kind: "deployment", Name: "name", Namespace: "ns"},
			clusterName: "cluster",
			valid:       false,
		},
		{
			name:        "empty language",
			LibConfig:   common.LibConfig{Version: "latest"},
			K8sTarget:   K8sTarget{Cluster: "cluster", Kind: "deployment", Name: "name", Namespace: "ns"},
			clusterName: "cluster",
			valid:       false,
		},
		{
			name:        "empty cluster",
			LibConfig:   common.LibConfig{Language: "lang", Version: "latest"},
			K8sTarget:   K8sTarget{Kind: "deployment", Name: "name", Namespace: "ns"},
			clusterName: "cluster",
			valid:       false,
		},
		{
			name:        "wrong cluster",
			LibConfig:   common.LibConfig{Language: "lang", Version: "latest"},
			K8sTarget:   K8sTarget{Cluster: "wrong-cluster", Kind: "deployment", Name: "name", Namespace: "ns"},
			clusterName: "cluster",
			valid:       false,
		},
		{
			name:        "empty kind",
			LibConfig:   common.LibConfig{Language: "lang", Version: "latest"},
			K8sTarget:   K8sTarget{Cluster: "cluster", Name: "name", Namespace: "ns"},
			clusterName: "cluster",
			valid:       false,
		},
		{
			name:        "empty name",
			LibConfig:   common.LibConfig{Language: "lang", Version: "latest"},
			K8sTarget:   K8sTarget{Cluster: "cluster", Kind: "deployment", Namespace: "ns"},
			clusterName: "cluster",
			valid:       false,
		},
		{
			name:        "empty namesapce",
			LibConfig:   common.LibConfig{Language: "lang", Version: "latest"},
			K8sTarget:   K8sTarget{Cluster: "cluster", Kind: "deployment", Name: "name"},
			clusterName: "cluster",
			valid:       false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := PatchRequest{
				LibConfig: tt.LibConfig,
				K8sTarget: tt.K8sTarget,
			}
			err := pr.Validate(tt.clusterName)
			require.True(t, (err == nil) == tt.valid)
		})
	}
}
