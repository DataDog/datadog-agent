// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

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
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	wmutil "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processorstest"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestPersistentVolumeHandlers_BeforeCacheCheck(t *testing.T) {
	resourceModel := &model.PersistentVolume{}
	resource := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-pv",
		},
	}

	ctx := processorstest.NewProcessorContextBeforeCacheCheck("", "persistentvolumes")
	entityID := taggertypes.NewEntityID(
		taggertypes.KubernetesMetadata,
		string(wmutil.GenerateKubeMetadataEntityID(ctx.GetCollectorGroup(), ctx.GetCollectorName(), resource.Namespace, resource.Name)),
	)
	tagger := processorstest.NewFakeTagger(map[taggertypes.EntityID][]string{entityID: {"tagger-tag:value"}})
	handlers := NewPersistentVolumeHandlers(tagger)

	skip := handlers.EnrichModel(ctx, resource, resourceModel)
	assert.False(t, skip)
	assert.Equal(t, []string{"tagger-tag:value"}, resourceModel.Tags)
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
	processor := processors.NewProcessor(NewPersistentVolumeHandlers(processorstest.NewEmptyFakeTagger()))
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
