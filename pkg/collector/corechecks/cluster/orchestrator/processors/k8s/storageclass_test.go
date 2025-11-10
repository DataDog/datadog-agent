// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator && test

package k8s

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	model "github.com/DataDog/agent-payload/v5/process"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestStorageClassHandlers_ExtractResource(t *testing.T) {
	handlers := &StorageClassHandlers{}

	// Create test storageclass
	storageClass := createTestStorageClass()

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

	// Extract resource
	resourceModel := handlers.ExtractResource(ctx, storageClass)

	// Validate extraction
	storageClassModel, ok := resourceModel.(*model.StorageClass)
	assert.True(t, ok)
	assert.NotNil(t, storageClassModel)
	assert.Equal(t, "test-storageclass", storageClassModel.Metadata.Name)
	assert.Equal(t, "test-provisioner", storageClassModel.Provisioner)
	assert.Equal(t, "Retain", storageClassModel.ReclaimPolicy)
	assert.True(t, storageClassModel.AllowVolumeExpansion)
	assert.Equal(t, "WaitForFirstConsumer", storageClassModel.VolumeBindingMode)
	assert.Len(t, storageClassModel.MountOptions, 1)
	assert.Equal(t, "mount-option", storageClassModel.MountOptions[0])
	assert.Len(t, storageClassModel.Parameters, 1)
	assert.Equal(t, "bar", storageClassModel.Parameters["foo"])
}

func TestStorageClassHandlers_ResourceList(t *testing.T) {
	handlers := &StorageClassHandlers{}

	// Create test storageclasses
	storageClass1 := createTestStorageClass()
	storageClass2 := createTestStorageClass()
	storageClass2.Name = "storageclass2"
	storageClass2.UID = "uid2"

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
	resourceList := []*storagev1.StorageClass{storageClass1, storageClass2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*storagev1.StorageClass)
	assert.True(t, ok)
	assert.Equal(t, "test-storageclass", resource1.Name)
	assert.NotSame(t, storageClass1, resource1) // Should be a copy

	resource2, ok := resources[1].(*storagev1.StorageClass)
	assert.True(t, ok)
	assert.Equal(t, "storageclass2", resource2.Name)
	assert.NotSame(t, storageClass2, resource2) // Should be a copy
}

func TestStorageClassHandlers_ResourceUID(t *testing.T) {
	handlers := &StorageClassHandlers{}

	storageClass := createTestStorageClass()
	expectedUID := types.UID("test-storageclass-uid")
	storageClass.UID = expectedUID

	uid := handlers.ResourceUID(nil, storageClass)
	assert.Equal(t, expectedUID, uid)
}

func TestStorageClassHandlers_ResourceVersion(t *testing.T) {
	handlers := &StorageClassHandlers{}

	storageClass := createTestStorageClass()
	expectedVersion := "123"
	storageClass.ResourceVersion = expectedVersion

	version := handlers.ResourceVersion(nil, storageClass, nil)
	assert.Equal(t, expectedVersion, version)
}

func TestStorageClassHandlers_BuildMessageBody(t *testing.T) {
	handlers := &StorageClassHandlers{}

	storageClass1 := createTestStorageClass()
	storageClass2 := createTestStorageClass()
	storageClass2.Name = "storageclass2"

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

	storageClass1Model := k8sTransformers.ExtractStorageClass(ctx, storageClass1)
	storageClass2Model := k8sTransformers.ExtractStorageClass(ctx, storageClass2)

	// Build message body
	resourceModels := []interface{}{storageClass1Model, storageClass2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorStorageClass)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.StorageClasses, 2)
	assert.Equal(t, "test-storageclass", collectorMsg.StorageClasses[0].Metadata.Name)
	assert.Equal(t, "storageclass2", collectorMsg.StorageClasses[1].Metadata.Name)
}

func TestStorageClassHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &StorageClassHandlers{}

	storageClass := createTestStorageClass()

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "StorageClass",
			APIVersion:       "storage.k8s.io/v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	resourceModel := &model.StorageClass{}
	skip := handlers.BeforeMarshalling(ctx, storageClass, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "StorageClass", storageClass.Kind)
	assert.Equal(t, "storage.k8s.io/v1", storageClass.APIVersion)
}

func TestStorageClassHandlers_AfterMarshalling(t *testing.T) {
	handlers := &StorageClassHandlers{}

	storageClass := createTestStorageClass()

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

	// Create resource model
	resourceModel := &model.StorageClass{}

	// Create test YAML
	testYAML := []byte("apiVersion: storage.k8s.io/v1\nkind: StorageClass\nmetadata:\n  name: test-storageclass")

	// Call AfterMarshalling
	skip := handlers.AfterMarshalling(ctx, storageClass, resourceModel, testYAML)

	// Validate
	assert.False(t, skip)
	// Note: StorageClass doesn't store YAML in the model, so we just verify it doesn't skip
}

