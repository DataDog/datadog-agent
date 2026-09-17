// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package logsagentexporter

import (
	"testing"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	logsmapping "github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/logs"
)

// buildK8sLogs creates a plog.Logs with a single pod log record for testing.
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

func TestTranslateK8sObjects_Deduplication(t *testing.T) {
	logger := zap.NewNop()

	t.Run("same manifest sent twice is deduplicated", func(t *testing.T) {
		cache := logsmapping.NewManifestCache()
		ld := buildK8sLogs("cluster-1", "my-cluster", "pod-uid-1", "v1", false)

		first := logsmapping.TranslateK8sObjects(ld, cache, logger, 0)
		require.Len(t, first, 1)
		require.Len(t, first[0].Chunks, 1)
		assert.Len(t, first[0].Chunks[0], 1)

		second := logsmapping.TranslateK8sObjects(ld, cache, logger, 0)
		assert.Empty(t, second, "duplicate pull manifest should produce no results")
	})

	t.Run("updated resourceVersion is not deduplicated", func(t *testing.T) {
		cache := logsmapping.NewManifestCache()
		ld1 := buildK8sLogs("cluster-1", "my-cluster", "pod-uid-2", "v1", false)
		ld2 := buildK8sLogs("cluster-1", "my-cluster", "pod-uid-2", "v2", false)

		logsmapping.TranslateK8sObjects(ld1, cache, logger, 0)
		second := logsmapping.TranslateK8sObjects(ld2, cache, logger, 0)
		require.Len(t, second, 1)
		require.Len(t, second[0].Chunks, 1)
		assert.Len(t, second[0].Chunks[0], 1, "updated resourceVersion should not be skipped")
	})

	t.Run("watch events bypass deduplication cache", func(t *testing.T) {
		cache := logsmapping.NewManifestCache()
		ld := buildK8sLogs("cluster-1", "my-cluster", "pod-uid-3", "v1", true)

		logsmapping.TranslateK8sObjects(ld, cache, logger, 0)
		second := logsmapping.TranslateK8sObjects(ld, cache, logger, 0)
		require.Len(t, second, 1)
		require.Len(t, second[0].Chunks, 1, "watch events should always be forwarded")
	})

	t.Run("nil cache disables deduplication", func(t *testing.T) {
		ld := buildK8sLogs("cluster-1", "my-cluster", "pod-uid-4", "v1", false)

		first := logsmapping.TranslateK8sObjects(ld, nil, logger, 0)
		second := logsmapping.TranslateK8sObjects(ld, nil, logger, 0)
		require.Len(t, first, 1)
		require.Len(t, first[0].Chunks, 1)
		require.Len(t, second, 1)
		require.Len(t, second[0].Chunks, 1, "nil cache should not deduplicate")
	})
}

