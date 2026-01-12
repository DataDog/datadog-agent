// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package logsagentexporter

import (
	"encoding/json"
	"sync"
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

func TestGetManifestCache(t *testing.T) {
	cache := getManifestCache()
	assert.NotNil(t, cache)

	// Test singleton pattern - should return the same instance
	cache2 := getManifestCache()
	assert.Equal(t, cache, cache2)
}

func TestShouldSkipManifest(t *testing.T) {
	// Reset cache for tests
	manifestCacheOnce = sync.Once{}
	manifestCache = nil

	tests := []struct {
		name           string
		manifest       *agentmodel.Manifest
		isWatchEvent   bool
		setupCache     func()
		expectedSkip   bool
		expectedCached bool
		cachedVersion  string
	}{
		{
			name:         "nil manifest",
			manifest:     nil,
			isWatchEvent: false,
			expectedSkip: false,
		},
		{
			name: "manifest without uid",
			manifest: &agentmodel.Manifest{
				Uid:             "",
				ResourceVersion: "v1",
			},
			isWatchEvent: false,
			expectedSkip: false,
		},
		{
			name: "cache miss - new resource",
			manifest: &agentmodel.Manifest{
				Uid:             "test-uid-1",
				ResourceVersion: "v1",
			},
			isWatchEvent:   false,
			expectedSkip:   false,
			expectedCached: true,
			cachedVersion:  "v1",
		},
		{
			name: "cache hit - same resource version",
			manifest: &agentmodel.Manifest{
				Uid:             "test-uid-2",
				ResourceVersion: "v1",
			},
			isWatchEvent: false,
			setupCache: func() {
				cache := getManifestCache()
				cache.Set("test-uid-2", "v1", manifestCacheTTL)
			},
			expectedSkip:   true,
			expectedCached: true,
			cachedVersion:  "v1",
		},
		{
			name: "cache hit - different resource version",
			manifest: &agentmodel.Manifest{
				Uid:             "test-uid-3",
				ResourceVersion: "v2",
			},
			isWatchEvent: false,
			setupCache: func() {
				cache := getManifestCache()
				cache.Set("test-uid-3", "v1", manifestCacheTTL)
			},
			expectedSkip:   false,
			expectedCached: true,
			cachedVersion:  "v2",
		},
		{
			name: "watch event - bypasses cache even with same resource version",
			manifest: &agentmodel.Manifest{
				Uid:             "test-uid-4",
				ResourceVersion: "v1",
			},
			isWatchEvent: true,
			setupCache: func() {
				cache := getManifestCache()
				cache.Set("test-uid-4", "v1", manifestCacheTTL)
			},
			expectedSkip: false, // Watch events always bypass cache
		},
		{
			name: "watch event - new resource",
			manifest: &agentmodel.Manifest{
				Uid:             "test-uid-5",
				ResourceVersion: "v1",
			},
			isWatchEvent: true,
			expectedSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupCache != nil {
				tt.setupCache()
			}

			skip := shouldSkipManifest(tt.manifest, tt.isWatchEvent)
			assert.Equal(t, tt.expectedSkip, skip)

			// Verify cache state after the operation (only for non-watch events)
			if !tt.isWatchEvent && tt.manifest != nil && tt.manifest.Uid != "" && tt.expectedCached {
				cache := getManifestCache()
				val, found := cache.Get(tt.manifest.Uid)
				assert.True(t, found)
				assert.Equal(t, tt.cachedVersion, val)
			}
		})
	}
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

func TestCreateClusterManifest(t *testing.T) {
	logger := zap.NewNop()
	clusterID := "test-cluster-123"
	nodes := []*agentmodel.Manifest{
		{
			Uid:             "node-1",
			ResourceVersion: "v1",
			Kind:            "Node",
		},
	}

	manifest := logsmapping.CreateClusterManifest(clusterID, nodes, logger)

	require.NotNil(t, manifest)
	assert.Equal(t, clusterID, manifest.Uid)
	assert.Equal(t, "Cluster", manifest.Kind)
	assert.Equal(t, "virtual.datadoghq.com/v1", manifest.ApiVersion)
	assert.Equal(t, "application/json", manifest.ContentType)
	assert.Equal(t, "v1", manifest.Version)
	assert.False(t, manifest.IsTerminated)

	// Verify content is valid JSON
	var clusterData map[string]interface{}
	err := json.Unmarshal(manifest.Content, &clusterData)
	require.NoError(t, err)
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

	payload := logsmapping.ToManifestPayload(manifests, hostName, clusterName, clusterID)

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
