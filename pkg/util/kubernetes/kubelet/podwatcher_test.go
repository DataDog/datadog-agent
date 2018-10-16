// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build kubelet

package kubelet

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/DataDog/datadog-agent/pkg/config"
)

/*
The fixture podlist_1.8-1.json contains 6 pods, 4/6 are Ready:
nginx is Running but the readiness probe have an initialDelay
apiserver is from file, its status isn't updated yet:
see https://github.com/kubernetes/kubernetes/pull/57106

The fixture podlist_1.8-2.json have the nginx running
*/

type PodwatcherTestSuite struct {
	suite.Suite
}

// Make sure globalKubeUtil is deleted before each test
func (suite *PodwatcherTestSuite) SetupTest() {
	ResetGlobalKubeUtil()
}

func (suite *PodwatcherTestSuite) TestPodWatcherComputeChanges() {
	sourcePods, err := loadPodsFixture("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 6)

	threePods := sourcePods[:3]
	sixthPods := sourcePods[3:]

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
	changes, err = watcher.computeChanges(sixthPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 3)
	require.Equal(suite.T(), changes[0].Metadata.UID, sixthPods[0].Metadata.UID)

	// A new container ID in an existing pod should trigger
	sixthPods[0].Status.Containers[0].ID = "testNewID"
	changes, err = watcher.computeChanges(sixthPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
	require.Equal(suite.T(), changes[0].Metadata.UID, sixthPods[0].Metadata.UID)

	// Sending the same pod again with no change
	changes, err = watcher.computeChanges(sixthPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 0)
}

func (suite *PodwatcherTestSuite) TestPodWatcherComputeChangesInConditions() {
	sourcePods, err := loadPodsFixture("./testdata/podlist_1.8-1.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 6)

	watcher := &PodWatcher{
		lastSeen:       make(map[string]time.Time),
		expiryDuration: 5 * time.Minute,
	}

	changes, err := watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 4, fmt.Sprintf("%d", len(changes)))
	for _, po := range changes {
		require.True(suite.T(), IsPodReady(po))
	}

	// Same list should detect no change
	changes, err = watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 0)

	// The nginx become Ready
	sourcePods, err = loadPodsFixture("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 6)

	// Should detect 1 change: nginx
	changes, err = watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
	assert.Equal(suite.T(), "nginx", changes[0].Spec.Containers[0].Name)
	require.True(suite.T(), IsPodReady(changes[0]))
}

func (suite *PodwatcherTestSuite) TestPodWatcherExpireDelay() {
	sourcePods, err := loadPodsFixture("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 6)

	watcher := &PodWatcher{
		lastSeen:       make(map[string]time.Time),
		expiryDuration: 5 * time.Minute,
	}

	_, err = watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), watcher.lastSeen, 10)

	expire, err := watcher.Expire()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 0)

	// Try
	testContainerID := "docker://b3e4cd65204e04d1a2d4b7683cae2f59b2075700f033a6b09890bd0d3fecf6b6"

	// 4 minutes should NOT be enough to expire
	watcher.lastSeen[testContainerID] = watcher.lastSeen[testContainerID].Add(-4 * time.Minute)
	expire, err = watcher.Expire()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 0)

	// 6 minutes should be enough to expire
	watcher.lastSeen[testContainerID] = watcher.lastSeen[testContainerID].Add(-6 * time.Minute)
	expire, err = watcher.Expire()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 1)
	require.Equal(suite.T(), testContainerID, expire[0])
	require.Len(suite.T(), watcher.lastSeen, 9)
}

func (suite *PodwatcherTestSuite) TestPodWatcherExpireWholePod() {
	sourcePods, err := loadPodsFixture("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 6)

	watcher := &PodWatcher{
		lastSeen:       make(map[string]time.Time),
		expiryDuration: 5 * time.Minute,
	}

	_, err = watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), watcher.lastSeen, 10)

	expire, err := watcher.Expire()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 0)

	// Make everything old
	for k := range watcher.lastSeen {
		watcher.lastSeen[k] = watcher.lastSeen[k].Add(-10 * time.Minute)
	}

	// Remove one pod from the list, make sure we take the good one
	oldPod := sourcePods[5]
	require.Contains(suite.T(), oldPod.Metadata.UID, "d91aa43c-0769-11e8-afcc-000c29dea4f6")

	_, err = watcher.computeChanges(sourcePods[0:5])
	require.Nil(suite.T(), err)
	require.Len(suite.T(), watcher.lastSeen, 10)

	// That one should expire, we'll have 8 entities left
	expire, err = watcher.Expire()
	require.Nil(suite.T(), err)
	expectedExpire := []string{
		"kubernetes_pod://d91aa43c-0769-11e8-afcc-000c29dea4f6",
		"docker://3e13513f94b41d23429804243820438fb9a214238bf2d4f384741a48b575670a",
	}
	require.Equal(suite.T(), len(expectedExpire), len(expire))
	for _, expectedEntity := range expectedExpire {
		assert.Contains(suite.T(), expire, expectedEntity)
	}
	require.Len(suite.T(), watcher.lastSeen, 8)
}

func (suite *PodwatcherTestSuite) TestPullChanges() {
	mockConfig := config.NewMock()

	kubelet, err := newDummyKubelet("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.StartTLS()
	defer ts.Close()
	require.Nil(suite.T(), err)

	mockConfig.Set("kubernetes_kubelet_host", "127.0.0.1")
	mockConfig.Set("kubernetes_https_kubelet_port", kubeletPort)
	mockConfig.Set("kubelet_tls_verify", false)

	watcher, err := NewPodWatcher(5 * time.Minute)
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), watcher)
	<-kubelet.Requests // Throwing away the first /pods GET

	pods, err := watcher.PullChanges()
	require.Nil(suite.T(), err)
	<-kubelet.Requests // Throwing away /pods GET
	// The second pod is pending with no container
	require.Len(suite.T(), pods, 5)
}

func TestPodwatcherTestSuite(t *testing.T) {
	suite.Run(t, new(PodwatcherTestSuite))
}