// TestTranslateK8sObjects_MultiCluster verifies that ResourceLogs from different clusters
// in a single plog.Logs are grouped into separate results, each attributed to the correct cluster.
func TestTranslateK8sObjects_MultiCluster(t *testing.T) {
	logger := zap.NewNop()

	// Build one plog.Logs containing two ResourceLogs blocks, one per cluster.
	ldA := buildK8sLogs("cluster-A-uid", "cluster-A", "pod-a", "v1", false)
	ldB := buildK8sLogs("cluster-B-uid", "cluster-B", "pod-b", "v1", false)
	// Merge ldB's ResourceLogs into ldA so we have a single mixed batch.
	ldB.ResourceLogs().MoveAndAppendTo(ldA.ResourceLogs())
	require.Equal(t, 2, ldA.ResourceLogs().Len(), "test setup: expected two ResourceLogs blocks")

	results := logsmapping.TranslateK8sObjects(ldA, nil, logger, 0)
	require.Len(t, results, 2, "each cluster should get its own result")

	// Index results by cluster ID for stable assertions independent of ordering.
	byID := make(map[string]*logsmapping.K8sTranslationResult, len(results))
	for _, r := range results {
		byID[r.ClusterID] = r
	}
	require.Contains(t, byID, "cluster-A-uid")
	require.Contains(t, byID, "cluster-B-uid")

	// Each cluster's result carries its own name.
	assert.Equal(t, "cluster-A", byID["cluster-A-uid"].ClusterName)
	assert.Equal(t, "cluster-B", byID["cluster-B-uid"].ClusterName)

	// And each cluster's manifests were only counted under that cluster.
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

	// A second ResourceLog with a valid pod body but no cluster attributes.
	ldMissing := plog.NewLogs()
	rl := ldMissing.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	lr := sl.LogRecords().AppendEmpty()
	lr.Body().SetStr(`{"apiVersion":"v1","kind":"Pod","metadata":{"uid":"pod-orphan","resourceVersion":"v1","name":"orphan"}}`)
	ldMissing.ResourceLogs().MoveAndAppendTo(ldGood.ResourceLogs())
	require.Equal(t, 2, ldGood.ResourceLogs().Len(), "test setup: expected two ResourceLogs blocks")

	results := logsmapping.TranslateK8sObjects(ldGood, nil, logger, 0)
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

	// Two different pods in the same cluster — each will be a separate manifest.
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("k8s.cluster.uid", "cluster-chunk-uid")
	rl.Resource().Attributes().PutStr("k8s.cluster.name", "chunk-cluster")
	sl := rl.ScopeLogs().AppendEmpty()

	for _, uid := range []string{"pod-chunk-1", "pod-chunk-2"} {
		lr := sl.LogRecords().AppendEmpty()
		lr.Body().SetStr(`{"apiVersion":"v1","kind":"Pod","metadata":{"uid":"` + uid + `","resourceVersion":"v1","name":"` + uid + `"}}`)
	}

	// maxChunkSize=1 forces one manifest per chunk.
	results := logsmapping.TranslateK8sObjects(ld, nil, logger, 1)
	require.Len(t, results, 1)
	assert.Len(t, results[0].Chunks, 2, "each manifest should be in its own chunk when maxChunkSize=1")
}

// TestTranslateK8sObjects_CrossClusterCacheIsolation verifies that deduplication cache entries
// are scoped per cluster, so the same manifest UID in two different clusters does not suppress
// one another.
func TestTranslateK8sObjects_CrossClusterCacheIsolation(t *testing.T) {
	logger := zap.NewNop()
	cache := logsmapping.NewManifestCache()

	// Build two ResourceLogs blocks that share a manifest UID but belong to different clusters.
	ldA := buildK8sLogs("cluster-A-uid", "cluster-A", "shared-uid", "v1", false)
	ldB := buildK8sLogs("cluster-B-uid", "cluster-B", "shared-uid", "v1", false)

	// Translate cluster A first so "shared-uid" is cached under cluster-A.
	firstA := logsmapping.TranslateK8sObjects(ldA, cache, logger, 0)
	require.Len(t, firstA, 1)
	require.Len(t, firstA[0].Chunks, 1)

	// Translate cluster B with the same UID — it must NOT be suppressed by cluster A's cache entry.
	firstB := logsmapping.TranslateK8sObjects(ldB, cache, logger, 0)
	require.Len(t, firstB, 1, "cluster B's manifest should not be suppressed by cluster A's cache entry")
	require.Len(t, firstB[0].Chunks, 1)

	// A second send to cluster A with the same UID+version should be deduplicated (same cluster).
	ldA2 := buildK8sLogs("cluster-A-uid", "cluster-A", "shared-uid", "v1", false)
	secondA := logsmapping.TranslateK8sObjects(ldA2, cache, logger, 0)
	assert.Empty(t, secondA, "same UID+version within cluster A should be deduplicated")
}

