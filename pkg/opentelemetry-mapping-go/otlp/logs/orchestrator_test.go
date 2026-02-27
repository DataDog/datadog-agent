// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package logs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildManifestFromK8sResource_NodeName(t *testing.T) {
	tests := []struct {
		name             string
		k8sResource      map[string]interface{}
		expectedNodeName string
	}{
		{
			name: "Pod with nodeName in spec",
			k8sResource: map[string]interface{}{
				"kind":       "Pod",
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"uid":             "pod-uid-123",
					"resourceVersion": "1",
				},
				"spec": map[string]interface{}{
					"nodeName": "worker-1",
				},
			},
			expectedNodeName: "worker-1",
		},
		{
			name: "Pod without spec",
			k8sResource: map[string]interface{}{
				"kind":       "Pod",
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"uid":             "pod-uid-456",
					"resourceVersion": "1",
				},
			},
			expectedNodeName: "",
		},
		{
			name: "Pod with spec but no nodeName",
			k8sResource: map[string]interface{}{
				"kind":       "Pod",
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"uid":             "pod-uid-789",
					"resourceVersion": "1",
				},
				"spec": map[string]interface{}{
					"containers": []interface{}{},
				},
			},
			expectedNodeName: "",
		},
		{
			name: "Node uses metadata.name as nodeName",
			k8sResource: map[string]interface{}{
				"kind":       "Node",
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"name":            "node-1",
					"uid":             "node-uid-789",
					"resourceVersion": "1",
				},
			},
			expectedNodeName: "node-1",
		},
		{
			name: "Node without metadata.name",
			k8sResource: map[string]interface{}{
				"kind":       "Node",
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"uid":             "node-uid-101",
					"resourceVersion": "1",
				},
			},
			expectedNodeName: "",
		},
		{
			name: "Deployment has empty nodeName",
			k8sResource: map[string]interface{}{
				"kind":       "Deployment",
				"apiVersion": "apps/v1",
				"metadata": map[string]interface{}{
					"uid":             "deploy-uid-202",
					"resourceVersion": "1",
				},
			},
			expectedNodeName: "",
		},
		{
			name: "Service has empty nodeName",
			k8sResource: map[string]interface{}{
				"kind":       "Service",
				"apiVersion": "v1",
				"metadata": map[string]interface{}{
					"uid":             "svc-uid-303",
					"resourceVersion": "1",
				},
			},
			expectedNodeName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest, err := BuildManifestFromK8sResource(tt.k8sResource, false)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedNodeName, manifest.NodeName)
		})
	}
}
