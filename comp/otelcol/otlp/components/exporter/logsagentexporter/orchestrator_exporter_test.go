// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package logsagentexporter

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	orchestratormodel "github.com/DataDog/datadog-agent/pkg/orchestrator/model"
	gocache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
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
		setupCache     func()
		expectedSkip   bool
		expectedCached bool
		cachedVersion  string
	}{
		{
			name:         "nil manifest",
			manifest:     nil,
			expectedSkip: false,
		},
		{
			name: "manifest without uid",
			manifest: &agentmodel.Manifest{
				Uid:             "",
				ResourceVersion: "v1",
			},
			expectedSkip: false,
		},
		{
			name: "cache miss - new resource",
			manifest: &agentmodel.Manifest{
				Uid:             "test-uid-1",
				ResourceVersion: "v1",
			},
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
			setupCache: func() {
				cache := getManifestCache()
				cache.Set("test-uid-3", "v1", manifestCacheTTL)
			},
			expectedSkip:   false,
			expectedCached: true,
			cachedVersion:  "v2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupCache != nil {
				tt.setupCache()
			}

			skip := shouldSkipManifest(tt.manifest)
			assert.Equal(t, tt.expectedSkip, skip)

			// Verify cache state after the operation
			if tt.manifest != nil && tt.manifest.Uid != "" && tt.expectedCached {
				cache := getManifestCache()
				val, found := cache.Get(tt.manifest.Uid)
				assert.True(t, found)
				assert.Equal(t, tt.cachedVersion, val)
			}
		})
	}
}

func TestShouldSkipResourceKind(t *testing.T) {
	tests := []struct {
		name         string
		kind         string
		expectedSkip bool
	}{
		{
			name:         "secret should be skipped",
			kind:         "secret",
			expectedSkip: true,
		},
		{
			name:         "Secret (capitalized) should be skipped",
			kind:         "Secret",
			expectedSkip: true,
		},
		{
			name:         "configmap should be skipped",
			kind:         "configmap",
			expectedSkip: true,
		},
		{
			name:         "ConfigMap (capitalized) should be skipped",
			kind:         "ConfigMap",
			expectedSkip: true,
		},
		{
			name:         "Pod should not be skipped",
			kind:         "Pod",
			expectedSkip: false,
		},
		{
			name:         "Deployment should not be skipped",
			kind:         "Deployment",
			expectedSkip: false,
		},
		{
			name:         "Node should not be skipped",
			kind:         "Node",
			expectedSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip := shouldSkipResourceKind(tt.kind)
			assert.Equal(t, tt.expectedSkip, skip)
		})
	}
}

func TestGetManifestType(t *testing.T) {
	tests := []struct {
		name         string
		kind         string
		expectedType int
	}{
		{
			name:         "Node type",
			kind:         "Node",
			expectedType: k8sTypeMap["Node"],
		},
		{
			name:         "Pod type",
			kind:         "Pod",
			expectedType: k8sTypeMap["Pod"],
		},
		{
			name:         "Unknown type",
			kind:         "UnknownResource",
			expectedType: int(orchestratormodel.K8sUnsetType),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifestType := getManifestType(tt.kind)
			assert.Equal(t, tt.expectedType, manifestType)
		})
	}
}

func TestBuildCommonTags(t *testing.T) {
	tags := buildCommonTags()
	assert.Contains(t, tags, "otel_receiver:k8sobjectsreceiver")
	assert.Len(t, tags, 1)
}

func TestBuildTags(t *testing.T) {
	// Create test resource with attributes
	resource := pcommon.NewResource()
	resource.Attributes().PutStr("k8s.cluster.name", "test-cluster")
	resource.Attributes().PutStr("k8s.namespace.name", "default")

	// Create test log record with attributes
	logRecord := plog.NewLogRecord()
	logRecord.Attributes().PutStr("k8s.pod.name", "test-pod")
	logRecord.Attributes().PutStr("k8s.container.name", "test-container")

	tags := buildTags(resource, logRecord)

	// Verify common tags are included
	assert.Contains(t, tags, "otel_receiver:k8sobjectsreceiver")

	// Verify resource attributes are included
	assert.Contains(t, tags, "otel_k8s_cluster_name:test-cluster")
	assert.Contains(t, tags, "otel_k8s_namespace_name:default")

	// Verify log record attributes are included
	assert.Contains(t, tags, "otel_k8s_pod_name:test-pod")
	assert.Contains(t, tags, "otel_k8s_container_name:test-container")
}