// TestShouldSkipResourceKind tests that secrets and configmaps are rejected.
// This is tested indirectly through ToManifest since shouldSkipResourceKind is an internal function.
func TestShouldSkipResourceKind(t *testing.T) {
	logRecord := plog.NewLogRecord()

	tests := []struct {
		name        string
		kind        string
		expectError bool
	}{
		{
			name:        "secret should be skipped",
			kind:        "Secret",
			expectError: true,
		},
		{
			name:        "configmap should be skipped",
			kind:        "ConfigMap",
			expectError: true,
		},
		{
			name:        "Pod should not be skipped",
			kind:        "Pod",
			expectError: false,
		},
		{
			name:        "Deployment should not be skipped",
			kind:        "Deployment",
			expectError: false,
		},
		{
			name:        "Node should not be skipped",
			kind:        "Node",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyJSON := `{
				"apiVersion": "v1",
				"kind": "` + tt.kind + `",
				"metadata": {
					"uid": "test-uid-123",
					"resourceVersion": "12345",
					"name": "test-resource"
				}
			}`
			logRecord.Body().SetStr(bodyJSON)

			_, _, err := logsmapping.ToManifest(logRecord)
			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "sensitive data")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestGetManifestType verifies that manifests get the correct type based on their kind.
// This is tested indirectly through ToManifest since getManifestType is an internal function.
func TestGetManifestType(t *testing.T) {
	logRecord := plog.NewLogRecord()

	tests := []struct {
		name string
		kind string
	}{
		{name: "Node type", kind: "Node"},
		{name: "Pod type", kind: "Pod"},
		{name: "Deployment type", kind: "Deployment"},
		{name: "Unknown type", kind: "UnknownResource"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyJSON := `{
				"apiVersion": "v1",
				"kind": "` + tt.kind + `",
				"metadata": {
					"uid": "test-uid-123",
					"resourceVersion": "12345",
					"name": "test-resource"
				}
			}`
			logRecord.Body().SetStr(bodyJSON)

			manifest, _, err := logsmapping.ToManifest(logRecord)
			require.NoError(t, err)
			assert.Equal(t, tt.kind, manifest.Kind)
			// Verify that Type field is set (non-negative)
			assert.GreaterOrEqual(t, manifest.Type, int32(0))
		})
	}
}

// TestBuildTags verifies that tags are correctly built from resource and log record attributes.
// This is tested indirectly through ToManifest since buildTags is an internal function.
func TestBuildTags(t *testing.T) {
	// Create test resource with attributes
	resource := pcommon.NewResource()
	resource.Attributes().PutStr("k8s.cluster.name", "test-cluster")
	resource.Attributes().PutStr("k8s.namespace.name", "default")

	// Create test log record with attributes
	logRecord := plog.NewLogRecord()
	logRecord.Attributes().PutStr("k8s.pod.name", "test-pod")
	logRecord.Attributes().PutStr("k8s.container.name", "test-container")

	bodyJSON := `{
		"apiVersion": "v1",
		"kind": "Pod",
		"metadata": {
			"uid": "test-uid-123",
			"resourceVersion": "12345",
			"name": "test-pod"
		}
	}`
	logRecord.Body().SetStr(bodyJSON)

	manifest, _, err := logsmapping.ToManifest(logRecord)
	require.NoError(t, err)

	tags := manifest.Tags

	// Verify common tags are included
	assert.Contains(t, tags, "otel_receiver:k8sobjectsreceiver")
}

func TestBuildManifestFromK8sResource(t *testing.T) {
	resource := pcommon.NewResource()
	resource.Attributes().PutStr("test.resource", "value")
	logRecord := plog.NewLogRecord()
	logRecord.Attributes().PutStr("test.log", "value")

	tests := []struct {
		name          string
		k8sResource   map[string]interface{}
		isTerminated  bool
		expectError   bool
		errorContains string
		validateFn    func(*testing.T, *agentmodel.Manifest)
	}{
		{
			name: "valid pod resource",
			k8sResource: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"uid":             "pod-123",
					"resourceVersion": "12345",
					"name":            "test-pod",
					"namespace":       "default",
				},
				"spec": map[string]interface{}{
					"containers": []interface{}{
						map[string]interface{}{
							"name":  "nginx",
							"image": "nginx:latest",
						},
					},
				},
			},
			isTerminated: false,
			expectError:  false,
			validateFn: func(t *testing.T, m *agentmodel.Manifest) {
				assert.Equal(t, "pod-123", m.Uid)
				assert.Equal(t, "12345", m.ResourceVersion)
				assert.Equal(t, "Pod", m.Kind)
				assert.Equal(t, "v1", m.ApiVersion)
				assert.False(t, m.IsTerminated)
				assert.Equal(t, "application/json", m.ContentType)
				assert.Equal(t, "v1", m.Version)
				assert.NotEmpty(t, m.Content)
			},
		},
		{
			name: "terminated resource (deleted)",
			k8sResource: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"uid":             "pod-456",
					"resourceVersion": "12346",
					"name":            "deleted-pod",
				},
			},
			isTerminated: true,
			expectError:  false,
			validateFn: func(t *testing.T, m *agentmodel.Manifest) {
				assert.Equal(t, "pod-456", m.Uid)
				assert.True(t, m.IsTerminated)
			},
		},
		{
			name: "resource without metadata",
			k8sResource: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
			},
			expectError:   true,
			errorContains: "missing metadata",
		},
		{
			name: "resource without uid",
			k8sResource: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Pod",
				"metadata": map[string]interface{}{
					"resourceVersion": "12345",
					"name":            "test-pod",
				},
			},
			expectError:   true,
			errorContains: "missing uid",
		},
		{
			name: "secret resource (should be skipped)",
			k8sResource: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Secret",
				"metadata": map[string]interface{}{
					"uid":             "secret-123",
					"resourceVersion": "12345",
				},
			},
			expectError:   true,
			errorContains: "contains sensitive data",
		},
		{
			name: "configmap resource (should be skipped)",
			k8sResource: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"uid":             "cm-123",
					"resourceVersion": "12345",
				},
			},
			expectError:   true,
			errorContains: "contains sensitive data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest, err := logsmapping.BuildManifestFromK8sResource(tt.k8sResource, tt.isTerminated)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, manifest)
			if tt.validateFn != nil {
				tt.validateFn(t, manifest)
			}
		})
	}
}

