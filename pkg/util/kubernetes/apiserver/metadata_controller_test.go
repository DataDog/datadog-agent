// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubeapiserver

package apiserver

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"
)

func alwaysReady() bool { return true }

func TestMetadataControllerSyncEndpoints(t *testing.T) {
	client := fake.NewSimpleClientset()

	metaController, informerFactory := newFakeMetadataController(client)

	pod1 := newFakePod(
		"default",
		"pod1_name",
		"1111",
		"1.1.1.1",
	)

	pod2 := newFakePod(
		"default",
		"pod2_name",
		"2222",
		"2.2.2.2",
	)

	pod3 := newFakePod(
		"default",
		"pod3_name",
		"3333",
		"3.3.3.3",
	)

	for _, node := range []*v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node3"}},
	} {
		// We are adding objects directly into the store for testing purposes. Do NOT call
		// informerFactory.Start() since the fake apiserver client doesn't actually contain our objects.
		err := informerFactory.
			Core().
			V1().
			Nodes().
			Informer().
			GetStore().
			Add(node)
		require.NoError(t, err)
	}

	tests := []struct {
		desc            string
		delete          bool // whether to add or delete endpoints
		endpoints       *v1.Endpoints
		expectedBundles map[string]ServicesMapper
	}{
		{
			"one service on multiple nodes",
			false,
			&v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "svc1"},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							newFakeEndpointAddress("node1", pod1),
							newFakeEndpointAddress("node2", pod2),
						},
					},
				},
			},
			map[string]ServicesMapper{
				"node1": {
					"default": {
						"pod1_name": sets.NewString("svc1"),
					},
				},
				"node2": {
					"default": {
						"pod2_name": sets.NewString("svc1"),
					},
				},
			},
		},
		{
			"pod added to service",
			false,
			&v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "svc1"},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							newFakeEndpointAddress("node1", pod1),
							newFakeEndpointAddress("node2", pod2),
							newFakeEndpointAddress("node1", pod3),
						},
					},
				},
			},
			map[string]ServicesMapper{
				"node1": {
					"default": {
						"pod1_name": sets.NewString("svc1"),
						"pod3_name": sets.NewString("svc1"),
					},
				},
				"node2": {
					"default": {
						"pod2_name": sets.NewString("svc1"),
					},
				},
			},
		},
		{
			"pod deleted from service",
			false,
			&v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "svc1"},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							newFakeEndpointAddress("node1", pod1),
							newFakeEndpointAddress("node2", pod2),
						},
					},
				},
			},
			map[string]ServicesMapper{
				"node1": {
					"default": {
						"pod1_name": sets.NewString("svc1"),
					},
				},
				"node2": {
					"default": {
						"pod2_name": sets.NewString("svc1"),
					},
				},
			},
		},
		{
			"add service for existing pod",
			false,
			&v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "svc2"},
				Subsets: []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{
							newFakeEndpointAddress("node1", pod1),
						},
					},
				},
			},
			map[string]ServicesMapper{
				"node1": {
					"default": {
						"pod1_name": sets.NewString("svc1", "svc2"),
					},
				},
				"node2": {
					"default": {
						"pod2_name": sets.NewString("svc1"),
					},
				},
			},
		},
		{
			"delete service with pods on multiple nodes",
			true,
			&v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "svc1"},
			},
			map[string]ServicesMapper{
				"node1": {
					"default": {
						"pod1_name": sets.NewString("svc2"),
					},
				},
			},
		},
		{
			"add endpoints for leader election",
			false,
			&v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "leader-election",
					Annotations: map[string]string{
						"control-plane.alpha.kubernetes.io/leader": `{"holderIdentity":"foo"}`,
					},
				},
			},
			map[string]ServicesMapper{ // no changes to cluster metadata
				"node1": {
					"default": {
						"pod1_name": sets.NewString("svc2"),
					},
				},
			},
		},
		{
			"update endpoints for leader election",
			false,
			&v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Name:      "leader-election",
					Annotations: map[string]string{
						"control-plane.alpha.kubernetes.io/leader": `{"holderIdentity":"bar"}`,
					},
				},
			},
			map[string]ServicesMapper{ // no changes to cluster metadata
				"node1": {
					"default": {
						"pod1_name": sets.NewString("svc2"),
					},
				},
			},
		},
		{
			"delete every service",
			true,
			&v1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "svc2"},
			},
			map[string]ServicesMapper{},
		},
	}

	for i, tt := range tests {
		t.Logf("Running step %d %s", i, tt.desc)

		store := informerFactory.
			Core().
			V1().
			Endpoints().
			Informer().
			GetStore()

		var err error
		if tt.delete {
			err = store.Delete(tt.endpoints)
		} else {
			err = store.Add(tt.endpoints)
		}
		require.NoError(t, err)

		key, err := cache.MetaNamespaceKeyFunc(tt.endpoints)
		require.NoError(t, err)

		err = metaController.syncEndpoints(key)
		require.NoError(t, err)

		for nodeName, expectedMapper := range tt.expectedBundles {
			metaBundle, ok := metaController.store.Get(nodeName)
			require.True(t, ok, "No meta bundle for %s", nodeName)
			assert.Equal(t, expectedMapper, metaBundle.Services, nodeName)
		}
	}
}

