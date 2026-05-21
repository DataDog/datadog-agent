// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	model "github.com/DataDog/agent-payload/v5/process"
)

const fullClusterInfoPayload = `apiVersion: v1
clusterName: lenaic-karpenter-test
clusterArn: arn:aws:eks:eu-west-3:013364996899:cluster/lenaic-karpenter-test
region: eu-west-3
generatedAt: 2026-05-13T22:42:45.753133827Z
nodeManagement:
    asg:
        eksctl-lenaic-karpenter-test-nodegroup-ng-bar-NodeGroup:
            nodes:
                - ip-10-11-234-221.eu-west-3.compute.internal
    eksManagedNodeGroup:
        ng:
            nodes:
                - ip-10-11-232-187.eu-west-3.compute.internal
                - ip-10-11-234-216.eu-west-3.compute.internal
        ng-foo:
            nodes:
                - ip-10-11-233-156.eu-west-3.compute.internal
    fargate:
        dd-karpenter-lenaic-karpenter-test:
            nodes:
                - fargate-ip-10-11-233-211.eu-west-3.compute.internal
                - fargate-ip-10-11-233-55.eu-west-3.compute.internal
            managedByDatadog: true
    karpenter:
        dd-karpenter-pejpk:
            nodes:
                - ip-10-11-234-171.eu-west-3.compute.internal
                - ip-10-11-234-179.eu-west-3.compute.internal
            managedByDatadog: true
        dd-karpenter-y7msc:
            nodes: []
            managedByDatadog: true
autoscaling:
    clusterAutoscaler:
        present: false
    karpenter:
        present: true
        namespace: dd-karpenter
        name: karpenter
        version: 1.12.1
        managedByDatadog: true
        installerVersion: v0.7.0
    eksAutoMode:
        enabled: false
`

// newFakeClient wraps fake.NewClientset with a reactor that honours
// FieldSelectors on ConfigMap list calls. The default fake client only
// applies the LabelSelector, which would let wrong-named ConfigMaps
// leak through tests that exercise the metadata.name filter.
func newFakeClient(objects ...runtime.Object) *fake.Clientset {
	client := fake.NewClientset(objects...)
	tracker := client.Tracker()
	gvr := corev1.SchemeGroupVersion.WithResource("configmaps")
	gvk := corev1.SchemeGroupVersion.WithKind("ConfigMap")
	client.PrependReactor("list", "configmaps", func(action ktesting.Action) (bool, runtime.Object, error) {
		listAction := action.(ktesting.ListAction)
		obj, err := tracker.List(gvr, gvk, listAction.GetNamespace())
		if err != nil {
			return true, nil, err
		}
		fieldSelector := listAction.GetListRestrictions().Fields
		if fieldSelector == nil || fieldSelector.Empty() {
			return true, obj, nil
		}
		cmList := obj.(*corev1.ConfigMapList)
		filtered := &corev1.ConfigMapList{ListMeta: cmList.ListMeta}
		for _, cm := range cmList.Items {
			if fieldSelector.Matches(fields.Set{
				"metadata.name":      cm.Name,
				"metadata.namespace": cm.Namespace,
			}) {
				filtered.Items = append(filtered.Items, cm)
			}
		}
		return true, filtered, nil
	})
	return client
}

func newClusterInfoConfigMap(namespace, name, payload string, labels map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Data: map[string]string{
			clusterInfoConfigMapDataKey: payload,
		},
	}
}

func TestFetchClusterInfo_Absent(t *testing.T) {
	client := newFakeClient()
	info, err := FetchClusterInfo(context.Background(), client.CoreV1())
	require.NoError(t, err)
	assert.Nil(t, info)
}

func TestFetchClusterInfo_FullPayload(t *testing.T) {
	cm := newClusterInfoConfigMap("dd-karpenter", clusterInfoConfigMapName, fullClusterInfoPayload, map[string]string{
		clusterInfoManagedByLabel: clusterInfoManagedByValue,
	})
	client := newFakeClient(cm)

	info, err := FetchClusterInfo(context.Background(), client.CoreV1())
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.Equal(t, "v1", info.APIVersion)
	assert.Equal(t, "lenaic-karpenter-test", info.ClusterName)
	assert.Equal(t, "arn:aws:eks:eu-west-3:013364996899:cluster/lenaic-karpenter-test", info.ClusterARN)
	assert.Equal(t, "eu-west-3", info.Region)
	assert.False(t, info.GeneratedAt.IsZero())
	assert.True(t, info.Autoscaling.Karpenter.Present)
	assert.Equal(t, "1.12.1", info.Autoscaling.Karpenter.Version)
	assert.True(t, info.Autoscaling.Karpenter.ManagedByDatadog)
	assert.Equal(t, "v0.7.0", info.Autoscaling.Karpenter.InstallerVersion)
	assert.False(t, info.Autoscaling.ClusterAutoscaler.Present)
	assert.False(t, info.Autoscaling.EKSAutoMode.Enabled)
	assert.Len(t, info.NodeManagement, 4)
	assert.Len(t, info.NodeManagement["karpenter"], 2)
	assert.True(t, info.NodeManagement["fargate"]["dd-karpenter-lenaic-karpenter-test"].ManagedByDatadog)
}

