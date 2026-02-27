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
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestCRDHandlers_ResourceList(t *testing.T) {
	handlers := &CRDHandlers{}

	// Create test custom resource definitions
	crd1 := createTestCustomResourceDefinition("crd-1")
	crd2 := createTestCustomResourceDefinition("crd-2")

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
	resourceList := []runtime.Object{crd1, crd2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*v1.CustomResourceDefinition)
	assert.True(t, ok)
	assert.Equal(t, "crd-1", resource1.Name)
	assert.NotSame(t, crd1, resource1) // Should be a copy

	resource2, ok := resources[1].(*v1.CustomResourceDefinition)
	assert.True(t, ok)
	assert.Equal(t, "crd-2", resource2.Name)
	assert.NotSame(t, crd2, resource2) // Should be a copy
}

func TestCRDHandlers_ResourceUID(t *testing.T) {
	handlers := &CRDHandlers{}

	crd := createTestCustomResourceDefinition("test-crd")
	expectedUID := types.UID("test-uid-123")
	crd.UID = expectedUID

	uid := handlers.ResourceUID(nil, crd)
	assert.Equal(t, expectedUID, uid)
}

func TestCRDHandlers_ResourceVersion(t *testing.T) {
	handlers := &CRDHandlers{}

	crd := createTestCustomResourceDefinition("test-crd")
	expectedVersion := "v123"
	crd.ResourceVersion = expectedVersion

	version := handlers.ResourceVersion(nil, crd, nil)
	assert.Equal(t, expectedVersion, version)
}

func TestCRDHandlers_BuildManifestMessageBody(t *testing.T) {
	handlers := &CRDHandlers{}

	// Create test manifest objects
	manifest1 := &model.Manifest{
		Uid:             "test-uid-1",
		ResourceVersion: "1203",
		Type:            int32(orchestrator.K8sCRD),
		Version:         "v1",
		ContentType:     "json",
		Content:         []byte("test-content-1"),
	}
	manifest2 := &model.Manifest{
		Uid:             "test-uid-2",
		ResourceVersion: "5678",
		Type:            int32(orchestrator.K8sCRD),
		Version:         "v1",
		ContentType:     "json",
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

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorManifestCRD)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.Manifest.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.Manifest.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.Manifest.GroupId)
	assert.Equal(t, int32(2), collectorMsg.Manifest.GroupSize)
	assert.Len(t, collectorMsg.Manifest.Manifests, 2)
}

func TestCRDHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &CRDHandlers{}

	crd := createTestCustomResourceDefinition("test-crd")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "CustomResourceDefinition",
			APIVersion:       "apiextensions.k8s.io/v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Call BeforeMarshalling
	skip := handlers.BeforeMarshalling(ctx, crd, nil)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "CustomResourceDefinition", crd.Kind)
	assert.Equal(t, "apiextensions.k8s.io/v1", crd.APIVersion)
}

func TestCRDHandlers_AfterMarshalling(t *testing.T) {
	handlers := &CRDHandlers{}

	crd := createTestCustomResourceDefinition("test-crd")

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

	// Test YAML
	testYAML := []byte(`{"apiVersion":"apiextensions.k8s.io/v1","kind":"CustomResourceDefinition","metadata":{"name":"test"}}`)

	skip := handlers.AfterMarshalling(ctx, crd, nil, testYAML)
	assert.False(t, skip)
}

func TestCRDHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &CRDHandlers{}

	// Create custom resource definition with sensitive annotations and labels
	crd := createTestCustomResourceDefinition("test-crd")
	crd.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	crd.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

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

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(ctx, crd)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", crd.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", crd.Labels["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "my-annotation", crd.Annotations["annotation"])
	assert.Equal(t, "my-app", crd.Labels["app"])
}

