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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestPersistentVolumeClaimHandlers_ExtractResource(t *testing.T) {
	handlers := &PersistentVolumeClaimHandlers{}

	// Create test persistent volume claim
	pvc := createTestPersistentVolumeClaim()

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
	resourceModel := handlers.ExtractResource(ctx, pvc)

	// Validate extraction
	pvcModel, ok := resourceModel.(*model.PersistentVolumeClaim)
	assert.True(t, ok)
	assert.NotNil(t, pvcModel)
	assert.Equal(t, "test-pvc", pvcModel.Metadata.Name)
	assert.Equal(t, "default", pvcModel.Metadata.Namespace)
	assert.NotNil(t, pvcModel.Spec)
	assert.Equal(t, "gold", pvcModel.Spec.StorageClassName)
}

func TestPersistentVolumeClaimHandlers_ResourceList(t *testing.T) {
	handlers := &PersistentVolumeClaimHandlers{}

	// Create test persistent volume claims
	pvc1 := createTestPersistentVolumeClaim()
	pvc2 := createTestPersistentVolumeClaim()
	pvc2.Name = "pvc2"
	pvc2.UID = "uid2"

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
	resourceList := []*corev1.PersistentVolumeClaim{pvc1, pvc2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*corev1.PersistentVolumeClaim)
	assert.True(t, ok)
	assert.Equal(t, "test-pvc", resource1.Name)
	assert.NotSame(t, pvc1, resource1) // Should be a copy

	resource2, ok := resources[1].(*corev1.PersistentVolumeClaim)
	assert.True(t, ok)
	assert.Equal(t, "pvc2", resource2.Name)
	assert.NotSame(t, pvc2, resource2) // Should be a copy
}

func TestPersistentVolumeClaimHandlers_ResourceUID(t *testing.T) {
	handlers := &PersistentVolumeClaimHandlers{}

	pvc := createTestPersistentVolumeClaim()
	expectedUID := types.UID("test-pvc-uid")
	pvc.UID = expectedUID

	uid := handlers.ResourceUID(nil, pvc)
	assert.Equal(t, expectedUID, uid)
}

func TestPersistentVolumeClaimHandlers_ResourceVersion(t *testing.T) {
	handlers := &PersistentVolumeClaimHandlers{}

	pvc := createTestPersistentVolumeClaim()
	expectedVersion := "123"
	pvc.ResourceVersion = expectedVersion

	// Create a mock resource model
	resourceModel := &model.PersistentVolumeClaim{}

	version := handlers.ResourceVersion(nil, pvc, resourceModel)
	assert.Equal(t, expectedVersion, version)
}

func TestPersistentVolumeClaimHandlers_BuildMessageBody(t *testing.T) {
	handlers := &PersistentVolumeClaimHandlers{}

	pvc1 := createTestPersistentVolumeClaim()
	pvc2 := createTestPersistentVolumeClaim()
	pvc2.Name = "pvc2"

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

	pvc1Model := k8sTransformers.ExtractPersistentVolumeClaim(ctx, pvc1)
	pvc2Model := k8sTransformers.ExtractPersistentVolumeClaim(ctx, pvc2)

	// Build message body
	resourceModels := []interface{}{pvc1Model, pvc2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorPersistentVolumeClaim)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.PersistentVolumeClaims, 2)
	assert.Equal(t, "test-pvc", collectorMsg.PersistentVolumeClaims[0].Metadata.Name)
	assert.Equal(t, "pvc2", collectorMsg.PersistentVolumeClaims[1].Metadata.Name)
}

func TestPersistentVolumeClaimHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &PersistentVolumeClaimHandlers{}

	pvc := createTestPersistentVolumeClaim()

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "PersistentVolumeClaim",
			APIVersion:       "v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Create resource model
	resourceModel := &model.PersistentVolumeClaim{}

	// Call BeforeMarshalling
	skip := handlers.BeforeMarshalling(ctx, pvc, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "PersistentVolumeClaim", pvc.Kind)
	assert.Equal(t, "v1", pvc.APIVersion)
}