func TestToManifest(t *testing.T) {
	tests := []struct {
		name            string
		bodyJSON        string
		expectError     bool
		errorContains   string
		expectWatchMode bool
		validateFn      func(*testing.T, *agentmodel.Manifest)
	}{
		{
			name: "pull mode - direct k8s object",
			bodyJSON: `{
				"apiVersion": "v1",
				"kind": "Pod",
				"metadata": {
					"uid": "pod-pull-123",
					"resourceVersion": "10001",
					"name": "test-pod",
					"namespace": "default"
				}
			}`,
			expectError:     false,
			expectWatchMode: false,
			validateFn: func(t *testing.T, m *agentmodel.Manifest) {
				assert.Equal(t, "pod-pull-123", m.Uid)
				assert.Equal(t, "10001", m.ResourceVersion)
				assert.Equal(t, "Pod", m.Kind)
				assert.False(t, m.IsTerminated)
			},
		},
		{
			name: "watch mode - ADDED event",
			bodyJSON: `{
				"type": "ADDED",
				"object": {
					"apiVersion": "v1",
					"kind": "Pod",
					"metadata": {
						"uid": "pod-watch-123",
						"resourceVersion": "10002",
						"name": "watched-pod"
					}
				}
			}`,
			expectError:     false,
			expectWatchMode: true,
			validateFn: func(t *testing.T, m *agentmodel.Manifest) {
				assert.Equal(t, "pod-watch-123", m.Uid)
				assert.Equal(t, "10002", m.ResourceVersion)
				assert.Equal(t, "Pod", m.Kind)
				assert.False(t, m.IsTerminated)
			},
		},
		{
			name: "watch mode - MODIFIED event",
			bodyJSON: `{
				"type": "MODIFIED",
				"object": {
					"apiVersion": "v1",
					"kind": "Deployment",
					"metadata": {
						"uid": "deploy-watch-123",
						"resourceVersion": "10003",
						"name": "watched-deployment"
					}
				}
			}`,
			expectError:     false,
			expectWatchMode: true,
			validateFn: func(t *testing.T, m *agentmodel.Manifest) {
				assert.Equal(t, "deploy-watch-123", m.Uid)
				assert.Equal(t, "Deployment", m.Kind)
				assert.False(t, m.IsTerminated)
			},
		},
		{
			name: "watch mode - DELETED event",
			bodyJSON: `{
				"type": "DELETED",
				"object": {
					"apiVersion": "v1",
					"kind": "Pod",
					"metadata": {
						"uid": "pod-deleted-123",
						"resourceVersion": "10004",
						"name": "deleted-pod"
					}
				}
			}`,
			expectError:     false,
			expectWatchMode: true,
			validateFn: func(t *testing.T, m *agentmodel.Manifest) {
				assert.Equal(t, "pod-deleted-123", m.Uid)
				assert.True(t, m.IsTerminated)
			},
		},
		{
			name:          "invalid json",
			bodyJSON:      `{invalid json`,
			expectError:   true,
			errorContains: "failed to unmarshal",
		},
		{
			name: "watch mode - object field not a map",
			bodyJSON: `{
				"type": "ADDED",
				"object": "not a map"
			}`,
			expectError:   true,
			errorContains: "object field in body is not a map",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logRecord := plog.NewLogRecord()
			logRecord.Body().SetStr(tt.bodyJSON)

			manifest, isWatchMode, err := logsmapping.ToManifest(logRecord)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, manifest)
			assert.Equal(t, tt.expectWatchMode, isWatchMode, "Watch mode detection mismatch")
			if tt.validateFn != nil {
				tt.validateFn(t, manifest)
			}
		})
	}
}

