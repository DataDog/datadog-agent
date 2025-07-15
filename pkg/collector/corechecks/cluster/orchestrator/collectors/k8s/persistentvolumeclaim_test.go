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
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestPersistentVolumeClaimCollector(t *testing.T) {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	filesystem := corev1.PersistentVolumeFilesystem

	persistentVolumeClaim := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Finalizers:      []string{"foo.com/x", metav1.FinalizerOrphanDependents, "bar.com/y"},
			Name:            "pvc",
			Namespace:       "project",
			ResourceVersion: "1218",
			UID:             types.UID("0ff96226-578d-4679-b3c8-72e8a485c0ef"),
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
	client := fake.NewClientset(persistentVolumeClaim)

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
	collector := NewPersistentVolumeClaimCollector(metadataAsTags)

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
	assert.IsType(t, &model.CollectorPersistentVolumeClaim{}, result.Result.MetadataMessages[0])
	assert.IsType(t, &model.CollectorManifest{}, result.Result.ManifestMessages[0])
}
