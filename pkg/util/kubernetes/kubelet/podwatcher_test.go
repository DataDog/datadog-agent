// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package kubelet

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestPodWatcherComputeChanges(t *testing.T) {
	raw, err := ioutil.ReadFile("./test/podlist_1.6.json")
	require.Nil(t, err)
	var podlist PodList
	json.Unmarshal(raw, &podlist)
	sourcePods := podlist.Items
	require.Len(t, sourcePods, 4)

	threePods := sourcePods[0:3]
	fourthPod := sourcePods[3:4]

	watcher := &PodWatcher{
		latestResVersion: -1,
		lastSeen:         make(map[string]time.Time),
		expiryDuration:   5 * time.Minute,
	}

	changes, err := watcher.computechanges(threePods)
	require.Nil(t, err)
	require.Len(t, changes, 3)

	// Same list should detect no change
	changes, err = watcher.computechanges(threePods)
	require.Nil(t, err)
	require.Len(t, changes, 0)

	// A pod with new containers should be sent
	fourthPod[0].Metadata.ResVersion = fmt.Sprintf("%d", watcher.latestResVersion-1)
	changes, err = watcher.computechanges(fourthPod)
	require.Nil(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, changes[0].Metadata.UID, fourthPod[0].Metadata.UID)

	// A newer resversion should be computed
	fourthPod[0].Metadata.ResVersion = fmt.Sprintf("%d", watcher.latestResVersion+1)
	changes, err = watcher.computechanges(fourthPod)
	require.Nil(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, changes[0].Metadata.UID, fourthPod[0].Metadata.UID)

	// A new container ID in an existing pod should trigger
	fourthPod[0].Status.Containers[0].ID = "testNewID"
	changes, err = watcher.computechanges(fourthPod)
	require.Nil(t, err)
	require.Len(t, changes, 1)
	require.Equal(t, changes[0].Metadata.UID, fourthPod[0].Metadata.UID)

	// Sending the same pod again with no change
	changes, err = watcher.computechanges(fourthPod)
	require.Nil(t, err)
	require.Len(t, changes, 0)
}

func TestPodWatcherExpireContainers(t *testing.T) {
	raw, err := ioutil.ReadFile("./test/podlist_1.6.json")
	require.Nil(t, err)
	var podlist PodList
	json.Unmarshal(raw, &podlist)
	sourcePods := podlist.Items
	require.Len(t, sourcePods, 4)

	watcher := &PodWatcher{
		latestResVersion: -1,
		lastSeen:         make(map[string]time.Time),
		expiryDuration:   5 * time.Minute,
	}

	_, err = watcher.computechanges(sourcePods)
	require.Nil(t, err)
	require.Len(t, watcher.lastSeen, 5)

	expire, err := watcher.ExpireContainers()
	require.Nil(t, err)
	require.Len(t, expire, 0)

	testContainerID := "docker://b2beae57bb2ada35e083c78029fe6a742848ff021d839107e2ede87a9ce7bf50"

	// 4 minutes should NOT be enough to expire
	watcher.lastSeen[testContainerID] = watcher.lastSeen[testContainerID].Add(-4 * time.Minute)
	expire, err = watcher.ExpireContainers()
	require.Nil(t, err)
	require.Len(t, expire, 0)

	// 6 minutes should be enough to expire
	watcher.lastSeen[testContainerID] = watcher.lastSeen[testContainerID].Add(-6 * time.Minute)
	expire, err = watcher.ExpireContainers()
	require.Nil(t, err)
	require.Len(t, expire, 1)
	require.Equal(t, testContainerID, expire[0])
	require.Len(t, watcher.lastSeen, 4)
}

func TestPullChanges(t *testing.T) {
	kubelet, err := newDummyKubelet("./test/podlist_1.6.json")
	require.Nil(t, err)
	ts, kubeletPort, err := kubelet.Start()
	defer ts.Close()
	require.Nil(t, err)

	config.Datadog.SetDefault("kubernetes_kubelet_host", "localhost")
	config.Datadog.SetDefault("kubernetes_http_kubelet_port", kubeletPort)

	watcher, err := NewPodWatcher()
	require.Nil(t, err)
	require.NotNil(t, watcher)
	<-kubelet.Requests // Throwing away /healthz GET

	pods, err := watcher.PullChanges()
	<-kubelet.Requests // Throwing away /pods GET
	require.Nil(t, err)
	require.Len(t, pods, 4)
}
