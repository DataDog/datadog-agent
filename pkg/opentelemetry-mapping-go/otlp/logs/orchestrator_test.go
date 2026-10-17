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
	"go.uber.org/zap"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	orchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
)

func buildK8sLogs(clusterID, clusterName, uid, resourceVersion string, isWatch bool) plog.Logs {
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("k8s.cluster.uid", clusterID)
	rl.Resource().Attributes().PutStr("k8s.cluster.name", clusterName)
	sl := rl.ScopeLogs().AppendEmpty()
	lr := sl.LogRecords().AppendEmpty()
	var body string
	if isWatch {
		body = `{"type":"ADDED","object":{"apiVersion":"v1","kind":"Pod","metadata":{"uid":"` + uid + `","resourceVersion":"` + resourceVersion + `","name":"test-pod"}}}`
	} else {
		body = `{"apiVersion":"v1","kind":"Pod","metadata":{"uid":"` + uid + `","resourceVersion":"` + resourceVersion + `","name":"test-pod"}}`
	}
	lr.Body().SetStr(body)
	return ld
}

func TestGetManifestType(t *testing.T) {
	tests := []struct {
		name         string
		kind         string
		expectedType int
	}{
		{
			name:         "known kind Pod",
			kind:         "Pod",
			expectedType: int(orchestratormodel.K8sPod),
		},
		{
			name:         "known kind Deployment",
			kind:         "Deployment",
			expectedType: int(orchestratormodel.K8sDeployment),
		},
		{
			name:         "unknown kind treated as custom resource",
			kind:         "MyCustomResource",
			expectedType: int(orchestratormodel.K8sCR),
		},
		{
			name:         "empty kind treated as unset",
			kind:         "",
			expectedType: int(orchestratormodel.K8sUnsetType),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedType, getManifestType(tt.kind))
		})
	}
}

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

// TestTranslateK8sObjects_MultiCluster verifies that ResourceLogs from different clusters
// in a single plog.Logs are grouped into separate results, each attributed to the correct cluster.
func TestTranslateK8sObjects_MultiCluster(t *testing.T) {
	logger := zap.NewNop()

	ldA := buildK8sLogs("cluster-A-uid", "cluster-A", "pod-a", "v1", false)
	ldB := buildK8sLogs("cluster-B-uid", "cluster-B", "pod-b", "v1", false)
	ldB.ResourceLogs().MoveAndAppendTo(ldA.ResourceLogs())
	require.Equal(t, 2, ldA.ResourceLogs().Len(), "test setup: expected two ResourceLogs blocks")

	results := TranslateK8sObjects(ldA, nil, logger, 0)
	require.Len(t, results, 2, "each cluster should get its own result")

	byID := make(map[string]*K8sTranslationResult, len(results))
	for _, r := range results {
		byID[r.ClusterID] = r
	}
	require.Contains(t, byID, "cluster-A-uid")
	require.Contains(t, byID, "cluster-B-uid")

	assert.Equal(t, "cluster-A", byID["cluster-A-uid"].ClusterName)
	assert.Equal(t, "cluster-B", byID["cluster-B-uid"].ClusterName)

	require.Len(t, byID["cluster-A-uid"].Chunks, 1)
	require.Len(t, byID["cluster-A-uid"].Chunks[0], 1)
	assert.Equal(t, "pod-a", byID["cluster-A-uid"].Chunks[0][0].Uid)

	require.Len(t, byID["cluster-B-uid"].Chunks, 1)
	require.Len(t, byID["cluster-B-uid"].Chunks[0], 1)
	assert.Equal(t, "pod-b", byID["cluster-B-uid"].Chunks[0][0].Uid)
}

// TestTranslateK8sObjects_MissingClusterAttrs verifies that a ResourceLog missing cluster
// identity is skipped without contaminating other ResourceLogs in the same batch.
func TestTranslateK8sObjects_MissingClusterAttrs(t *testing.T) {
	logger := zap.NewNop()

	ldGood := buildK8sLogs("cluster-1-uid", "cluster-1", "pod-good", "v1", false)

	ldMissing := plog.NewLogs()
	rl := ldMissing.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	lr := sl.LogRecords().AppendEmpty()
	lr.Body().SetStr(`{"apiVersion":"v1","kind":"Pod","metadata":{"uid":"pod-orphan","resourceVersion":"v1","name":"orphan"}}`)
	ldMissing.ResourceLogs().MoveAndAppendTo(ldGood.ResourceLogs())
	require.Equal(t, 2, ldGood.ResourceLogs().Len(), "test setup: expected two ResourceLogs blocks")

	results := TranslateK8sObjects(ldGood, nil, logger, 0)
	require.Len(t, results, 1, "only the ResourceLog with cluster identity should produce a result")
	assert.Equal(t, "cluster-1-uid", results[0].ClusterID)
	require.Len(t, results[0].Chunks, 1)
	require.Len(t, results[0].Chunks[0], 1)
	assert.Equal(t, "pod-good", results[0].Chunks[0][0].Uid)
}

// TestTranslateK8sObjects_MaxChunkSize verifies that a small maxChunkSize causes manifests to be
// split across multiple chunks rather than collected into one.
func TestTranslateK8sObjects_MaxChunkSize(t *testing.T) {
	logger := zap.NewNop()

	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("k8s.cluster.uid", "cluster-chunk-uid")
	rl.Resource().Attributes().PutStr("k8s.cluster.name", "chunk-cluster")
	sl := rl.ScopeLogs().AppendEmpty()
	for _, uid := range []string{"pod-chunk-1", "pod-chunk-2"} {
		lr := sl.LogRecords().AppendEmpty()
		lr.Body().SetStr(`{"apiVersion":"v1","kind":"Pod","metadata":{"uid":"` + uid + `","resourceVersion":"v1","name":"` + uid + `"}}`)
	}

	results := TranslateK8sObjects(ld, nil, logger, 1)
	require.Len(t, results, 1)
	assert.Len(t, results[0].Chunks, 2, "each manifest should be in its own chunk when maxChunkSize=1")
}

// TestTranslateK8sObjects_CrossClusterCacheIsolation verifies that deduplication cache entries
// are scoped per cluster, so the same manifest UID in two different clusters does not suppress
// one another.
func TestTranslateK8sObjects_CrossClusterCacheIsolation(t *testing.T) {
	logger := zap.NewNop()
	cache := NewManifestCache()

	ldA := buildK8sLogs("cluster-A-uid", "cluster-A", "shared-uid", "v1", false)
	ldB := buildK8sLogs("cluster-B-uid", "cluster-B", "shared-uid", "v1", false)

	firstA := TranslateK8sObjects(ldA, cache, logger, 0)
	require.Len(t, firstA, 1)
	require.Len(t, firstA[0].Chunks, 1)

	firstB := TranslateK8sObjects(ldB, cache, logger, 0)
	require.Len(t, firstB, 1, "cluster B's manifest should not be suppressed by cluster A's cache entry")
	require.Len(t, firstB[0].Chunks, 1)

	ldA2 := buildK8sLogs("cluster-A-uid", "cluster-A", "shared-uid", "v1", false)
	secondA := TranslateK8sObjects(ldA2, cache, logger, 0)
	assert.Empty(t, secondA, "same UID+version within cluster A should be deduplicated")
}