func TestToManifestPayload(t *testing.T) {
	hostName := "test-host"
	clusterName := "test-cluster"
	clusterID := "cluster-123"

	manifests := []*agentmodel.Manifest{
		{
			Uid:             "manifest-1",
			ResourceVersion: "v1",
			Kind:            "Pod",
		},
		{
			Uid:             "manifest-2",
			ResourceVersion: "v2",
			Kind:            "Deployment",
		},
	}

	payload := logsmapping.ToManifestPayload(manifests, hostName, clusterName, clusterID, agentmodel.OriginCollector_datadogExporter)

	require.NotNil(t, payload)
	assert.Equal(t, clusterName, payload.ClusterName)
	assert.Equal(t, clusterID, payload.ClusterId)
	assert.Equal(t, hostName, payload.HostName)
	assert.Equal(t, manifests, payload.Manifests)
	assert.Equal(t, agentmodel.OriginCollector_datadogExporter, payload.OriginCollector)
	assert.Contains(t, payload.Tags, "otel_receiver:k8sobjectsreceiver")
}

func TestManifestCacheTTL(t *testing.T) {
	// This test verifies the cache TTL behavior
	// Note: This is a slower test as it involves time.Sleep
	if testing.Short() {
		t.Skip("Skipping TTL test in short mode")
	}

	// Create a fresh cache with a very short TTL for testing
	testCache := gocache.New(100*time.Millisecond, 50*time.Millisecond)

	manifest := &agentmodel.Manifest{
		Uid:             "ttl-test-uid-unique",
		ResourceVersion: "v1",
	}

	// Manually add to the test cache
	testCache.Set(manifest.Uid, manifest.ResourceVersion, 100*time.Millisecond)

	// Immediate check should find the entry
	_, found := testCache.Get(manifest.Uid)
	assert.True(t, found, "Cache should have the entry immediately after setting")

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// After expiry, entry should be gone
	_, found = testCache.Get(manifest.Uid)
	assert.False(t, found, "Cache entry should have expired")
}
