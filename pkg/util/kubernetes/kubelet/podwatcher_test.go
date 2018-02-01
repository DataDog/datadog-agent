// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type PodwatcherTestSuite struct {
	suite.Suite
}

// Make sure globalKubeUtil is deleted before each test
func (suite *PodwatcherTestSuite) SetupTest() {
	ResetGlobalKubeUtil()
}

func (suite *PodwatcherTestSuite) TestPodWatcherComputeChanges() {
	raw, err := ioutil.ReadFile("./testdata/podlist_1.6.json")
	require.Nil(suite.T(), err)
	var podList PodList
	json.Unmarshal(raw, &podList)
	sourcePods := podList.Items
	require.Len(suite.T(), sourcePods, 4)

	threePods := sourcePods[0:3]
	fourthPod := sourcePods[3:4]

	watcher := &PodWatcher{
		lastSeen:       make(map[string]time.Time),
		expiryDuration: 5 * time.Minute,
	}

	changes, err := watcher.computeChanges(threePods)
	require.Nil(suite.T(), err)
	// The second pod is pending with no container
	require.Len(suite.T(), changes, 2)

	// Same list should detect no change
	changes, err = watcher.computeChanges(threePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 0)

	// A pod with new containers should be sent
	changes, err = watcher.computeChanges(fourthPod)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
	require.Equal(suite.T(), changes[0].Metadata.UID, fourthPod[0].Metadata.UID)

	// A new container ID in an existing pod should trigger
	fourthPod[0].Status.Containers[0].ID = "testNewID"
	changes, err = watcher.computeChanges(fourthPod)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
	require.Equal(suite.T(), changes[0].Metadata.UID, fourthPod[0].Metadata.UID)

	// Sending the same pod again with no change
	changes, err = watcher.computeChanges(fourthPod)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 0)
}

func (suite *PodwatcherTestSuite) TestPodWatcherComputeChangesInConditions() {
	// this fixture contains 5 pods, 3/5 are Ready:
	// nginx is Running but the readiness probe have an initialDelay
	// apiserver is from file, its status isn't updated yet:
	// see https://github.com/kubernetes/kubernetes/pull/57106
	raw, err := ioutil.ReadFile("./testdata/podlist_1.8-1.json")
	require.Nil(suite.T(), err)
	var podList PodList
	json.Unmarshal(raw, &podList)
	require.Len(suite.T(), podList.Items, 5)

	watcher := &PodWatcher{
		lastSeen:       make(map[string]time.Time),
		expiryDuration: 5 * time.Minute,
	}

	changes, err := watcher.computeChanges(podList.Items)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 3, fmt.Sprintf("%d", len(changes)))

	// Same list should detect no change
	changes, err = watcher.computeChanges(podList.Items)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 0)

	// The nginx become Ready
	raw, err = ioutil.ReadFile("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	json.Unmarshal(raw, &podList)
	require.Len(suite.T(), podList.Items, 5)

	// Should detect 1 change: nginx
	changes, err = watcher.computeChanges(podList.Items)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
	assert.Equal(suite.T(), "nginx", changes[0].Spec.Containers[0].Name)
}

func (suite *PodwatcherTestSuite) TestPodWatcherExpireContainers() {
	raw, err := ioutil.ReadFile("./testdata/podlist_1.6.json")
	require.Nil(suite.T(), err)
	var podList PodList
	json.Unmarshal(raw, &podList)
	sourcePods := podList.Items
	require.Len(suite.T(), sourcePods, 4)

	watcher := &PodWatcher{
		lastSeen:       make(map[string]time.Time),
		expiryDuration: 5 * time.Minute,
	}

	_, err = watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), watcher.lastSeen, 5)

	expire, err := watcher.ExpireContainers()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 0)

	testContainerID := "docker://b2beae57bb2ada35e083c78029fe6a742848ff021d839107e2ede87a9ce7bf50"

	// 4 minutes should NOT be enough to expire
	watcher.lastSeen[testContainerID] = watcher.lastSeen[testContainerID].Add(-4 * time.Minute)
	expire, err = watcher.ExpireContainers()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 0)

	// 6 minutes should be enough to expire
	watcher.lastSeen[testContainerID] = watcher.lastSeen[testContainerID].Add(-6 * time.Minute)
	expire, err = watcher.ExpireContainers()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 1)
	require.Equal(suite.T(), testContainerID, expire[0])
	require.Len(suite.T(), watcher.lastSeen, 4)
}

func (suite *PodwatcherTestSuite) TestPullChanges() {
	kubelet, err := newDummyKubelet("./testdata/podlist_1.6.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.StartTLS()
	defer ts.Close()
	require.Nil(suite.T(), err)

	config.Datadog.Set("kubernetes_kubelet_host", "127.0.0.1")
	config.Datadog.Set("kubernetes_https_kubelet_port", kubeletPort)
	config.Datadog.Set("kubelet_tls_verify", false)

	watcher, err := NewPodWatcher(5 * time.Minute)
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), watcher)
	<-kubelet.Requests // Throwing away the first /pods GET

	pods, err := watcher.PullChanges()
	require.Nil(suite.T(), err)
	<-kubelet.Requests // Throwing away /pods GET
	// The second pod is pending with no container
	require.Len(suite.T(), pods, 3)
}

func TestPodwatcherTestSuite(t *testing.T) {
	suite.Run(t, new(PodwatcherTestSuite))
}
