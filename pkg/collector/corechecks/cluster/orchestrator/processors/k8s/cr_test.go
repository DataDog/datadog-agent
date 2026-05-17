// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator && test

package k8s

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestCRHandlers_ResourceList(t *testing.T) {
	handlers := &CRHandlers{}

	// Create test custom resources
	cr1 := createTestCustomResource("cr-1", "namespace-1")
	cr2 := createTestCustomResource("cr-2", "namespace-2")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Convert list
	resourceList := []runtime.Object{cr1, cr2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*unstructured.Unstructured)
	assert.True(t, ok)
	assert.Equal(t, "cr-1", resource1.GetName())
	assert.NotSame(t, cr1, resource1) // Should be a copy

	resource2, ok := resources[1].(*unstructured.Unstructured)
	assert.True(t, ok)
	assert.Equal(t, "cr-2", resource2.GetName())
	assert.NotSame(t, cr2, resource2) // Should be a copy
}

func TestCRHandlers_ResourceUID(t *testing.T) {
	handlers := &CRHandlers{}

	cr := createTestCustomResource("test-cr", "test-namespace")
	expectedUID := types.UID("test-uid-123")
	cr.SetUID(expectedUID)

	uid := handlers.ResourceUID(nil, cr)
	assert.Equal(t, expectedUID, uid)
}

func TestCRHandlers_ResourceVersion(t *testing.T) {
	handlers := &CRHandlers{}

	cr := createTestCustomResource("test-cr", "test-namespace")
	expectedVersion := "v123"
	cr.SetResourceVersion(expectedVersion)

	version := handlers.ResourceVersion(nil, cr, nil)
	assert.Equal(t, expectedVersion, version)
}

func TestCRHandlers_BuildManifestMessageBody(t *testing.T) {
	handlers := &CRHandlers{}

	// Create test manifest objects
	manifest1 := &model.Manifest{
		Uid:             "test-uid-1",
		ResourceVersion: "1202",
		Type:            int32(orchestrator.K8sCR),
		Version:         "v1",
		ContentType:     "yaml",
		Content:         []byte("test-content-1"),
	}
	manifest2 := &model.Manifest{
		Uid:             "test-uid-2",
		ResourceVersion: "5678",
		Type:            int32(orchestrator.K8sCR),
		Version:         "v1",
		ContentType:     "yaml",
		Content:         []byte("test-content-2"),
	}

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"

	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Build manifest message body
	resourceManifests := []interface{}{manifest1, manifest2}
	messageBody := handlers.BuildManifestMessageBody(ctx, resourceManifests, 2)

	// Validate message body for non-generic resource
	collectorMsg, ok := messageBody.(*model.CollectorManifestCR)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.Manifest.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.Manifest.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.Manifest.GroupId)
	assert.Equal(t, int32(2), collectorMsg.Manifest.GroupSize)
	assert.Len(t, collectorMsg.Manifest.Manifests, 2)
}

func TestCRHandlers_BuildManifestMessageBody_GenericResource(t *testing.T) {
	handlers := &CRHandlers{IsGenericResource: true}

	// Create test manifest objects
	manifest1 := &model.Manifest{
		Uid:             "test-uid-1",
		ResourceVersion: "1234",
		Type:            int32(orchestrator.K8sCR),
		Version:         "v1",
		ContentType:     "yaml",
		Content:         []byte("test-content-1"),
	}
	manifest2 := &model.Manifest{
		Uid:             "test-uid-2",
		ResourceVersion: "5678",
		Type:            int32(orchestrator.K8sCR),
		Version:         "v1",
		ContentType:     "yaml",
		Content:         []byte("test-content-2"),
	}

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"

	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Build manifest message body
	resourceManifests := []interface{}{manifest1, manifest2}
	messageBody := handlers.BuildManifestMessageBody(ctx, resourceManifests, 2)

	// Validate message body for generic resource (should be CollectorManifest)
	collectorMsg, ok := messageBody.(*model.CollectorManifest)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.Manifests, 2)
}

func TestCRHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &CRHandlers{}

	// Create custom resource with sensitive annotations and labels
	cr := createTestCustomResource("test-cr", "test-namespace")
	cr.SetAnnotations(map[string]string{
		"kubectl.kubernetes.io/last-applied-configuration": "secret-value",
		"annotation": "my-annotation",
	})
	cr.SetLabels(map[string]string{
		"kubectl.kubernetes.io/last-applied-configuration": "secret-value",
		"app": "my-app",
	})

	// Create processor context with scrubbing enabled
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.IsScrubbingEnabled = true

	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(ctx, cr)

	// Validate that sensitive data was removed
	annotations := cr.GetAnnotations()
	labels := cr.GetLabels()
	assert.Equal(t, "-", annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", labels["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "my-annotation", annotations["annotation"])
	assert.Equal(t, "my-app", labels["app"])
}