func TestMetadataController(t *testing.T) {
	client := fake.NewSimpleClientset()

	metaController, informerFactory := newFakeMetadataController(client)

	metaController.endpoints = make(chan struct{}, 1)
	metaController.nodes = make(chan struct{}, 1)

	stop := make(chan struct{})
	defer close(stop)
	informerFactory.Start(stop)
	go metaController.Run(stop)

	c := client.CoreV1()
	require.NotNil(t, c)

	// Create a Ready Schedulable node
	// As we don't have a controller they don't need to have some heartbeat mechanism
	node := &v1.Node{
		Spec: v1.NodeSpec{
			PodCIDR:       "192.168.1.0/24",
			Unschedulable: false,
		},
		Status: v1.NodeStatus{
			Addresses: []v1.NodeAddress{
				{
					Address: "172.31.119.125",
					Type:    "InternalIP",
				},
				{
					Address: "ip-172-31-119-125.eu-west-1.compute.internal",
					Type:    "InternalDNS",
				},
				{
					Address: "ip-172-31-119-125.eu-west-1.compute.internal",
					Type:    "Hostname",
				},
			},
			Conditions: []v1.NodeCondition{
				{
					Type:    "Ready",
					Status:  "True",
					Reason:  "KubeletReady",
					Message: "kubelet is posting ready status",
				},
			},
		},
	}
	node.Name = "ip-172-31-119-125"
	_, err := c.Nodes().Create(node)
	require.NoError(t, err)

	requireReceive(t, metaController.nodes, "nodes")

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
		},
		Spec: v1.PodSpec{
			NodeName: node.Name,
			Containers: []v1.Container{
				{
					Name:  "nginx",
					Image: "nginx:latest",
				},
			},
		},
	}
	pod.Name = "nginx"
	pod.Labels = map[string]string{"app": "nginx"}
	pendingPod, err := c.Pods("default").Create(pod)
	require.NoError(t, err)

	pendingPod.Status = v1.PodStatus{
		Phase:  "Running",
		PodIP:  "172.17.0.1",
		HostIP: "172.31.119.125",
		Conditions: []v1.PodCondition{
			{
				Type:   "Ready",
				Status: "True",
			},
		},
		// mark it ready
		ContainerStatuses: []v1.ContainerStatus{
			{
				Name:  "nginx",
				Ready: true,
				Image: "nginx:latest",
				State: v1.ContainerState{Running: &v1.ContainerStateRunning{StartedAt: metav1.Now()}},
			},
		},
	}
	_, err = c.Pods("default").UpdateStatus(pendingPod)
	require.NoError(t, err)

	svc := &v1.Service{
		Spec: v1.ServiceSpec{
			Selector: map[string]string{
				"app": "nginx",
			},
			Ports: []v1.ServicePort{{Port: 443}},
		},
	}
	svc.Name = "nginx-1"
	_, err = c.Services("default").Create(svc)
	require.NoError(t, err)

	ep := &v1.Endpoints{
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{
						IP:       pendingPod.Status.PodIP,
						NodeName: &node.Name,
						TargetRef: &v1.ObjectReference{
							Kind:      "Pod",
							Namespace: pendingPod.Namespace,
							Name:      pendingPod.Name,
							UID:       pendingPod.UID,
						},
					},
				},
				Ports: []v1.EndpointPort{
					{
						Name:     "https",
						Port:     443,
						Protocol: "TCP",
					},
				},
			},
		},
	}
	ep.Name = "nginx-1"
	_, err = c.Endpoints("default").Create(ep)
	require.NoError(t, err)

	requireReceive(t, metaController.endpoints, "endpoints")

	metadataNames, err := GetPodMetadataNames(node.Name, pod.Namespace, pod.Name)
	require.NoError(t, err)
	assert.Len(t, metadataNames, 1)
	assert.Contains(t, metadataNames, "kube_service:nginx-1")

	// Add a new service/endpoint on the nginx Pod
	svc.Name = "nginx-2"
	_, err = c.Services("default").Create(svc)
	require.NoError(t, err)

	ep.Name = "nginx-2"
	_, err = c.Endpoints("default").Create(ep)
	require.NoError(t, err)

	requireReceive(t, metaController.endpoints, "endpoints")

	metadataNames, err = GetPodMetadataNames(node.Name, pod.Namespace, pod.Name)
	require.NoError(t, err)
	assert.Len(t, metadataNames, 2)
	assert.Contains(t, metadataNames, "kube_service:nginx-1")
	assert.Contains(t, metadataNames, "kube_service:nginx-2")

	cl := &APIClient{Cl: client, timeoutSeconds: 5}

	fullmapper, errList := GetMetadataMapBundleOnAllNodes(cl)
	require.Nil(t, errList)
	list := fullmapper["Nodes"]
	assert.Contains(t, list, "ip-172-31-119-125")
	fullMap := list.(map[string]*MetadataMapperBundle)
	services, found := fullMap["ip-172-31-119-125"].ServicesForPod(metav1.NamespaceDefault, "nginx")
	assert.True(t, found)
	assert.Contains(t, services, "nginx-1")

	err = c.Nodes().Delete(node.Name, &metav1.DeleteOptions{})
	require.NoError(t, err)

	requireReceive(t, metaController.nodes, "nodes")

	_, err = GetMetadataMapBundleOnNode(node.Name)
	require.Error(t, err)
}

func newFakeMetadataController(client kubernetes.Interface) (*MetadataController, informers.SharedInformerFactory) {
	informerFactory := informers.NewSharedInformerFactory(client, 0)

	metaController := NewMetadataController(
		informerFactory.Core().V1().Nodes(),
		informerFactory.Core().V1().Endpoints(),
	)
	metaController.nodeListerSynced = alwaysReady
	metaController.endpointsListerSynced = alwaysReady

	return metaController, informerFactory
}

func requireReceive(t *testing.T, ch chan struct{}, msgAndArgs ...interface{}) {
	timeout := time.NewTimer(2 * time.Second)

	select {
	case <-ch:
	case <-timeout.C:
		require.FailNow(t, "Timeout waiting to receive from channel", msgAndArgs...)
	}
}
