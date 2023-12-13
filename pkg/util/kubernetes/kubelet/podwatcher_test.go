// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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
	require.Len(suite.T(), sourcePods, 7)

	threePods := sourcePods[:3]
	remainingPods := sourcePods[3:]

	watcher := newWatcher()

	changes, err := watcher.computeChanges(threePods)
	require.Nil(suite.T(), err)
	// The first pod is a static pod but should be found
	require.Len(suite.T(), changes, 3)

	// Same list should detect no change
	changes, err = watcher.computeChanges(threePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 0)

	// A pod with new containers should be sent
	changes, err = watcher.computeChanges(remainingPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 4)
	require.Equal(suite.T(), changes[0].Metadata.UID, remainingPods[0].Metadata.UID)

	// A new container ID in an existing pod should trigger
	remainingPods[0].Status.Containers[0].ID = "testNewID"
	remainingPods[0].Status.AllContainers[0].ID = "testNewID"
	changes, err = watcher.computeChanges(remainingPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
	require.Equal(suite.T(), changes[0].Metadata.UID, remainingPods[0].Metadata.UID)

	// Sending the same pod again with no change
	changes, err = watcher.computeChanges(remainingPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 0)
}

func (suite *PodwatcherTestSuite) TestPodWatcherComputeChangesInConditions() {
	sourcePods, err := loadPodsFixture("./testdata/podlist_1.8-1.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 6)

	watcher := newWatcher()

	changes, err := watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 6, fmt.Sprintf("%d", len(changes)))
	for _, po := range changes {
		// nginx pod is not ready but still detected by the podwatcher
		if po.Metadata.Name == "nginx-99d8b564-4r4vq" {
			require.False(suite.T(), IsPodReady(po))
		} else {
			require.True(suite.T(), IsPodReady(po))
		}
	}

	// Same list should detect no change
	changes, err = watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 0)

	// The nginx become Ready
	sourcePods, err = loadPodsFixture("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 7)

	// Should detect 2 changes: nginx and the new kube-proxy static pod
	changes, err = watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 2)
	assert.Equal(suite.T(), "nginx", changes[0].Spec.Containers[0].Name)
	require.True(suite.T(), IsPodReady(changes[0]))
}

func (suite *PodwatcherTestSuite) TestPodWatcherWithInitContainers() {
	sourcePods, err := loadPodsFixture("./testdata/podlist_init_container_running.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 5)

	watcher := newWatcher()

	changes, err := watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 5, fmt.Sprintf("%d", len(changes)))

	// Init container finishes
	sourcePods, err = loadPodsFixture("./testdata/podlist_init_container_terminated.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 5)

	// Should detect the change with the main container being started
	changes, err = watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
	assert.Equal(suite.T(), "myapp-container", changes[0].Spec.Containers[0].Name)
	require.True(suite.T(), IsPodReady(changes[0]))
}

func (suite *PodwatcherTestSuite) TestPodWatcherWithShortLivedContainers() {
	sourcePods, err := loadPodsFixture("./testdata/podlist_short_lived_absent.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 4)

	watcher := newWatcher()

	changes, err := watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 4, fmt.Sprintf("%d", len(changes)))

	// Short lived pod started and is already terminated
	sourcePods, err = loadPodsFixture("./testdata/podlist_short_lived_terminated.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 5)

	// Should detect the change of the terminated short lived pod
	changes, err = watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
	assert.Equal(suite.T(), "short-lived-container", changes[0].Spec.Containers[0].Name)
	require.False(suite.T(), IsPodReady(changes[0]))
}

func (suite *PodwatcherTestSuite) TestPodWatcherReadinessChange() {
	sourcePods, err := loadPodsFixture("./testdata/podlist_container_not_ready.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 5)

	watcher := newWatcher()

	changes, err := watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 5, fmt.Sprintf("%d", len(changes)))
	expire, err := watcher.Expire()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 0)

	// The container goes into ready state
	sourcePods, err = loadPodsFixture("./testdata/podlist_container_ready.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 5)

	// Should detect the change of state of the redis container
	changes, err = watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
	assert.Equal(suite.T(), "redis-unready", changes[0].Spec.Containers[0].Name)
	require.True(suite.T(), IsPodReady(changes[0]))
	expire, err = watcher.Expire()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 0)

	// the pod goes unready again, detect change
	sourcePods, err = loadPodsFixture("./testdata/podlist_container_not_ready.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 5)
	changes, err = watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
	expire, err = watcher.Expire()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 0)

	testContainerID := "docker://84adac90973fa1263ccf1e296cec72acb4128b6e19fd25bffe4fafb059adafc0"

	// simulate unreadiness for 10 sec
	watcher.lastSeenReady[testContainerID] = watcher.lastSeenReady[testContainerID].Add(-10 * time.Second)
	sourcePods, err = loadPodsFixture("./testdata/podlist_container_not_ready.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 5)
	changes, err = watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 0)
	require.Len(suite.T(), watcher.lastSeenReady, 5)
	expire, err = watcher.Expire()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 0)

	// goes ready again, detect change
	sourcePods, err = loadPodsFixture("./testdata/podlist_container_ready.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 5)
	changes, err = watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
	expire, err = watcher.Expire()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 0)

	// simulate unreadiness for 45 sec
	// service should be removed
	watcher.lastSeenReady[testContainerID] = watcher.lastSeenReady[testContainerID].Add(-45 * time.Second)
	sourcePods, err = loadPodsFixture("./testdata/podlist_container_not_ready.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 5)
	changes, err = watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
	require.Len(suite.T(), watcher.lastSeenReady, 5)
	expire, err = watcher.Expire()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 1)
	require.Equal(suite.T(), testContainerID, expire[0])
	require.Len(suite.T(), watcher.lastSeenReady, 4)

	// The container goes into ready state again
	sourcePods, err = loadPodsFixture("./testdata/podlist_container_ready.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 5)
	// Should detect the change of state of the redis container
	changes, err = watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
	assert.Equal(suite.T(), "redis-unready", changes[0].Spec.Containers[0].Name)
	require.True(suite.T(), IsPodReady(changes[0]))
	expire, err = watcher.Expire()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 0)
}