func TestCRProcessor_Process(t *testing.T) {
	// Create test custom resources with unique UIDs
	cr1 := createTestCustomResource("cr-1", "namespace-1")
	cr1.SetUID(types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"))
	cr1.SetResourceVersion("1202")

	cr2 := createTestCustomResource("cr-2", "namespace-2")
	cr2.SetUID(types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7"))
	cr2.SetResourceVersion("1302")

	// Create fake client
	client := fake.NewClientset()
	apiClient := &apiserver.APIClient{Cl: client}

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"

	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			NodeType:         orchestrator.K8sCR,
			Kind:             "MyCustomResource",
			APIVersion:       "example.com/v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process custom resources
	processor := processors.NewProcessor(&CRHandlers{})
	result, listed, processed := processor.Process(ctx, []runtime.Object{cr1, cr2})

	// Assertions
	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)

	// CRs produce manifest messages but no metadata messages
	assert.Nil(t, result.MetadataMessages[0], "MetadataMessages should be nil")
	assert.Len(t, result.ManifestMessages, 1)

	// Validate manifest message
	manifestMsg, ok := result.ManifestMessages[0].(*model.CollectorManifestCR)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", manifestMsg.Manifest.ClusterName)
	assert.Equal(t, "test-cluster-id", manifestMsg.Manifest.ClusterId)
	assert.Equal(t, int32(1), manifestMsg.Manifest.GroupId)
	assert.Equal(t, "test-host", manifestMsg.Manifest.HostName)
	assert.Len(t, manifestMsg.Manifest.Manifests, 2)

	// Validate manifest details for first CR
	manifest1 := manifestMsg.Manifest.Manifests[0]
	assert.Equal(t, cr1.GetUID(), types.UID(manifest1.Uid))
	assert.Equal(t, cr1.GetResourceVersion(), manifest1.ResourceVersion)
	assert.Equal(t, int32(21), manifest1.Type) // K8sCR
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Verify manifest content matches custom resource
	var crYaml map[string]interface{}
	err := yaml.Unmarshal(manifest1.Content, &crYaml)
	assert.NoError(t, err, "YAML should be valid")

	// Check metadata
	metadata, ok := crYaml["metadata"].(map[string]interface{})
	assert.True(t, ok, "metadata should exist")
	assert.Equal(t, "cr-1", metadata["name"])
	assert.Equal(t, "namespace-1", metadata["namespace"])
	assert.Equal(t, string(cr1.GetUID()), metadata["uid"])
	assert.Equal(t, cr1.GetResourceVersion(), metadata["resourceVersion"])

	// Check spec
	spec, ok := crYaml["spec"].(map[string]interface{})
	assert.True(t, ok, "spec should exist")
	assert.Equal(t, int(3), spec["replicas"])
	assert.Equal(t, "nginx:latest", spec["image"])

	// Check status
	status, ok := crYaml["status"].(map[string]interface{})
	assert.True(t, ok, "status should exist")
	assert.Equal(t, true, status["ready"])

	// Validate manifest details for second CR
	manifest2 := manifestMsg.Manifest.Manifests[1]
	assert.Equal(t, cr2.GetUID(), types.UID(manifest2.Uid))
	assert.Equal(t, cr2.GetResourceVersion(), manifest2.ResourceVersion)
	assert.Equal(t, int32(21), manifest2.Type) // K8sCR
	assert.Equal(t, "v1", manifest2.Version)
	assert.Equal(t, "json", manifest2.ContentType)

	// Verify second manifest content
	var crYaml2 map[string]interface{}
	err = yaml.Unmarshal(manifest2.Content, &crYaml2)
	assert.NoError(t, err, "YAML should be valid")

	metadata2, ok := crYaml2["metadata"].(map[string]interface{})
	assert.True(t, ok, "metadata should exist")
	assert.Equal(t, "cr-2", metadata2["name"])
	assert.Equal(t, "namespace-2", metadata2["namespace"])
}

func createTestCustomResource(name, namespace string) *unstructured.Unstructured {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "example.com/v1",
			"kind":       "MyCustomResource",
			"metadata": map[string]interface{}{
				"name":              name,
				"namespace":         namespace,
				"uid":               "test-uid-123",
				"resourceVersion":   "1234",
				"creationTimestamp": creationTime.Format(time.RFC3339),
				"labels": map[string]interface{}{
					"app": "my-app",
				},
				"annotations": map[string]interface{}{
					"annotation": "my-annotation",
				},
			},
			"spec": map[string]interface{}{
				"replicas": float64(3),
				"image":    "nginx:latest",
			},
			"status": map[string]interface{}{
				"ready": true,
			},
		},
	}
}