func TestCRDProcessor_Process(t *testing.T) {
	// Create test custom resource definitions with unique UIDs
	crd1 := createTestCustomResourceDefinition("crd-1")
	crd1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	crd1.ResourceVersion = "1203"

	crd2 := createTestCustomResourceDefinition("crd-2")
	crd2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	crd2.ResourceVersion = "1303"

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
			NodeType:         orchestrator.K8sCRD,
			Kind:             "CustomResourceDefinition",
			APIVersion:       "apiextensions.k8s.io/v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process custom resource definitions
	processor := processors.NewProcessor(&CRDHandlers{})
	result, listed, processed := processor.Process(ctx, []runtime.Object{crd1, crd2})

	// Assertions
	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)

	// CRDs produce manifest messages but no metadata messages
	assert.Len(t, result.MetadataMessages, 1)
	assert.Nil(t, result.MetadataMessages[0], "MetadataMessages should be nil")
	assert.Len(t, result.ManifestMessages, 1)

	// Validate manifest message
	manifestMsg, ok := result.ManifestMessages[0].(*model.CollectorManifestCRD)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", manifestMsg.Manifest.ClusterName)
	assert.Equal(t, "test-cluster-id", manifestMsg.Manifest.ClusterId)
	assert.Equal(t, int32(1), manifestMsg.Manifest.GroupId)
	assert.Equal(t, "test-host", manifestMsg.Manifest.HostName)
	assert.Len(t, manifestMsg.Manifest.Manifests, 2)

	// Validate manifest details for first CRD
	manifest1 := manifestMsg.Manifest.Manifests[0]
	assert.Equal(t, crd1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, crd1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(20), manifest1.Type) // K8sCRD
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Verify manifest content matches custom resource definition
	var crdYaml map[string]interface{}
	err := yaml.Unmarshal(manifest1.Content, &crdYaml)
	assert.NoError(t, err, "YAML should be valid")

	// Check metadata
	metadata, ok := crdYaml["metadata"].(map[string]interface{})
	assert.True(t, ok, "metadata should exist")
	assert.Equal(t, "crd-1", metadata["name"])
	assert.Equal(t, string(crd1.UID), metadata["uid"])
	assert.Equal(t, crd1.ResourceVersion, metadata["resourceVersion"])

	// Check spec
	spec, ok := crdYaml["spec"].(map[string]interface{})
	assert.True(t, ok, "spec should exist")
	assert.Equal(t, "example.com", spec["group"])

	// Check names
	names, ok := spec["names"].(map[string]interface{})
	assert.True(t, ok, "names should exist")
	assert.Equal(t, "MyCustomResource", names["kind"])
	assert.Equal(t, "mycustomresources", names["plural"])
	assert.Equal(t, "mycustomresource", names["singular"])

	// Check versions
	versions, ok := spec["versions"].([]interface{})
	assert.True(t, ok, "versions should exist")
	assert.Len(t, versions, 1)

	version, ok := versions[0].(map[string]interface{})
	assert.True(t, ok, "version should be a map")
	assert.Equal(t, "v1", version["name"])
	assert.Equal(t, true, version["served"])
	assert.Equal(t, true, version["storage"])

	// Validate manifest details for second CRD
	manifest2 := manifestMsg.Manifest.Manifests[1]
	assert.Equal(t, crd2.UID, types.UID(manifest2.Uid))
	assert.Equal(t, crd2.ResourceVersion, manifest2.ResourceVersion)
	assert.Equal(t, int32(20), manifest2.Type) // K8sCRD
	assert.Equal(t, "v1", manifest2.Version)
	assert.Equal(t, "json", manifest2.ContentType)

	// Verify second manifest content
	var crdYaml2 map[string]interface{}
	err = yaml.Unmarshal(manifest2.Content, &crdYaml2)
	assert.NoError(t, err, "YAML should be valid")

	metadata2, ok := crdYaml2["metadata"].(map[string]interface{})
	assert.True(t, ok, "metadata should exist")
	assert.Equal(t, "crd-2", metadata2["name"])
}

func createTestCustomResourceDefinition(name string) *v1.CustomResourceDefinition {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	return &v1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			UID:               "test-crd-uid-123",
			ResourceVersion:   "1234",
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
		},
		Spec: v1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: v1.CustomResourceDefinitionNames{
				Kind:     "MyCustomResource",
				ListKind: "MyCustomResourceList",
				Plural:   "mycustomresources",
				Singular: "mycustomresource",
			},
			Scope: v1.NamespaceScoped,
			Versions: []v1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &v1.CustomResourceValidation{
						OpenAPIV3Schema: &v1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]v1.JSONSchemaProps{
								"spec": {
									Type: "object",
									Properties: map[string]v1.JSONSchemaProps{
										"replicas": {Type: "integer"},
										"image":    {Type: "string"},
									},
								},
							},
						},
					},
				},
			},
		},
		Status: v1.CustomResourceDefinitionStatus{
			Conditions: []v1.CustomResourceDefinitionCondition{
				{
					Type:   v1.Established,
					Status: v1.ConditionTrue,
				},
			},
		},
	}
}
