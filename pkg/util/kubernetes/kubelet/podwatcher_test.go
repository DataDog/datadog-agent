// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type PodwatcherTestSuite struct {
	suite.Suite
}

// Make sure globalKubeUtil is deleted before each test
func (suite *PodwatcherTestSuite) SetupTest() {
	globalKubeUtil = nil
}

func (suite *PodwatcherTestSuite) TestPodWatcherComputeChanges() {
	raw, err := ioutil.ReadFile("./test/podlist_1.6.json")
	require.Nil(suite.T(), err)
	var podlist PodList
	json.Unmarshal(raw, &podlist)
	sourcePods := podlist.Items
	require.Len(suite.T(), sourcePods, 4)

	threePods := sourcePods[0:3]
	fourthPod := sourcePods[3:4]

	watcher := &PodWatcher{
		latestResVersion: -1,
		lastSeen:         make(map[string]time.Time),
		expiryDuration:   5 * time.Minute,
	}

	changes, err := watcher.computechanges(threePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 3)

	// Same list should detect no change
	changes, err = watcher.computechanges(threePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 0)

	// A pod with new containers should be sent
	fourthPod[0].Metadata.ResVersion = fmt.Sprintf("%d", watcher.latestResVersion-1)
	changes, err = watcher.computechanges(fourthPod)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
	require.Equal(suite.T(), changes[0].Metadata.UID, fourthPod[0].Metadata.UID)

	// A newer resversion should be computed
	fourthPod[0].Metadata.ResVersion = fmt.Sprintf("%d", watcher.latestResVersion+1)
	changes, err = watcher.computechanges(fourthPod)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
	require.Equal(suite.T(), changes[0].Metadata.UID, fourthPod[0].Metadata.UID)

	// A new container ID in an existing pod should trigger
	fourthPod[0].Status.Containers[0].ID = "testNewID"
	changes, err = watcher.computechanges(fourthPod)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
	require.Equal(suite.T(), changes[0].Metadata.UID, fourthPod[0].Metadata.UID)

	// Sending the same pod again with no change
	changes, err = watcher.computechanges(fourthPod)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 0)
}

func (suite *PodwatcherTestSuite) TestPodWatcherExpireContainers() {
	raw, err := ioutil.ReadFile("./test/podlist_1.6.json")
	require.Nil(suite.T(), err)
	var podlist PodList
	json.Unmarshal(raw, &podlist)
	sourcePods := podlist.Items
	require.Len(suite.T(), sourcePods, 4)

	watcher := &PodWatcher{
		latestResVersion: -1,
		lastSeen:         make(map[string]time.Time),
		expiryDuration:   5 * time.Minute,
	}

	_, err = watcher.computechanges(sourcePods)
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
	kubelet, err := newDummyKubelet("./test/podlist_1.6.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(suite.T(), err)

	config.Datadog.SetDefault("kubernetes_kubelet_host", "localhost")
	config.Datadog.SetDefault("kubernetes_http_kubelet_port", kubeletPort)

	watcher, err := NewPodWatcher(5 * time.Minute)
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), watcher)
	<-kubelet.Requests // Throwing away /healthz GET

	pods, err := watcher.PullChanges()
	<-kubelet.Requests // Throwing away /pods GET
	require.Nil(suite.T(), err)
	require.Len(suite.T(), pods, 4)
}

func TestPodwatcherTestSuite(t *testing.T) {
	suite.Run(t, new(PodwatcherTestSuite))
}
