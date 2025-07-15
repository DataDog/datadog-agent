//go:build kubeapiserver && orchestrator

package k8s

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestPersistentVolumeCollector(t *testing.T) {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	filesystem := corev1.PersistentVolumeFilesystem
	parsedResource := resource.MustParse("2Gi")

	persistentVolume := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Finalizers:      []string{"foo.com/x", metav1.FinalizerOrphanDependents, "bar.com/y"},
			Name:            "pv",
			Namespace:       "project",
			ResourceVersion: "1217",
			UID:             types.UID("0ff96226-578d-4679-b3c8-72e8a485c0ef"),
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
						{
							MatchExpressions: []corev1.NodeSelectorRequirement{
								{
									Key:      "test-key3",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"test-value1", "test-value3"},
								},
							}},
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
	client := fake.NewClientset(persistentVolume)

	// Create fake informer factory
	informerFactory := informers.NewSharedInformerFactoryWithOptions(client, 300*time.Second)

	// Create OrchestratorInformerFactory with fake informers
	orchestratorInformerFactory := &collectors.OrchestratorInformerFactory{
		InformerFactory: informerFactory,
	}

	apiClient := &apiserver.APIClient{Cl: client}

	orchestratorCfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	orchestratorCfg.KubeClusterName = "test-cluster"

	runCfg := &collectors.CollectorRunConfig{
		K8sCollectorRunConfig: collectors.K8sCollectorRunConfig{
			APIClient:                   apiClient,
			OrchestratorInformerFactory: orchestratorInformerFactory,
		},
		ClusterID:   "test-cluster",
		Config:      orchestratorCfg,
		MsgGroupRef: atomic.NewInt32(0),
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewPersistentVolumeCollector(metadataAsTags)

	collector.Init(runCfg)

	// Start the informer factory
	stopCh := make(chan struct{})
	defer close(stopCh)
	informerFactory.Start(stopCh)

	// Wait for the informer to sync
	cache.WaitForCacheSync(stopCh, collector.Informer().HasSynced)

	// Run the collector
	result, err := collector.Run(runCfg)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, result.ResourcesListed)
	assert.Equal(t, 1, result.ResourcesProcessed)
	assert.Len(t, result.Result.MetadataMessages, 1)
	assert.Len(t, result.Result.ManifestMessages, 1)
	assert.IsType(t, &model.CollectorPersistentVolume{}, result.Result.MetadataMessages[0])
	assert.IsType(t, &model.CollectorManifest{}, result.Result.ManifestMessages[0])
}
