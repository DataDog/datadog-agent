// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && test

package k8s

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/tools/cache"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/registry"
)

func TestClusterCollector(t *testing.T) {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	resourceMemory := resource.MustParse("8Gi")
	resourceCPU := resource.MustParse("2000m")

	// Prepare two fake nodes
	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "node-1",
			UID:             types.UID("node-1"),
			ResourceVersion: "1",
		},
		Status: corev1.NodeStatus{
			NodeInfo: corev1.NodeSystemInfo{KubeletVersion: "v1.20.0"},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resourceCPU,
				corev1.ResourceMemory: resourceMemory,
				corev1.ResourcePods:   resource.MustParse("110"),
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resourceCPU,
				corev1.ResourceMemory: resourceMemory,
				corev1.ResourcePods:   resource.MustParse("110"),
			},
		},
	}

	node2 := node1.DeepCopy()
	node2.ObjectMeta.Name = "node-2"
	node2.ObjectMeta.UID = types.UID("node-2")
	node2.ObjectMeta.ResourceVersion = "2"

	// kube-system namespace is required by the ClusterProcessor in order to compute creation timestamp
	kubeSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "kube-system",
			CreationTimestamp: creationTime,
		},
	}

	runCfg, start := registry.NewTestRunConfig(t, []runtime.Object{node1, node2, kubeSystemNS})
	runCfg.APIClient.Cl.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &version.Info{
		Major:      "1",
		Minor:      "20",
		GitVersion: "v1.20.0",
	}
	collector := NewClusterCollector()
	collector.Init(runCfg)
	start()
	require.True(t, cache.WaitForCacheSync(runCfg.StopCh, collector.Informer().HasSynced))

	result, err := collector.Run(runCfg)
	require.NoError(t, err)
	assert.Equal(t, 1, result.ResourcesListed)
	assert.Equal(t, 1, result.ResourcesProcessed)
	require.Len(t, result.Result.MetadataMessages, 1)
	assert.IsType(t, &model.CollectorCluster{}, result.Result.MetadataMessages[0])
	require.Len(t, result.Result.ManifestMessages, 1)
	assert.IsType(t, &model.CollectorManifest{}, result.Result.ManifestMessages[0])
}
