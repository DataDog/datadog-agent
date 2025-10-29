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
)

func TestPersistentVolumeHandlers_ExtractResource(t *testing.T) {
	handlers := &PersistentVolumeHandlers{}

	// Create test persistent volume
	pv := createTestPersistentVolume()

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
	resourceModel := handlers.ExtractResource(ctx, pv)

	// Validate extraction
	pvModel, ok := resourceModel.(*model.PersistentVolume)
	assert.True(t, ok)
	assert.NotNil(t, pvModel)
	assert.Equal(t, "test-pv", pvModel.Metadata.Name)
	assert.Equal(t, "default", pvModel.Metadata.Namespace)
	assert.NotNil(t, pvModel.Spec)
	assert.Equal(t, "gold", pvModel.Spec.StorageClassName)
}

func TestPersistentVolumeHandlers_ResourceList(t *testing.T) {
	handlers := &PersistentVolumeHandlers{}

	// Create test persistent volumes
	pv1 := createTestPersistentVolume()
	pv2 := createTestPersistentVolume()
	pv2.Name = "pv2"
	pv2.UID = "uid2"

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
	resourceList := []*corev1.PersistentVolume{pv1, pv2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*corev1.PersistentVolume)
	assert.True(t, ok)
	assert.Equal(t, "test-pv", resource1.Name)
	assert.NotSame(t, pv1, resource1) // Should be a copy

	resource2, ok := resources[1].(*corev1.PersistentVolume)
	assert.True(t, ok)
	assert.Equal(t, "pv2", resource2.Name)
	assert.NotSame(t, pv2, resource2) // Should be a copy
}

func TestPersistentVolumeHandlers_ResourceUID(t *testing.T) {
	handlers := &PersistentVolumeHandlers{}

	pv := createTestPersistentVolume()
	expectedUID := types.UID("test-pv-uid")
	pv.UID = expectedUID

	uid := handlers.ResourceUID(nil, pv)
	assert.Equal(t, expectedUID, uid)
}

func TestPersistentVolumeHandlers_ResourceVersion(t *testing.T) {
	handlers := &PersistentVolumeHandlers{}

	pv := createTestPersistentVolume()
	expectedVersion := "123"
	pv.ResourceVersion = expectedVersion

	// Create a mock resource model
	resourceModel := &model.PersistentVolume{}

	version := handlers.ResourceVersion(nil, pv, resourceModel)
	assert.Equal(t, expectedVersion, version)
}

func TestPersistentVolumeHandlers_BuildMessageBody(t *testing.T) {
	handlers := &PersistentVolumeHandlers{}

	pv1 := createTestPersistentVolume()
	pv2 := createTestPersistentVolume()
	pv2.Name = "pv2"

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

	pv1Model := k8sTransformers.ExtractPersistentVolume(ctx, pv1)
	pv2Model := k8sTransformers.ExtractPersistentVolume(ctx, pv2)

	// Build message body
	resourceModels := []interface{}{pv1Model, pv2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorPersistentVolume)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.PersistentVolumes, 2)
	assert.Equal(t, "test-pv", collectorMsg.PersistentVolumes[0].Metadata.Name)
	assert.Equal(t, "pv2", collectorMsg.PersistentVolumes[1].Metadata.Name)
}

func TestPersistentVolumeHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &PersistentVolumeHandlers{}

	pv := createTestPersistentVolume()

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "PersistentVolume",
			APIVersion:       "v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Create resource model
	resourceModel := &model.PersistentVolume{}

	// Call BeforeMarshalling
	skip := handlers.BeforeMarshalling(ctx, pv, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "PersistentVolume", pv.Kind)
	assert.Equal(t, "v1", pv.APIVersion)
}

func TestPersistentVolumeHandlers_AfterMarshalling(t *testing.T) {
	handlers := &PersistentVolumeHandlers{}

	pv := createTestPersistentVolume()

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
	resourceModel := &model.PersistentVolume{}

	// Call AfterMarshalling
	yaml := []byte("test-yaml")
	skip := handlers.AfterMarshalling(ctx, pv, resourceModel, yaml)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, yaml, resourceModel.Yaml)
}

