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

func truePtr() *bool {
	t := true
	return &t
}

func falsePtr() *bool {
	f := false
	return &f
}

func TestPatchRequestValidate(t *testing.T) {
	tests := []struct {
		name        string
		LibConfig   common.LibConfig
		K8sTarget   K8sTarget
		clusterName string
		valid       bool
	}{
		{
			name:      "valid",
			LibConfig: common.LibConfig{Language: "lang", Version: "latest"},
			//K8sTarget:   K8sTarget{Cluster: "cluster", Kind: "deployment", Name: "name", Namespace: "ns"},
			K8sTarget: K8sTarget{
				ClusterTargets: []K8sClusterTarget{
					K8sClusterTarget{ClusterName: "cluster", Enabled: truePtr(), EnabledNamespaces: &([]string{"ns"})},
				},
			},
			clusterName: "cluster",
			valid:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := Request{
				LibConfig: tt.LibConfig,
				K8sTarget: &tt.K8sTarget,
			}
			err := request.Validate(tt.clusterName)
			require.True(t, (err == nil) == tt.valid)
		})
	}
}
