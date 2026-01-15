// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator && test

package k8s

import (
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestClusterProcessor_fillClusterResourceVersion(t *testing.T) {
	cluster := &model.Cluster{}
	fillClusterResourceVersion(cluster)
	assert.NotEmpty(t, cluster.ResourceVersion)
}

func TestClusterProcessor_getKubeSystemCreationTimeStamp(t *testing.T) {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	kubeSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "kube-system",
			CreationTimestamp: creationTime,
		},
	}

	client := fake.NewClientset(kubeSystemNS)
	ts, err := getKubeSystemCreationTimeStamp(client.CoreV1())
	assert.NoError(t, err)
	assert.Equal(t, creationTime, ts)
}

func TestClusterProcessor_Process_Success(t *testing.T) {
	// Create test nodes
	node1 := createTestClusterNode("node-1", "v1.20.0", "1000m", "2Gi", "110")
	node2 := createTestClusterNode("node-2", "v1.20.0", "2000m", "4Gi", "220")
	node3 := createTestClusterNode("node-3", "v1.21.0", "1500m", "3Gi", "165")

	// Create kube-system namespace for cluster creation timestamp
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	kubeSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "kube-system",
			CreationTimestamp: creationTime,
		},
	}

	// Create fake client
	client := fake.NewClientset(node1, node2, node3, kubeSystemNS)

	// Configure fake discovery client
	fakeDiscoveryClient := client.Discovery().(*fakediscovery.FakeDiscovery)
	fakeDiscoveryClient.FakedServerVersion = &version.Info{
		Major:      "1",
		Minor:      "20",
		GitVersion: "v1.20.0",
	}

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
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process nodes
	processor := NewClusterProcessor()
	result, processed, err := processor.Process(ctx, []*corev1.Node{node1, node2, node3})

	// Assertions
	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorCluster)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)

	cluster := metaMsg.Cluster
	expectedResourceMemory := resource.MustParse("9Gi")
	expectedResourceCPU := resource.MustParse("4500m")

	assert.Equal(t, int32(3), cluster.NodeCount)
	assert.Equal(t, map[string]int32{"v1.20.0": 2, "v1.21.0": 1}, cluster.KubeletVersions)
	assert.Equal(t, map[string]int32{"v1.20.0": 1}, cluster.ApiServerVersions)
	assert.Equal(t, uint32(495), cluster.PodCapacity)
	assert.Equal(t, uint32(495), cluster.PodAllocatable)
	assert.Equal(t, uint64(expectedResourceMemory.Value()), cluster.MemoryCapacity)
	assert.Equal(t, uint64(expectedResourceMemory.Value()), cluster.MemoryAllocatable)
	assert.Equal(t, uint64(expectedResourceCPU.MilliValue()), cluster.CpuCapacity)
	assert.Equal(t, uint64(expectedResourceCPU.MilliValue()), cluster.CpuAllocatable)
	assert.Equal(t, creationTime.Unix(), cluster.CreationTimestamp)
	assert.NotEmpty(t, cluster.ResourceVersion)
	assert.Len(t, cluster.NodesInfo, 3)

	// Validate manifest message
	manifestMsg, ok := result.ManifestMessages[0].(*model.CollectorManifest)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", manifestMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", manifestMsg.ClusterId)
	assert.Equal(t, int32(1), manifestMsg.GroupId)
	assert.Equal(t, "test-host", manifestMsg.HostName)
	assert.Len(t, manifestMsg.Manifests, 1)
	assert.Equal(t, manifestMsg.OriginCollector, model.OriginCollector_datadogAgent)

	manifest := manifestMsg.Manifests[0]
	assert.Equal(t, "test-cluster-id", manifest.Uid)
	assert.Equal(t, cluster.ResourceVersion, manifest.ResourceVersion)
	assert.Equal(t, int32(5), manifest.Type) // K8sCluster
	assert.Equal(t, "v1", manifest.Version)
	assert.Equal(t, "json", manifest.ContentType)

	// Verify manifest content matches cluster model
	var clusterFromManifest model.Cluster
	err = json.Unmarshal(manifest.Content, &clusterFromManifest)
	assert.NoError(t, err)
	if clusterFromManifest.ExtendedResourcesAllocatable == nil {
		clusterFromManifest.ExtendedResourcesAllocatable = make(map[string]int64)
	}
	if clusterFromManifest.ExtendedResourcesCapacity == nil {
		clusterFromManifest.ExtendedResourcesCapacity = make(map[string]int64)
	}

	sort.Slice(clusterFromManifest.NodesInfo, func(i, j int) bool {
		return clusterFromManifest.NodesInfo[i].Name < clusterFromManifest.NodesInfo[j].Name
	})
	assert.Equal(t, *cluster, clusterFromManifest)
}

func TestClusterProcessor_ExtendedResources(t *testing.T) {
	// Create node with extended resources
	node := createTestClusterNode("node-1", "v1.20.0", "1000m", "2Gi", "110")

	// Add extended resources
	node.Status.Capacity[corev1.ResourceName("nvidia.com/gpu")] = resource.MustParse("4")
	node.Status.Capacity[corev1.ResourceName("example.com/foo")] = resource.MustParse("10")
	node.Status.Allocatable[corev1.ResourceName("nvidia.com/gpu")] = resource.MustParse("2")
	node.Status.Allocatable[corev1.ResourceName("example.com/foo")] = resource.MustParse("5")

	// Create kube-system namespace
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	kubeSystemNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "kube-system",
			CreationTimestamp: creationTime,
		},
	}

	client := fake.NewClientset(node, kubeSystemNS)
	fakeDiscoveryClient := client.Discovery().(*fakediscovery.FakeDiscovery)
	fakeDiscoveryClient.FakedServerVersion = &version.Info{
		Major:      "1",
		Minor:      "20",
		GitVersion: "v1.20.0",
	}

	apiClient := &apiserver.APIClient{Cl: client}
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
		APIClient: apiClient,
		HostName:  "test-host",
	}

	processor := NewClusterProcessor()
	result, processed, err := processor.Process(ctx, []*corev1.Node{node})

	require.NoError(t, err)
	assert.Equal(t, 1, processed)

	metaMsg := result.MetadataMessages[0].(*model.CollectorCluster)
	cluster := metaMsg.Cluster

	// Verify extended resources are included
	expectedCapacity := map[string]int64{
		"nvidia.com/gpu":  4,
		"example.com/foo": 10,
	}
	expectedAllocatable := map[string]int64{
		"nvidia.com/gpu":  2,
		"example.com/foo": 5,
	}

	assert.Equal(t, expectedCapacity, cluster.ExtendedResourcesCapacity)
	assert.Equal(t, expectedAllocatable, cluster.ExtendedResourcesAllocatable)
}

func createTestClusterNode(name, kubeletVersion, cpu, memory, pods string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			UID:             types.UID(name),
			ResourceVersion: "1",
		},
		Status: corev1.NodeStatus{
			NodeInfo: corev1.NodeSystemInfo{
				KubeletVersion: kubeletVersion,
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cpu),
				corev1.ResourceMemory: resource.MustParse(memory),
				corev1.ResourcePods:   resource.MustParse(pods),
			},
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(cpu),
				corev1.ResourceMemory: resource.MustParse(memory),
				corev1.ResourcePods:   resource.MustParse(pods),
			},
		},
	}
}