func TestPersistentVolumeHandlers_GetMetadataTags(t *testing.T) {
	handlers := &PersistentVolumeHandlers{}

	pv := &model.PersistentVolume{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	// Get metadata tags
	tags := handlers.GetMetadataTags(nil, pv)

	// Validate
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestPersistentVolumeHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &PersistentVolumeHandlers{}

	// Create persistent volume with sensitive annotations and labels
	pv := createTestPersistentVolume()
	pv.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	pv.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, pv)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", pv.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", pv.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestPersistentVolumeProcessor_Process(t *testing.T) {
	// Create test persistent volumes with unique UIDs
	pv1 := createTestPersistentVolume()
	pv1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	pv1.ResourceVersion = "1215"

	pv2 := createTestPersistentVolume()
	pv2.Name = "pv2"
	pv2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	pv2.ResourceVersion = "1315"

	// Create fake client
	client := fake.NewClientset(pv1, pv2)
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
			NodeType:         orchestrator.K8sPersistentVolume,
			Kind:             "PersistentVolume",
			APIVersion:       "v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process persistent volumes
	processor := processors.NewProcessor(&PersistentVolumeHandlers{})
	result, listed, processed := processor.Process(ctx, []*corev1.PersistentVolume{pv1, pv2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorPersistentVolume)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.PersistentVolumes, 2)

	expectedPv1 := k8sTransformers.ExtractPersistentVolume(ctx, pv1)

	assert.Equal(t, expectedPv1.Metadata, metaMsg.PersistentVolumes[0].Metadata)
	assert.Equal(t, expectedPv1.Spec, metaMsg.PersistentVolumes[0].Spec)
	assert.Equal(t, expectedPv1.Status, metaMsg.PersistentVolumes[0].Status)
	assert.Equal(t, expectedPv1.Tags, metaMsg.PersistentVolumes[0].Tags)

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
	assert.Equal(t, pv1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, pv1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(10), manifest1.Type) // K8sPersistentVolume
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestPv corev1.PersistentVolume
	err := json.Unmarshal(manifest1.Content, &actualManifestPv)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestPv.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestPv.ObjectMeta.CreationTimestamp.Time.UTC()}
	assert.Equal(t, pv1.ObjectMeta, actualManifestPv.ObjectMeta)
	assert.Equal(t, pv1.Spec, actualManifestPv.Spec)
	assert.Equal(t, pv1.Status, actualManifestPv.Status)
}

func createTestPersistentVolume() *corev1.PersistentVolume {
	filesystem := corev1.PersistentVolumeFilesystem
	parsedResource := resource.MustParse("2Gi")

	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			CreationTimestamp: metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC)),
			Labels: map[string]string{
				"app": "my-app",
			},
			Finalizers:      []string{"foo.com/x", metav1.FinalizerOrphanDependents, "bar.com/y"},
			Name:            "test-pv",
			Namespace:       "default",
			ResourceVersion: "1215",
			UID:             types.UID("test-pv-uid"),
		},
		Spec: corev1.PersistentVolumeSpec{
			MountOptions: []string{"ro", "soft"},
			Capacity:     corev1.ResourceList{corev1.ResourceStorage: parsedResource},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				GCEPersistentDisk: &corev1.GCEPersistentDiskVolumeSource{
					PDName:    "GCE",
					FSType:    "GCE",
					Partition: 10,
					ReadOnly:  false,
				},
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany, corev1.ReadWriteOnce},
			ClaimRef: &corev1.ObjectReference{
				Namespace: "test",
				Name:      "test-pv",
			},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimRetain,
			StorageClassName:              "gold",
			VolumeMode:                    &filesystem,
			NodeAffinity: &corev1.VolumeNodeAffinity{
				Required: &corev1.NodeSelector{
					NodeSelectorTerms: []corev1.NodeSelectorTerm{
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "test-key3",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"test-value1", "test-value3"},
								},
							},
							MatchFields: []corev1.NodeSelectorRequirement{
								{
									Key:      "test-key2",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"test-value0", "test-value2"},
								},
							},
						},
					},
				},
			},
		},
		Status: corev1.PersistentVolumeStatus{
			Phase:   corev1.VolumePending,
			Message: "test",
			Reason:  "test",
		},
	}
}