func (suite *PodwatcherTestSuite) TestPodWatcherExpireUnready() {
	sourcePods, err := loadPodsFixture("./testdata/podlist_container_ready.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 5)

	watcher := newWatcher()

	changes, err := watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 5, fmt.Sprintf("%d", len(changes)))

	expire, err := watcher.Expire()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 0)

	// The container goes into unready state
	sourcePods, err = loadPodsFixture("./testdata/podlist_container_not_ready.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 5)

	// Try
	testContainerID := "docker://84adac90973fa1263ccf1e296cec72acb4128b6e19fd25bffe4fafb059adafc0"

	// 10 seconds should NOT be enough to expire
	watcher.lastSeenReady[testContainerID] = watcher.lastSeenReady[testContainerID].Add(-10 * time.Second)
	expire, err = watcher.Expire()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 0)

	// 45 secs should be enough to expire
	watcher.lastSeenReady[testContainerID] = watcher.lastSeenReady[testContainerID].Add(-45 * time.Second)
	require.Len(suite.T(), watcher.lastSeenReady, 5)
	expire, err = watcher.Expire()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 1)
	require.Equal(suite.T(), testContainerID, expire[0])
	require.Len(suite.T(), watcher.lastSeenReady, 4)
}

func (suite *PodwatcherTestSuite) TestPodWatcherExpireDelay() {
	sourcePods, err := loadPodsFixture("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 7)

	watcher := newWatcher()

	_, err = watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	// 7 pods (including 2 statics) + 5 container statuses (static pods don't report these)
	require.Len(suite.T(), watcher.lastSeen, 12)
	require.Len(suite.T(), watcher.tagsDigest, 7)

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
	require.Len(suite.T(), watcher.lastSeen, 11)
	// 0 pods expired, we'll have all the 7 pods entities in tagsDigest
	require.Len(suite.T(), watcher.tagsDigest, 7)
}

func (suite *PodwatcherTestSuite) TestPodWatcherExpireWholePod() {
	sourcePods, err := loadPodsFixture("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 7)

	watcher := newWatcher()

	_, err = watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), watcher.lastSeen, 12)
	require.Len(suite.T(), watcher.tagsDigest, 7)

	expire, err := watcher.Expire()
	require.Nil(suite.T(), err)
	require.Len(suite.T(), expire, 0)

	// Make everything old
	for k := range watcher.lastSeen {
		watcher.lastSeen[k] = watcher.lastSeen[k].Add(-10 * time.Minute)
	}

	// Remove last pods from the list, make sure we stop at the right one
	oldPod := sourcePods[5]
	require.Contains(suite.T(), oldPod.Metadata.UID, "d91aa43c-0769-11e8-afcc-000c29dea4f6")

	_, err = watcher.computeChanges(sourcePods[0:5])
	require.Nil(suite.T(), err)
	require.Len(suite.T(), watcher.lastSeen, 12)
	require.Len(suite.T(), watcher.tagsDigest, 7)

	// That one should expire, we'll have 9 entities left
	expire, err = watcher.Expire()
	require.Nil(suite.T(), err)
	expectedExpire := []string{
		"kubernetes_pod://d91aa43c-0769-11e8-afcc-000c29dea4f6",
		"docker://3e13513f94b41d23429804243820438fb9a214238bf2d4f384741a48b575670a",
		"kubernetes_pod://260c2b1d43b094af6d6b4ccba082c2db",
	}

	require.Equal(suite.T(), len(expectedExpire), len(expire))
	for _, expectedEntity := range expectedExpire {
		assert.Contains(suite.T(), expire, expectedEntity)
	}
	require.Len(suite.T(), watcher.lastSeen, 9)
	// Two pods expired, we'll have 7 - 2 pods entities in tagsDigest
	require.Len(suite.T(), watcher.tagsDigest, 5)
}

