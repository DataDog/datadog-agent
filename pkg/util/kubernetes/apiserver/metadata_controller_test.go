// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apiv1 "github.com/DataDog/datadog-agent/pkg/clusteragent/api/v1"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	gocache "github.com/patrickmn/go-cache"
)

func TestMetadataControllerSyncEndpoints(t *testing.T) {
	client := fake.NewSimpleClientset()

	metaController, informerFactory := newFakeMetadataController(client)

	// don't use the global store so we can can inspect the store without
	// it being modified by other tests.
	metaController.store = &metaBundleStore{
		cache: gocache.New(gocache.NoExpiration, 5*time.Second),
	}

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
		expectedBundles map[string]apiv1.NamespacesPodsStringsSet
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
			map[string]apiv1.NamespacesPodsStringsSet{
				"node1": {
					"default": {
						"pod1_name": sets.New("svc1"),
					},
				},
				"node2": {
					"default": {
						"pod2_name": sets.New("svc1"),
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
			map[string]apiv1.NamespacesPodsStringsSet{
				"node1": {
					"default": {
						"pod1_name": sets.New("svc1"),
						"pod3_name": sets.New("svc1"),
					},
				},
				"node2": {
					"default": {
						"pod2_name": sets.New("svc1"),
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
			map[string]apiv1.NamespacesPodsStringsSet{
				"node1": {
					"default": {
						"pod1_name": sets.New("svc1"),
					},
				},
				"node2": {
					"default": {
						"pod2_name": sets.New("svc1"),
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
			map[string]apiv1.NamespacesPodsStringsSet{
				"node1": {
					"default": {
						"pod1_name": sets.New("svc1", "svc2"),
					},
				},
				"node2": {
					"default": {
						"pod2_name": sets.New("svc1"),
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
			map[string]apiv1.NamespacesPodsStringsSet{
				"node1": {
					"default": {
						"pod1_name": sets.New("svc2"),
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
			map[string]apiv1.NamespacesPodsStringsSet{ // no changes to cluster metadata
				"node1": {
					"default": {
						"pod1_name": sets.New("svc2"),
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
			map[string]apiv1.NamespacesPodsStringsSet{ // no changes to cluster metadata
				"node1": {
					"default": {
						"pod1_name": sets.New("svc2"),
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
			map[string]apiv1.NamespacesPodsStringsSet{},
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
			metaBundle, ok := metaController.store.get(nodeName)
			require.True(t, ok, "No meta bundle for %s", nodeName)
			assert.Equal(t, expectedMapper, metaBundle.Services, nodeName)
		}
	}
}

func TestMetadataController(t *testing.T) {
	// FIXME: Updating to k8s.io/client-go v0.9+ should allow revert this PR https://github.com/DataDog/datadog-agent/pull/2524
	// that allows a more fine-grain testing on the controller lifecycle (affected by bug https://github.com/kubernetes/kubernetes/pull/66078)
	client := fake.NewSimpleClientset()

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
	_, err := c.Nodes().Create(context.TODO(), node, metav1.CreateOptions{})
	require.NoError(t, err)

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
	pendingPod, err := c.Pods("default").Create(context.TODO(), pod, metav1.CreateOptions{})
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
	_, err = c.Pods("default").UpdateStatus(context.TODO(), pendingPod, metav1.UpdateOptions{})
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
	_, err = c.Services("default").Create(context.TODO(), svc, metav1.CreateOptions{})
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
	_, err = c.Endpoints("default").Create(context.TODO(), ep, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add a new service/endpoint on the nginx Pod
	svc.Name = "nginx-2"
	_, err = c.Services("default").Create(context.TODO(), svc, metav1.CreateOptions{})
	require.NoError(t, err)

	ep.Name = "nginx-2"
	_, err = c.Endpoints("default").Create(context.TODO(), ep, metav1.CreateOptions{})
	require.NoError(t, err)

	metaController, informerFactory := newFakeMetadataController(client)

	stop := make(chan struct{})
	defer close(stop)
	informerFactory.Start(stop)
	go metaController.Run(stop)

	testutil.AssertTrueBeforeTimeout(t, 100*time.Millisecond, 2*time.Second, func() bool {
		return metaController.endpointsListerSynced() && metaController.nodeListerSynced()
	})

	testutil.AssertTrueBeforeTimeout(t, 100*time.Millisecond, 2*time.Second, func() bool {
		metadataNames, err := GetPodMetadataNames(node.Name, pod.Namespace, pod.Name)
		if err != nil {
			return false
		}
		if len(metadataNames) != 2 {
			return false
		}
		assert.Contains(t, metadataNames, "kube_service:nginx-1")
		assert.Contains(t, metadataNames, "kube_service:nginx-2")
		return true
	})

	cl := &APIClient{Cl: client, timeoutSeconds: 5}

	testutil.AssertTrueBeforeTimeout(t, 100*time.Millisecond, 2*time.Second, func() bool {
		fullmapper, errList := GetMetadataMapBundleOnAllNodes(cl)
		require.Nil(t, errList)
		list := fullmapper.Nodes
		assert.Contains(t, list, "ip-172-31-119-125")
		bundle := metadataMapperBundle{Services: list["ip-172-31-119-125"].Services}
		services, found := bundle.ServicesForPod(metav1.NamespaceDefault, "nginx")
		if !found {
			return false
		}
		assert.Contains(t, services, "nginx-1")
		return true
	})

}

func newFakeMetadataController(client kubernetes.Interface) (*MetadataController, informers.SharedInformerFactory) {
	informerFactory := informers.NewSharedInformerFactory(client, 1*time.Second)

	metaController := NewMetadataController(
		informerFactory.Core().V1().Nodes(),
		informerFactory.Core().V1().Namespaces(),
		informerFactory.Core().V1().Endpoints(),
	)

	return metaController, informerFactory
}

func newFakePod(namespace, name, uid, ip string) v1.Pod {
	return v1.Pod{
		TypeMeta: metav1.TypeMeta{Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
		Status: v1.PodStatus{PodIP: ip},
	}
}

func newFakeEndpointAddress(nodeName string, pod v1.Pod) v1.EndpointAddress {
	return v1.EndpointAddress{
		IP:       pod.Status.PodIP,
		NodeName: &nodeName,
		TargetRef: &v1.ObjectReference{
			Kind:      pod.Kind,
			Namespace: pod.Namespace,
			Name:      pod.Name,
			UID:       pod.UID,
		},
	}
}