func TestStorageClassHandlers_GetMetadataTags(t *testing.T) {
	handlers := &StorageClassHandlers{}

	// Create a storageclass model with tags
	storageClassModel := &model.StorageClass{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	tags := handlers.GetMetadataTags(nil, storageClassModel)
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestStorageClassHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &StorageClassHandlers{}

	// Create storageclass with sensitive annotations and labels
	storageClass := createTestStorageClass()
	storageClass.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	storageClass.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, storageClass)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", storageClass.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", storageClass.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestStorageClassProcessor_Process(t *testing.T) {
	// Create test storageclasses with unique UIDs
	storageClass1 := createTestStorageClass()
	storageClass1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	storageClass1.ResourceVersion = "1225"

	storageClass2 := createTestStorageClass()
	storageClass2.Name = "storageclass2"
	storageClass2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	storageClass2.ResourceVersion = "1325"

	// Create fake client
	client := fake.NewClientset(storageClass1, storageClass2)
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
			NodeType:         orchestrator.K8sStorageClass,
			Kind:             "StorageClass",
			APIVersion:       "storage.k8s.io/v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process storageclasses
	processor := processors.NewProcessor(&StorageClassHandlers{})
	result, listed, processed := processor.Process(ctx, []*storagev1.StorageClass{storageClass1, storageClass2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorStorageClass)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.StorageClasses, 2)

	expectedStorageClass1 := k8sTransformers.ExtractStorageClass(ctx, storageClass1)

	assert.Equal(t, expectedStorageClass1.Metadata, metaMsg.StorageClasses[0].Metadata)
	assert.Equal(t, expectedStorageClass1.Provisioner, metaMsg.StorageClasses[0].Provisioner)
	assert.Equal(t, expectedStorageClass1.Parameters, metaMsg.StorageClasses[0].Parameters)
	assert.Equal(t, expectedStorageClass1.ReclaimPolicy, metaMsg.StorageClasses[0].ReclaimPolicy)
	assert.Equal(t, expectedStorageClass1.MountOptions, metaMsg.StorageClasses[0].MountOptions)
	assert.Equal(t, expectedStorageClass1.AllowVolumeExpansion, metaMsg.StorageClasses[0].AllowVolumeExpansion)
	assert.Equal(t, expectedStorageClass1.VolumeBindingMode, metaMsg.StorageClasses[0].VolumeBindingMode)
	assert.Equal(t, expectedStorageClass1.AllowedTopologies, metaMsg.StorageClasses[0].AllowedTopologies)
	assert.Equal(t, expectedStorageClass1.Tags, metaMsg.StorageClasses[0].Tags)

	// Validate manifest message
	manifestMsg, ok := result.ManifestMessages[0].(*model.CollectorManifest)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", manifestMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", manifestMsg.ClusterId)
	assert.Equal(t, int32(1), manifestMsg.GroupId)
	assert.Equal(t, "test-host", manifestMsg.HostName)
	assert.Len(t, manifestMsg.Manifests, 2)
	assert.Equal(t, manifestMsg.OriginCollector, model.OriginCollector_datadogAgent)

	// Validate manifest details
	manifest1 := manifestMsg.Manifests[0]
	assert.Equal(t, storageClass1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, storageClass1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(26), manifest1.Type) // K8sStorageClass
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestStorageClass storagev1.StorageClass
	err := json.Unmarshal(manifest1.Content, &actualManifestStorageClass)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestStorageClass.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestStorageClass.ObjectMeta.CreationTimestamp.Time.UTC()}
	assert.Equal(t, storageClass1.ObjectMeta, actualManifestStorageClass.ObjectMeta)
	assert.Equal(t, storageClass1.Provisioner, actualManifestStorageClass.Provisioner)
	assert.Equal(t, storageClass1.Parameters, actualManifestStorageClass.Parameters)
	assert.Equal(t, storageClass1.ReclaimPolicy, actualManifestStorageClass.ReclaimPolicy)
	assert.Equal(t, storageClass1.MountOptions, actualManifestStorageClass.MountOptions)
	assert.Equal(t, storageClass1.AllowVolumeExpansion, actualManifestStorageClass.AllowVolumeExpansion)
	assert.Equal(t, storageClass1.VolumeBindingMode, actualManifestStorageClass.VolumeBindingMode)
	assert.Equal(t, storageClass1.AllowedTopologies, actualManifestStorageClass.AllowedTopologies)
}

// Helper function to create a test storageclass
func createTestStorageClass() *storagev1.StorageClass {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-storageclass",
			UID:             "test-storageclass-uid",
			ResourceVersion: "1225",
			Labels: map[string]string{
				"app": "test-app",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
			CreationTimestamp: creationTime,
		},
		Provisioner: "test-provisioner",
		Parameters: map[string]string{
			"foo": "bar",
		},
		ReclaimPolicy:        pointer.Ptr(corev1.PersistentVolumeReclaimRetain),
		MountOptions:         []string{"mount-option"},
		AllowVolumeExpansion: pointer.Ptr(true),
		VolumeBindingMode:    pointer.Ptr(storagev1.VolumeBindingWaitForFirstConsumer),
		AllowedTopologies: []corev1.TopologySelectorTerm{
			{
				MatchLabelExpressions: []corev1.TopologySelectorLabelRequirement{
					{
						Key: "topology.kubernetes.io/zone",
						Values: []string{
							"us-central-1a",
							"us-central-1b",
						},
					},
				},
			},
			{
				MatchLabelExpressions: []corev1.TopologySelectorLabelRequirement{
					{
						Key:    "topology.kubernetes.io/region",
						Values: []string{"us-central1"},
					},
				},
			},
		},
	}
}