func TestFetchClusterInfo_WrongLabel(t *testing.T) {
	cm := newClusterInfoConfigMap("dd-karpenter", clusterInfoConfigMapName, fullClusterInfoPayload, map[string]string{
		"app.kubernetes.io/managed-by": "someone-else",
	})
	client := newFakeClient(cm)

	info, err := FetchClusterInfo(context.Background(), client.CoreV1())
	require.NoError(t, err)
	assert.Nil(t, info)
}

func TestFetchClusterInfo_WrongName(t *testing.T) {
	cm := newClusterInfoConfigMap("dd-karpenter", "some-other-name", fullClusterInfoPayload, map[string]string{
		clusterInfoManagedByLabel: clusterInfoManagedByValue,
	})
	client := newFakeClient(cm)

	info, err := FetchClusterInfo(context.Background(), client.CoreV1())
	require.NoError(t, err)
	assert.Nil(t, info)
}

func TestFetchClusterInfo_MultipleConfigMaps(t *testing.T) {
	labels := map[string]string{clusterInfoManagedByLabel: clusterInfoManagedByValue}
	cmA := newClusterInfoConfigMap("a-ns", clusterInfoConfigMapName, fullClusterInfoPayload, labels)
	cmB := newClusterInfoConfigMap("b-ns", clusterInfoConfigMapName, fullClusterInfoPayload, labels)
	// Insert in reverse-lex order: the selection must be by namespace, not insertion order.
	client := newFakeClient(cmB, cmA)

	info, err := FetchClusterInfo(context.Background(), client.CoreV1())
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "v1", info.APIVersion)
}

func TestFetchClusterInfo_UnknownAPIVersion(t *testing.T) {
	payload := "apiVersion: v2\nclusterName: foo\n"
	cm := newClusterInfoConfigMap("ns", clusterInfoConfigMapName, payload, map[string]string{
		clusterInfoManagedByLabel: clusterInfoManagedByValue,
	})
	client := newFakeClient(cm)

	info, err := FetchClusterInfo(context.Background(), client.CoreV1())
	require.NoError(t, err)
	assert.Nil(t, info)
}

func TestFetchClusterInfo_MissingDataKey(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterInfoConfigMapName,
			Namespace: "ns",
			Labels:    map[string]string{clusterInfoManagedByLabel: clusterInfoManagedByValue},
		},
	}
	client := newFakeClient(cm)

	info, err := FetchClusterInfo(context.Background(), client.CoreV1())
	require.NoError(t, err)
	assert.Nil(t, info)
}

func TestFetchClusterInfo_MalformedYAML(t *testing.T) {
	cm := newClusterInfoConfigMap("ns", clusterInfoConfigMapName, "this: is: not: valid yaml\n", map[string]string{
		clusterInfoManagedByLabel: clusterInfoManagedByValue,
	})
	client := newFakeClient(cm)

	info, err := FetchClusterInfo(context.Background(), client.CoreV1())
	require.Error(t, err)
	assert.Nil(t, info)
}

func TestApplyClusterInfo_NilInfoIsNoOp(t *testing.T) {
	cluster := &model.Cluster{NodeCount: 3}
	nodes := []*model.ClusterNodeInfo{{Name: "n1"}}
	ApplyClusterInfo(cluster, nodes, nil, "any-cluster")
	assert.Equal(t, int32(3), cluster.NodeCount)
	assert.Nil(t, cluster.Autoscaling)
	assert.Equal(t, int64(0), cluster.ClusterInfoGeneratedAtUnixNano)
	assert.Nil(t, cluster.CloudResourceId)
	assert.Empty(t, nodes[0].NodeManager)
}

func TestApplyClusterInfo_NodeEnrichment(t *testing.T) {
	info := mustFetchFromPayload(t, fullClusterInfoPayload)
	cluster := &model.Cluster{}
	nodes := []*model.ClusterNodeInfo{
		{Name: "ip-10-11-234-171.eu-west-3.compute.internal"},
		{Name: "ip-10-11-232-187.eu-west-3.compute.internal"},
		{Name: "fargate-ip-10-11-233-211.eu-west-3.compute.internal"},
		{Name: "added-after-snapshot"},
	}

	ApplyClusterInfo(cluster, nodes, info, "lenaic-karpenter-test")

	assert.Equal(t, "karpenter", nodes[0].NodeManager)
	assert.Equal(t, "dd-karpenter-pejpk", nodes[0].NodeManagerName)
	assert.True(t, nodes[0].NodeManagerManagedByDatadog)

	assert.Equal(t, "eksManagedNodeGroup", nodes[1].NodeManager)
	assert.Equal(t, "ng", nodes[1].NodeManagerName)
	assert.False(t, nodes[1].NodeManagerManagedByDatadog)

	assert.Equal(t, "fargate", nodes[2].NodeManager)
	assert.Equal(t, "dd-karpenter-lenaic-karpenter-test", nodes[2].NodeManagerName)
	assert.True(t, nodes[2].NodeManagerManagedByDatadog)

	assert.Empty(t, nodes[3].NodeManager)
	assert.Empty(t, nodes[3].NodeManagerName)
	assert.False(t, nodes[3].NodeManagerManagedByDatadog)
}