func TestBuildManifestFromK8sResource(t *testing.T) {
	ctx := context.Background()
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
				assert.Contains(t, m.Tags, "event_type:delete")
				assert.Contains(t, m.Tags, "source:watch")
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
			manifest, err := buildManifestFromK8sResource(ctx, tt.k8sResource, resource, logRecord, tt.isTerminated)

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
	ctx := context.Background()
	resource := pcommon.NewResource()

	tests := []struct {
		name          string
		bodyJSON      string
		expectError   bool
		errorContains string
		validateFn    func(*testing.T, *agentmodel.Manifest)
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
			expectError: false,
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
			expectError: false,
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
			expectError: false,
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
			expectError: false,
			validateFn: func(t *testing.T, m *agentmodel.Manifest) {
				assert.Equal(t, "pod-deleted-123", m.Uid)
				assert.True(t, m.IsTerminated)
				assert.Contains(t, m.Tags, "event_type:delete")
				assert.Contains(t, m.Tags, "source:watch")
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

			manifest, err := toManifest(ctx, logRecord, resource)

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

func TestCreateClusterManifest(t *testing.T) {
	logger := zap.NewNop()
	clusterID := "test-cluster-123"
	nodeCount := 5

	manifest := createClusterManifest(clusterID, nodeCount, logger)

	require.NotNil(t, manifest)
	assert.Equal(t, clusterID, manifest.Uid)
	assert.Equal(t, "Cluster", manifest.Kind)
	assert.Equal(t, "virtual.datadoghq.com/v1", manifest.ApiVersion)
	assert.Equal(t, "application/json", manifest.ContentType)
	assert.Equal(t, "v1", manifest.Version)
	assert.False(t, manifest.IsTerminated)

	// Verify tags include node count
	assert.Contains(t, manifest.Tags, "synthetic:true")
	assert.Contains(t, manifest.Tags, fmt.Sprintf("node_count:%d", nodeCount))

	// Verify content is valid JSON
	var clusterData map[string]interface{}
	err := json.Unmarshal(manifest.Content, &clusterData)
	require.NoError(t, err)

	// Verify cluster data structure
	assert.Equal(t, "Cluster", clusterData["kind"])
	assert.Equal(t, "virtual.datadoghq.com/v1", clusterData["apiVersion"])

	metadata, ok := clusterData["metadata"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, clusterID, metadata["uid"])

	annotations, ok := metadata["annotations"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, fmt.Sprintf("%d", nodeCount), annotations["node-count"])
	assert.Equal(t, "true", annotations["synthetic"])
}

func TestToManifestPayload(t *testing.T) {
	ctx := context.Background()
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

	payload := toManifestPayload(ctx, manifests, hostName, clusterName, clusterID)

	require.NotNil(t, payload)
	assert.Equal(t, clusterName, payload.ClusterName)
	assert.Equal(t, clusterID, payload.ClusterId)
	assert.Equal(t, hostName, payload.HostName)
	assert.Equal(t, manifests, payload.Manifests)
	assert.Equal(t, agentmodel.OriginCollector_datadogExporter, payload.OriginCollector)
	assert.Contains(t, payload.Tags, "otel_receiver:k8sobjectsreceiver")
}

func TestGetEndpoints(t *testing.T) {
	logger := zap.NewNop()

	tests := []struct {
		name              string
		config            OrchestratorConfig
		expectedEndpoints int
		validateFn        func(*testing.T, map[string]string)
	}{
		{
			name: "single configured endpoint",
			config: OrchestratorConfig{
				Endpoints: map[string]string{
					"https://orchestrator.datadoghq.com": "api-key-1",
				},
				Site: "datadoghq.com",
				Key:  "default-key",
			},
			expectedEndpoints: 1,
			validateFn: func(t *testing.T, endpoints map[string]string) {
				expectedURL := "https://orchestrator.datadoghq.com/api/v2/orchmanif"
				apiKey, exists := endpoints[expectedURL]
				assert.True(t, exists, "Expected URL not found in endpoints")
				assert.Equal(t, "api-key-1", apiKey)
			},
		},
		{
			name: "multiple configured endpoints",
			config: OrchestratorConfig{
				Endpoints: map[string]string{
					"https://orchestrator.datadoghq.com": "api-key-1",
					"https://orchestrator.datadoghq.eu":  "api-key-2",
				},
				Site: "datadoghq.com",
				Key:  "default-key",
			},
			expectedEndpoints: 2,
			validateFn: func(t *testing.T, endpoints map[string]string) {
				expectedURL1 := "https://orchestrator.datadoghq.com/api/v2/orchmanif"
				expectedURL2 := "https://orchestrator.datadoghq.eu/api/v2/orchmanif"

				apiKey1, exists1 := endpoints[expectedURL1]
				assert.True(t, exists1)
				assert.Equal(t, "api-key-1", apiKey1)

				apiKey2, exists2 := endpoints[expectedURL2]
				assert.True(t, exists2)
				assert.Equal(t, "api-key-2", apiKey2)
			},
		},
		{
			name: "no configured endpoints - use default",
			config: OrchestratorConfig{
				Endpoints: map[string]string{},
				Site:      "datadoghq.com",
				Key:       "default-key",
			},
			expectedEndpoints: 1,
			validateFn: func(t *testing.T, endpoints map[string]string) {
				expectedURL := "https://orchestrator.datadoghq.com/api/v2/orchmanif"
				apiKey, exists := endpoints[expectedURL]
				assert.True(t, exists)
				assert.Equal(t, "default-key", apiKey)
			},
		},
		{
			name: "endpoint with custom port",
			config: OrchestratorConfig{
				Endpoints: map[string]string{
					"https://orchestrator.custom.com:8443": "custom-key",
				},
				Site: "datadoghq.com",
				Key:  "default-key",
			},
			expectedEndpoints: 1,
			validateFn: func(t *testing.T, endpoints map[string]string) {
				expectedURL := "https://orchestrator.custom.com:8443/api/v2/orchmanif"
				apiKey, exists := endpoints[expectedURL]
				assert.True(t, exists)
				assert.Equal(t, "custom-key", apiKey)
			},
		},
		{
			name: "skip empty endpoint",
			config: OrchestratorConfig{
				Endpoints: map[string]string{
					"":                                   "should-be-skipped",
					"https://orchestrator.datadoghq.com": "valid-key",
				},
				Site: "datadoghq.com",
				Key:  "default-key",
			},
			expectedEndpoints: 1,
			validateFn: func(t *testing.T, endpoints map[string]string) {
				assert.Len(t, endpoints, 1)
				expectedURL := "https://orchestrator.datadoghq.com/api/v2/orchmanif"
				apiKey, exists := endpoints[expectedURL]
				assert.True(t, exists)
				assert.Equal(t, "valid-key", apiKey)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoints := getEndpoints(tt.config, logger)
			assert.Len(t, endpoints, tt.expectedEndpoints)
			if tt.validateFn != nil {
				tt.validateFn(t, endpoints)
			}
		})
	}
}

func TestSendManifestPayload(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	hostName := "test-host"
	clusterID := "cluster-123"

	// Create a test manifest payload
	manifests := []*agentmodel.Manifest{
		{
			Uid:             "test-manifest-1",
			ResourceVersion: "v1",
			Kind:            "Pod",
			Content:         []byte(`{"test": "data"}`),
		},
	}
	payload := toManifestPayload(ctx, manifests, hostName, "test-cluster", clusterID)

	tests := []struct {
		name           string
		setupServer    func() (*httptest.Server, OrchestratorConfig)
		expectedError  bool
		validateServer func(*testing.T, *http.Request)
	}{
		{
			name: "successful send with 200 response",
			setupServer: func() (*httptest.Server, OrchestratorConfig) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Validate request headers
					assert.Equal(t, "POST", r.Method)
					assert.Equal(t, "application/x-protobuf", r.Header.Get("Content-Type"))
					assert.Equal(t, "test-api-key", r.Header.Get("DD-API-KEY"))
					assert.Equal(t, hostName, r.Header.Get("X-Dd-Hostname"))
					assert.Equal(t, clusterID, r.Header.Get("X-Dd-Orchestrator-ClusterID"))
					assert.Equal(t, "agent", r.Header.Get("DD-EVP-ORIGIN"))
					assert.NotEmpty(t, r.Header.Get("X-DD-Agent-Timestamp"))
					assert.NotEmpty(t, r.Header.Get("DD-EVP-ORIGIN-VERSION"))

					w.WriteHeader(http.StatusOK)
				}))

				config := OrchestratorConfig{
					Endpoints: map[string]string{
						server.URL: "test-api-key",
					},
				}
				return server, config
			},
			expectedError: false,
		},
		{
			name: "successful send with 202 response",
			setupServer: func() (*httptest.Server, OrchestratorConfig) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusAccepted)
				}))

				config := OrchestratorConfig{
					Endpoints: map[string]string{
						server.URL: "test-api-key",
					},
				}
				return server, config
			},
			expectedError: false,
		},
		{
			name: "server returns 500 error",
			setupServer: func() (*httptest.Server, OrchestratorConfig) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				}))

				config := OrchestratorConfig{
					Endpoints: map[string]string{
						server.URL: "test-api-key",
					},
				}
				return server, config
			},
			expectedError: false, // Function doesn't return error, just logs
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, config := tt.setupServer()
			defer server.Close()

			err := sendManifestPayload(ctx, config, payload, hostName, clusterID, logger)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSendManifestPayloadMultipleEndpoints(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	hostName := "test-host"
	clusterID := "cluster-123"

	manifests := []*agentmodel.Manifest{
		{
			Uid:             "test-manifest",
			ResourceVersion: "v1",
			Kind:            "Pod",
		},
	}
	payload := toManifestPayload(ctx, manifests, hostName, "test-cluster", clusterID)

	// Create two test servers
	requestCount1 := 0
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount1++
		assert.Equal(t, "api-key-1", r.Header.Get("DD-API-KEY"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()

	requestCount2 := 0
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount2++
		assert.Equal(t, "api-key-2", r.Header.Get("DD-API-KEY"))
		w.WriteHeader(http.StatusOK)
	}))
	defer server2.Close()

	config := OrchestratorConfig{
		Endpoints: map[string]string{
			server1.URL: "api-key-1",
			server2.URL: "api-key-2",
		},
	}

	err := sendManifestPayload(ctx, config, payload, hostName, clusterID, logger)
	assert.NoError(t, err)

	// Verify both servers received the request
	assert.Equal(t, 1, requestCount1, "Server 1 should receive one request")
	assert.Equal(t, 1, requestCount2, "Server 2 should receive one request")
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