func (suite *PodwatcherTestSuite) TestPullChanges() {
	ctx := context.Background()
	mockConfig := config.Mock(nil)

	kubelet, err := newDummyKubelet("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	ts, kubeletPort, err := kubelet.StartTLS()
	defer os.Remove(kubelet.testingCertificate)
	defer os.Remove(kubelet.testingPrivateKey)
	require.Nil(suite.T(), err)
	defer ts.Close()

	mockConfig.SetWithoutSource("kubernetes_kubelet_host", "127.0.0.1")
	mockConfig.SetWithoutSource("kubernetes_https_kubelet_port", kubeletPort)
	mockConfig.SetWithoutSource("kubelet_tls_verify", false)

	watcher, err := NewPodWatcher(5 * time.Minute)
	require.Nil(suite.T(), err)
	require.NotNil(suite.T(), watcher)
	<-kubelet.Requests // Throwing away the first /spec GET

	ResetCache() // If we want to be sure to get a /pods request after
	pods, err := watcher.PullChanges(ctx)
	require.Nil(suite.T(), err)
	<-kubelet.Requests // Throwing away /pods GET
	require.Len(suite.T(), pods, 7)
}

func (suite *PodwatcherTestSuite) TestPodWatcherLabelsValueChange() {
	sourcePods, err := loadPodsFixture("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 7)

	watcher := newWatcher()

	twoPods := sourcePods[:2]
	changes, err := watcher.computeChanges(twoPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 2)

	twoPods[0].Metadata.Labels["label1"] = "value1"
	changes, err = watcher.computeChanges(twoPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)

	changes, err = watcher.computeChanges(twoPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 0)

	twoPods[0].Metadata.Labels["label1"] = "newvalue1"
	changes, err = watcher.computeChanges(twoPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)

	delete(twoPods[0].Metadata.Labels, "label1")
	changes, err = watcher.computeChanges(twoPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)

	twoPods[0].Metadata.Labels["newlabel1"] = "newvalue1"
	twoPods[1].Metadata.Labels["label1"] = "value1"
	changes, err = watcher.computeChanges(twoPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 2)
}

func (suite *PodwatcherTestSuite) TestPodWatcherPhaseChange() {
	sourcePods, err := loadPodsFixture("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 7)

	watcher := newWatcher()

	twoPods := sourcePods[:2]
	changes, err := watcher.computeChanges(twoPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 2)

	twoPods[0].Status.Phase = "Succeeded"
	changes, err = watcher.computeChanges(twoPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
}

func (suite *PodwatcherTestSuite) TestPodWatcherAnnotationsValueChange() {
	sourcePods, err := loadPodsFixture("./testdata/podlist_1.8-2.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 7)

	watcher := newWatcher()

	twoPods := sourcePods[:2]
	changes, err := watcher.computeChanges(twoPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 2)

	twoPods[0].Metadata.Annotations["annotation1"] = "value1"
	changes, err = watcher.computeChanges(twoPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)

	changes, err = watcher.computeChanges(twoPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 0)

	twoPods[0].Metadata.Annotations["annotation1"] = "newvalue1"
	changes, err = watcher.computeChanges(twoPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)

	delete(twoPods[0].Metadata.Annotations, "annotation1")
	changes, err = watcher.computeChanges(twoPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)

	twoPods[0].Metadata.Annotations["newannotation1"] = "newvalue1"
	twoPods[1].Metadata.Annotations["annotation1"] = "value1"
	changes, err = watcher.computeChanges(twoPods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 2)
}

func (suite *PodwatcherTestSuite) TestPodWatcherContainerCreatingTags() {
	sourcePods, err := loadPodsFixture("./testdata/podlist_container_creating.json")
	require.Nil(suite.T(), err)
	require.Len(suite.T(), sourcePods, 1)

	watcher := newWatcher()

	changes, err := watcher.computeChanges(sourcePods)
	require.Nil(suite.T(), err)
	require.Len(suite.T(), changes, 1)
}

func TestPodwatcherTestSuite(t *testing.T) {
	suite.Run(t, new(PodwatcherTestSuite))
}

func newWatcher() *PodWatcher {
	return &PodWatcher{
		lastSeen:       make(map[string]time.Time),
		lastSeenReady:  make(map[string]time.Time),
		tagsDigest:     make(map[string]string),
		oldPhase:       make(map[string]string),
		oldReadiness:   make(map[string]bool),
		expiryDuration: 5 * time.Minute,
	}
}
