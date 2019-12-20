// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build kubelet,linux

package kubelet

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

/*
The fixture podlist_1.8-1.json contains 6 pods, 4/6 are Ready:
nginx is Running but the readiness probe have an initialDelay
apiserver is from file, its status isn't updated yet:
see https://github.com/kubernetes/kubernetes/pull/57106
so it has no container to parse

-> 5 containers in output
*/

type ContainersTestSuite struct {
	suite.Suite
}

// Make sure globalKubeUtil is deleted before each test
func (suite *ContainersTestSuite) SetupTest() {
	ResetGlobalKubeUtil()
}

func (suite *ContainersTestSuite) TestParseContainerInPod() {
	sourcePods, err := loadPodsFixture("./testdata/podlist_1.8-1.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 6)

	expectedContainers := []*containers.Container{
		{
			Type:        "kubelet",
			ID:          "710695aa82cb808e979e39078f6dd18ece04d2bf444fdf78e9b37e360b6882d5",
			EntityID:    "container_id://710695aa82cb808e979e39078f6dd18ece04d2bf444fdf78e9b37e360b6882d5",
			Name:        "kube-scheduler-bpnn6-kube-scheduler",
			Image:       "gcr.io/google_containers/hyperkube:v1.8.3",
			Created:     1517487458,
			State:       "running",
			Health:      "healthy",
			AddressList: []containers.NetworkAddress{},
		},
		{
			Type:     "kubelet",
			ID:       "61e83ec5ce7af1c134c159bac1bf94d3413486ba655e5ebd6231e0a92a1c7b54",
			EntityID: "container_id://61e83ec5ce7af1c134c159bac1bf94d3413486ba655e5ebd6231e0a92a1c7b54",
			Name:     "nginx-99d8b564-4r4vq-nginx",
			Image:    "nginx:latest",
			Created:  1517490715,
			State:    "running",
			Health:   "unhealthy",
			AddressList: []containers.NetworkAddress{
				{IP: net.ParseIP("192.168.128.141"), Port: 80, Protocol: "TCP"},
				{IP: net.ParseIP("192.168.128.141"), Port: 443, Protocol: "TCP"},
			},
		},
		{
			Type:        "kubelet",
			ID:          "8a5d143fcca3f0b53dfe5f445905a2e82c02f0ff70fc0a98cc37eca389f9480c",
			EntityID:    "container_id://8a5d143fcca3f0b53dfe5f445905a2e82c02f0ff70fc0a98cc37eca389f9480c",
			Name:        "kube-controller-manager-kube-controller-manager",
			Image:       "gcr.io/google_containers/hyperkube:v1.8.3",
			Created:     1517487456,
			State:       "running",
			Health:      "healthy",
			AddressList: []containers.NetworkAddress{},
		},
		{
			Type:        "kubelet",
			ID:          "b3e4cd65204e04d1a2d4b7683cae2f59b2075700f033a6b09890bd0d3fecf6b6",
			EntityID:    "container_id://b3e4cd65204e04d1a2d4b7683cae2f59b2075700f033a6b09890bd0d3fecf6b6",
			Name:        "kube-proxy-rnd5q-kube-proxy",
			Image:       "gcr.io/google_containers/hyperkube:v1.8.3",
			Created:     1517487458,
			State:       "running",
			Health:      "healthy",
			AddressList: []containers.NetworkAddress{},
		},
		{
			Type:     "kubelet",
			ID:       "3e13513f94b41d23429804243820438fb9a214238bf2d4f384741a48b575670a",
			EntityID: "container_id://3e13513f94b41d23429804243820438fb9a214238bf2d4f384741a48b575670a",
			Name:     "redis-75586d7d7c-jrm7j-redis",
			Image:    "redis:latest",
			Created:  1517501194,
			State:    "running",
			Health:   "healthy",
			AddressList: []containers.NetworkAddress{
				{IP: net.ParseIP("172.17.0.3"), Port: 6379, Protocol: "TCP"},
				{IP: net.ParseIP("192.168.128.141"), Port: 1337, Protocol: "TCP"},
				{IP: net.ParseIP("172.17.0.3"), Port: 1337, Protocol: "TCP"},
			},
		},
	}

	expectedErrors := []error{nil, nil, nil, nil, nil}

	var results []*containers.Container
	var errors []error
	for _, pod := range sourcePods {
		for _, c := range pod.Status.Containers {
			c, err := parseContainerInPod(c, pod)
			results = append(results, c)
			errors = append(errors, err)
		}
	}

	assert.EqualValues(suite.T(), expectedContainers, results)
	assert.EqualValues(suite.T(), expectedErrors, errors)

}

func (suite *ContainersTestSuite) TestParseContainerReadiness() {
	sourcePods, err := loadPodsFixture("./testdata/podlist_1.8-1.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 6)

	// Container is past its initialDelaySeconds
	nginxPod := sourcePods[1]
	require.Equal(suite.T(), "nginx-99d8b564-4r4vq", nginxPod.Metadata.Name)

	nginxCState := nginxPod.Status.Containers[0]
	nginxCState.State.Running.StartedAt = time.Now().Add(-120 * time.Second)
	healthy := parseContainerReadiness(nginxCState, nginxPod)
	assert.Equal(suite.T(), healthy, containers.ContainerUnhealthy)

	// Container is within its initialDelaySeconds
	nginxCState.State.Running.StartedAt = time.Now().Add(-10 * time.Second)
	healthy = parseContainerReadiness(nginxCState, nginxPod)
	assert.Equal(suite.T(), healthy, containers.ContainerStartingHealth)

	// Container is ready
	nginxCState.Ready = true
	healthy = parseContainerReadiness(nginxCState, nginxPod)
	assert.Equal(suite.T(), healthy, containers.ContainerHealthy)

	// Nil pointer resilience
	nginxCState.Ready = false
	nginxCState.State.Running = nil
	healthy = parseContainerReadiness(nginxCState, nginxPod)
	assert.Equal(suite.T(), healthy, containers.ContainerUnknownHealth)
}

func TestContainersTestSuite(t *testing.T) {
	suite.Run(t, new(ContainersTestSuite))
}
