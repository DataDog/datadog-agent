// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build docker
// +build kubeapiserver

package kubernetes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	log "github.com/cihub/seelog"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/ericchiang/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const setupTimeout = time.Second * 10

type testSuite struct {
	suite.Suite
	apiClient      *apiserver.APIClient
	kubeConfigPath string
}

func TestSuiteKube(t *testing.T) {
	s := &testSuite{}

	// Start compose stack
	compose, err := initAPIServerCompose()
	require.Nil(t, err)
	output, err := compose.Start()
	defer compose.Stop()
	require.Nil(t, err, string(output))

	// Init apiclient
	pwd, err := os.Getwd()
	require.Nil(t, err)
	s.kubeConfigPath = filepath.Join(pwd, "testdata", "kubeconfig.json")
	config.Datadog.Set("kubernetes_kubeconfig_path", s.kubeConfigPath)
	_, err = os.Stat(s.kubeConfigPath)
	require.Nil(t, err, fmt.Sprintf("%v", err))

	suite.Run(t, s)
}

func (suite *testSuite) SetupTest() {
	var err error

	tick := time.NewTicker(time.Millisecond * 500)
	timeout := time.NewTicker(setupTimeout)
	for {
		select {
		case <-timeout.C:
			require.FailNow(suite.T(), "timeout after %s", setupTimeout.String())

		case <-tick.C:
			suite.apiClient, err = apiserver.GetAPIClient()
			if err != nil {
				log.Debugf("cannot init: %s", err)
				continue
			}
			// Confirm that we can query the kube-apiserver's resources
			log.Debugf("trying to get LatestEvents")
			_, _, resV, err := suite.apiClient.LatestEvents("0")
			if err == nil {
				log.Debugf("successfully get LatestEvents: %s", resV)
				return
			}
			log.Debugf("cannot get LatestEvents: %s", err)
		}
	}
}

func (suite *testSuite) TestKubeEvents() {
	// Init own client to write the events
	var k8sConf *k8s.Config
	k8sConf, err := apiserver.ParseKubeConfig(suite.kubeConfigPath)
	require.Nil(suite.T(), err)
	rawClient, err := k8s.NewClient(k8sConf)
	require.Nil(suite.T(), err)
	core := rawClient.CoreV1()
	require.NotNil(suite.T(), core)

	// Ignore potential startup events
	_, _, initresversion, err := suite.apiClient.LatestEvents("0")
	require.Nil(suite.T(), err)

	// Create started event
	testReference := createObjectReference("default", "integration_test", "event_test")
	startedEvent := createEvent("default", "test_started", "started", testReference)
	_, err = core.CreateEvent(context.Background(), startedEvent)
	require.Nil(suite.T(), err)

	// Test we get the new started event
	added, modified, resversion, err := suite.apiClient.LatestEvents(initresversion)
	require.Nil(suite.T(), err)
	assert.Len(suite.T(), added, 1)
	assert.Len(suite.T(), modified, 0)
	assert.Equal(suite.T(), "started", *added[0].Reason)

	// Create tick event
	tickEvent := createEvent("default", "test_tick", "tick", testReference)
	_, err = core.CreateEvent(context.Background(), tickEvent)
	require.Nil(suite.T(), err)

	// Test we get the new tick event
	added, modified, resversion, err = suite.apiClient.LatestEvents(resversion)
	require.Nil(suite.T(), err)
	assert.Len(suite.T(), added, 1)
	assert.Len(suite.T(), modified, 0)
	assert.Equal(suite.T(), "tick", *added[0].Reason)

	// Update tick event
	pointer2 := int32(2)
	tickEvent2 := added[0]
	tickEvent2.Count = &pointer2
	tickEvent3, err := core.UpdateEvent(context.Background(), tickEvent2)
	require.Nil(suite.T(), err)

	// Update tick event a second time
	pointer3 := int32(3)
	tickEvent3.Count = &pointer3
	_, err = core.UpdateEvent(context.Background(), tickEvent3)
	require.Nil(suite.T(), err)

	// Test we get the two modified test events
	added, modified, resversion, err = suite.apiClient.LatestEvents(resversion)
	require.Nil(suite.T(), err)
	assert.Len(suite.T(), added, 0)
	assert.Len(suite.T(), modified, 2)
	assert.Equal(suite.T(), "tick", *modified[0].Reason)
	assert.EqualValues(suite.T(), 2, *modified[0].Count)
	assert.Equal(suite.T(), "tick", *modified[1].Reason)
	assert.EqualValues(suite.T(), 3, *modified[1].Count)
	assert.EqualValues(suite.T(), *modified[0].Metadata.Uid, *modified[1].Metadata.Uid)

	// We should get nothing new now
	added, modified, resversion, err = suite.apiClient.LatestEvents(resversion)
	require.Nil(suite.T(), err)
	assert.Len(suite.T(), added, 0)
	assert.Len(suite.T(), modified, 0)

	// We should get 2+0 events from initresversion
	// apiserver does not send updates to objects if the add is in the same bucket
	added, modified, _, err = suite.apiClient.LatestEvents(initresversion)
	require.Nil(suite.T(), err)
	assert.Len(suite.T(), added, 2)
	assert.Len(suite.T(), modified, 0)
}

func (suite *testSuite) TestServiceMapper() {
	c, err := apiserver.GetCoreV1Client()
	require.Nil(suite.T(), err)

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
	_, err = c.Nodes().Create(node)
	require.Nil(suite.T(), err)

	pod := &v1.Pod{
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
	require.Nil(suite.T(), err)

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
	require.Nil(suite.T(), err)

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
	require.Nil(suite.T(), err)

	ep := &v1.Endpoints{
		Subsets: []v1.EndpointSubset{
			{
				Addresses: []v1.EndpointAddress{
					{
						IP:       pendingPod.Status.PodIP,
						NodeName: &node.Name,
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
	require.Nil(suite.T(), err)

	apiClient, err := apiserver.GetAPIClient()
	require.Nil(suite.T(), err)
	err = apiClient.ClusterServiceMapping()
	require.Nil(suite.T(), err)

	serviceNames, err := apiserver.GetPodServiceNames(node.Name, pod.Name)
	require.Nil(suite.T(), err)
	assert.Len(suite.T(), serviceNames, 1)
	assert.Contains(suite.T(), serviceNames, "nginx-1")

	// Add a new service/endpoint on the nginx Pod
	svc.Name = "nginx-2"
	_, err = c.Services("default").Create(svc)
	require.Nil(suite.T(), err)

	ep.Name = "nginx-2"
	_, err = c.Endpoints("default").Create(ep)
	require.Nil(suite.T(), err)

	err = apiClient.ClusterServiceMapping()
	require.Nil(suite.T(), err)

	serviceNames, err = apiserver.GetPodServiceNames(node.Name, pod.Name)
	require.Nil(suite.T(), err)
	assert.Len(suite.T(), serviceNames, 2)
	assert.Contains(suite.T(), serviceNames, "nginx-1")
	assert.Contains(suite.T(), serviceNames, "nginx-2")
}