func TestPersistentVolumeClaimHandlers_AfterMarshalling(t *testing.T) {
	handlers := &PersistentVolumeClaimHandlers{}

	pvc := createTestPersistentVolumeClaim()

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
	resourceModel := &model.PersistentVolumeClaim{}

	// Call AfterMarshalling
	yaml := []byte("test-yaml")
	skip := handlers.AfterMarshalling(ctx, pvc, resourceModel, yaml)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, yaml, resourceModel.Yaml)
}

func TestPersistentVolumeClaimHandlers_GetMetadataTags(t *testing.T) {
	handlers := &PersistentVolumeClaimHandlers{}

	pvc := &model.PersistentVolumeClaim{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	// Get metadata tags
	tags := handlers.GetMetadataTags(nil, pvc)

	// Validate
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestPersistentVolumeClaimHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &PersistentVolumeClaimHandlers{}

	// Create persistent volume claim with sensitive annotations and labels
	pvc := createTestPersistentVolumeClaim()
	pvc.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	pvc.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, pvc)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", pvc.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", pvc.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestPersistentVolumeClaimProcessor_Process(t *testing.T) {
	// Create test persistent volume claims with unique UIDs
	pvc1 := createTestPersistentVolumeClaim()
	pvc1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	pvc1.ResourceVersion = "1216"

	pvc2 := createTestPersistentVolumeClaim()
	pvc2.Name = "pvc2"
	pvc2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	pvc2.ResourceVersion = "1316"

	// Create fake client
	client := fake.NewClientset(pvc1, pvc2)
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
			NodeType:         orchestrator.K8sPersistentVolumeClaim,
			Kind:             "PersistentVolumeClaim",
			APIVersion:       "v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process persistent volume claims
	processor := processors.NewProcessor(&PersistentVolumeClaimHandlers{})
	result, listed, processed := processor.Process(ctx, []*corev1.PersistentVolumeClaim{pvc1, pvc2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorPersistentVolumeClaim)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.PersistentVolumeClaims, 2)

	expectedPvc1 := k8sTransformers.ExtractPersistentVolumeClaim(ctx, pvc1)

	assert.Equal(t, expectedPvc1.Metadata, metaMsg.PersistentVolumeClaims[0].Metadata)
	assert.Equal(t, expectedPvc1.Spec, metaMsg.PersistentVolumeClaims[0].Spec)
	assert.Equal(t, expectedPvc1.Status, metaMsg.PersistentVolumeClaims[0].Status)
	assert.Equal(t, expectedPvc1.Tags, metaMsg.PersistentVolumeClaims[0].Tags)

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
	assert.Equal(t, pvc1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, pvc1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(11), manifest1.Type) // K8sPersistentVolumeClaim
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestPvc corev1.PersistentVolumeClaim
	err := json.Unmarshal(manifest1.Content, &actualManifestPvc)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestPvc.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestPvc.ObjectMeta.CreationTimestamp.Time.UTC()}
	assert.Equal(t, pvc1.ObjectMeta, actualManifestPvc.ObjectMeta)
	assert.Equal(t, pvc1.Spec, actualManifestPvc.Spec)
	assert.Equal(t, pvc1.Status, actualManifestPvc.Status)
}

func createTestPersistentVolumeClaim() *corev1.PersistentVolumeClaim {
	filesystem := corev1.PersistentVolumeFilesystem

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			CreationTimestamp: metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC)),
			Labels: map[string]string{
				"app": "my-app",
			},
			Finalizers:      []string{"foo.com/x", metav1.FinalizerOrphanDependents, "bar.com/y"},
			Name:            "test-pvc",
			Namespace:       "default",
			ResourceVersion: "1216",
			UID:             types.UID("test-pvc-uid"),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany, corev1.ReadWriteOnce},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-sts",
				},
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("2Gi"),
				},
			},
			VolumeName:       "elasticsearch-volume",
			StorageClassName: pointer.Ptr("gold"),
			VolumeMode:       &filesystem,
			DataSource: &corev1.TypedLocalObjectReference{
				Name: "srcpvc",
				Kind: "PersistentVolumeClaim",
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase:       corev1.ClaimLost,
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse("2Gi"),
			},
			Conditions: []corev1.PersistentVolumeClaimCondition{
				{Reason: "OfflineResize"},
			},
		},
	}
}