func TestApplyClusterInfo_ClusterFields(t *testing.T) {
	info := mustFetchFromPayload(t, fullClusterInfoPayload)
	cluster := &model.Cluster{}

	ApplyClusterInfo(cluster, nil, info, "lenaic-karpenter-test")

	require.NotNil(t, cluster.Autoscaling)
	require.NotNil(t, cluster.Autoscaling.Karpenter)
	assert.True(t, cluster.Autoscaling.Karpenter.Present)
	assert.Equal(t, "1.12.1", cluster.Autoscaling.Karpenter.Version)
	assert.True(t, cluster.Autoscaling.Karpenter.ManagedByDatadog)
	assert.Equal(t, "v0.7.0", cluster.Autoscaling.Karpenter.InstallerVersion)

	require.NotNil(t, cluster.Autoscaling.ClusterAutoscaler)
	assert.False(t, cluster.Autoscaling.ClusterAutoscaler.Present)

	require.NotNil(t, cluster.Autoscaling.EksAutoMode)
	assert.False(t, cluster.Autoscaling.EksAutoMode.Enabled)

	assert.NotZero(t, cluster.ClusterInfoGeneratedAtUnixNano)

	arnWrapper, ok := cluster.CloudResourceId.(*model.Cluster_Arn)
	require.True(t, ok, "CloudResourceId should be a *Cluster_Arn")
	assert.Equal(t, "arn:aws:eks:eu-west-3:013364996899:cluster/lenaic-karpenter-test", arnWrapper.Arn)
}

func TestApplyClusterInfo_NoArnLeavesOneofUnset(t *testing.T) {
	info := mustFetchFromPayload(t, "apiVersion: v1\nclusterName: foo\ngeneratedAt: 2026-05-13T22:42:45Z\nnodeManagement: {}\nautoscaling:\n  clusterAutoscaler:\n    present: false\n  karpenter:\n    present: false\n  eksAutoMode:\n    enabled: false\n")
	cluster := &model.Cluster{}
	ApplyClusterInfo(cluster, nil, info, "foo")
	assert.Nil(t, cluster.CloudResourceId)
}

func TestApplyClusterInfo_Deterministic(t *testing.T) {
	info := mustFetchFromPayload(t, fullClusterInfoPayload)

	c1 := &model.Cluster{}
	n1 := []*model.ClusterNodeInfo{
		{Name: "ip-10-11-234-171.eu-west-3.compute.internal"},
		{Name: "ip-10-11-232-187.eu-west-3.compute.internal"},
	}
	ApplyClusterInfo(c1, n1, info, "lenaic-karpenter-test")

	c2 := &model.Cluster{}
	n2 := []*model.ClusterNodeInfo{
		{Name: "ip-10-11-234-171.eu-west-3.compute.internal"},
		{Name: "ip-10-11-232-187.eu-west-3.compute.internal"},
	}
	ApplyClusterInfo(c2, n2, info, "lenaic-karpenter-test")

	assert.Equal(t, c1, c2)
	assert.Equal(t, n1, n2)
}

func TestApplyClusterInfo_ClusterNameMismatchStillEnriches(t *testing.T) {
	info := mustFetchFromPayload(t, fullClusterInfoPayload)
	cluster := &model.Cluster{}

	ApplyClusterInfo(cluster, nil, info, "different-cluster-name")

	assert.NotNil(t, cluster.Autoscaling)
	assert.NotZero(t, cluster.ClusterInfoGeneratedAtUnixNano)
}

func TestBuildNodeManagerIndex_OverlappingNodesAreDeterministic(t *testing.T) {
	nm := map[string]map[string]nodeManagerEntry{
		"karpenter": {
			"pool-a": {Nodes: []string{"node-x"}},
		},
		"asg": {
			"group-z": {Nodes: []string{"node-x"}, ManagedByDatadog: true},
		},
	}
	first := buildNodeManagerIndex(nm)["node-x"]
	second := buildNodeManagerIndex(nm)["node-x"]
	assert.Equal(t, first, second)
}

func mustFetchFromPayload(t *testing.T, payload string) *ClusterInfo {
	t.Helper()
	cm := newClusterInfoConfigMap("ns", clusterInfoConfigMapName, payload, map[string]string{
		clusterInfoManagedByLabel: clusterInfoManagedByValue,
	})
	client := newFakeClient(cm)
	info, err := FetchClusterInfo(context.Background(), client.CoreV1())
	require.NoError(t, err)
	require.NotNil(t, info)
	return info
}
