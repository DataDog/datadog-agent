// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package logs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/plog"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
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

func TestToManifest_PreservesAttributes(t *testing.T) {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("k8s.cluster.uid", "cluster-uid")
	rl.Resource().Attributes().PutStr("k8s.cluster.name", "cluster-name")
	rl.Resource().Attributes().PutStr("deployment.environment.name", "prod")
	rl.Resource().Attributes().PutStr("service.name", "checkout")
	rl.Resource().Attributes().PutStr("shared.attribute", "resource-value")

	sl := rl.ScopeLogs().AppendEmpty()
	lr := sl.LogRecords().AppendEmpty()
	lr.Attributes().PutStr("team", "platform")
	lr.Attributes().PutBool("manual.tag", true)
	lr.Attributes().PutInt("rollout", 3)
	lr.Attributes().PutStr("shared.attribute", "log-value")
	slice := lr.Attributes().PutEmptySlice("zones")
	slice.AppendEmpty().SetStr("us-east-1a")
	slice.AppendEmpty().SetStr("us-east-1b")
	nested := lr.Attributes().PutEmptyMap("nested")
	nested.PutStr("owner", "sre")
	lr.Body().SetStr(`{
		"apiVersion": "apps/v1",
		"kind": "Deployment",
		"metadata": {
			"uid": "deployment-uid",
			"resourceVersion": "123",
			"name": "checkout"
		}
	}`)

	manifest, isWatch, err := ToManifest(lr, rl.Resource())

	require.NoError(t, err)
	assert.False(t, isWatch)
	assert.Equal(t, "deployment-uid", manifest.Uid)
	assert.Equal(t, map[string]string{
		"k8s.cluster.uid":             "cluster-uid",
		"k8s.cluster.name":            "cluster-name",
		"deployment.environment.name": "prod",
		"service.name":                "checkout",
		"shared.attribute":            "log-value",
		"team":                        "platform",
		"manual.tag":                  "true",
		"rollout":                     "3",
		"zones":                       `["us-east-1a","us-east-1b"]`,
		"nested":                      `{"owner":"sre"}`,
	}, manifest.ExtraAttributes)
	assert.Equal(t, []string{"otel_receiver:k8sobjectsreceiver"}, manifest.Tags)
}

func TestChunkManifestsBySizeAndWeight_IncludesTagsAndExtraAttributes(t *testing.T) {
	first := &agentmodel.Manifest{
		Content: []byte("1234"),
		Tags:    []string{"tag:value"},
		ExtraAttributes: map[string]string{
			"env": "prod",
		},
	}
	second := &agentmodel.Manifest{
		Content: []byte("5"),
	}

	maxWeight := first.Size()
	chunks := chunkManifestsBySizeAndWeight([]*agentmodel.Manifest{first, second}, 10, maxWeight)

	require.Len(t, chunks, 2)
	assert.Same(t, first, chunks[0][0])
	assert.Same(t, second, chunks[1][0])
}
