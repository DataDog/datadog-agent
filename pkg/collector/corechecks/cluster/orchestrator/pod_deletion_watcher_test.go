// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package orchestrator

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

type PodDeletionWatcherTestSuite struct {
	suite.Suite
	client      *fake.Clientset
	fakeWatcher *watch.RaceFreeFakeWatcher
	mu          sync.Mutex
}

func (s *PodDeletionWatcherTestSuite) setFakeWatcher(fakeWatcher *watch.RaceFreeFakeWatcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fakeWatcher = fakeWatcher
}

func (s *PodDeletionWatcherTestSuite) getFakeWatcher() *watch.RaceFreeFakeWatcher {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fakeWatcher
}

func (s *PodDeletionWatcherTestSuite) SetupTest() {
	s.client = fake.NewClientset()
	s.setFakeWatcher(watch.NewRaceFreeFake())

	// List operation used to retrieve an initial resourceVersion for the watch operation.
	s.client.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, &corev1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: "1000"}}, nil
	})
	// Watch operation used to retrieve pod events from a resourceVersion onwards.
	s.client.PrependWatchReactor("pods", func(action k8stesting.Action) (bool, watch.Interface, error) {
		return true, s.getFakeWatcher(), nil
	})
}

func createTestPod(name, namespace, uid, resourceVersion string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			UID:             types.UID(uid),
			ResourceVersion: resourceVersion,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "container", Image: "image:tag"},
			},
		},
	}
}

func (s *PodDeletionWatcherTestSuite) TestDeleteEvent() {
	deletedPodChan := make(chan *corev1.Pod, 1)
	testPod := createTestPod("pod-name", "pod-namespace", "pod-uid", "100")

	watcher := NewPodDeletionWatcher(s.client, func(pod *corev1.Pod) { deletedPodChan <- pod })
	watcher.Start()

	s.getFakeWatcher().Delete(testPod)

	deletedPod := <-deletedPodChan
	watcher.Stop()

	s.Equal(int64(1), watcher.Processed())
	s.Equal("pod-name", deletedPod.Name)
}

func (s *PodDeletionWatcherTestSuite) TestIgnoresNonDeleteEvents() {
	testPod := createTestPod("pod-name", "pod-namespace", "pod-uid", "100")
	watcher := NewPodDeletionWatcher(s.client, func(pod *corev1.Pod) {})
	watcher.Start()

	s.getFakeWatcher().Add(testPod)
	s.getFakeWatcher().Modify(testPod)

	watcher.Stop()

	s.Equal(int64(0), watcher.Processed())
}

func (s *PodDeletionWatcherTestSuite) TestMultipleDeletes() {
	deletedPodChan := make(chan *corev1.Pod, 3)
	deletedPods := []*corev1.Pod{}

	watcher := NewPodDeletionWatcher(s.client, func(pod *corev1.Pod) { deletedPodChan <- pod })
	watcher.Start()

	pods := []*corev1.Pod{
		createTestPod("pod-1", "namespace-1", "uid-1", "101"),
		createTestPod("pod-2", "namespace-2", "uid-2", "102"),
		createTestPod("pod-3", "namespace-3", "uid-3", "103"),
	}
	for _, pod := range pods {
		s.getFakeWatcher().Delete(pod)
		deletedPods = append(deletedPods, <-deletedPodChan)
	}

	watcher.Stop()

	s.Equal(int64(3), watcher.Processed())
	for i, pod := range pods {
		s.Equal(pod.Name, deletedPods[i].Name)
	}
}

func (s *PodDeletionWatcherTestSuite) TestRecoversFrom410GoneError() {
	deletedPodChan := make(chan *corev1.Pod, 1)
	watcher := NewPodDeletionWatcher(s.client, func(pod *corev1.Pod) { deletedPodChan <- pod })
	watcher.Start()

	pod1 := createTestPod("pod-1", "namespace-1", "uid-1", "101")
	s.getFakeWatcher().Delete(pod1)
	<-deletedPodChan

	// Send 410 Gone error
	s.getFakeWatcher().Error(&metav1.Status{
		Code:   http.StatusGone,
		Reason: metav1.StatusReasonGone,
		Status: metav1.StatusFailure,
	})

	// Terminate previous watcher and create a new one.
	s.getFakeWatcher().Stop()
	s.setFakeWatcher(watch.NewRaceFreeFake())

	// Make sure new watcher events are consumed.
	pod2 := createTestPod("pod-2", "namespace-2", "uid-2", "102")
	s.getFakeWatcher().Delete(pod2)
	<-deletedPodChan

	watcher.Stop()
}

func (s *PodDeletionWatcherTestSuite) TestStartIdempotent() {
	watcher := NewPodDeletionWatcher(s.client, func(pod *corev1.Pod) {})
	watcher.Start()
	watcher.Start()
	s.Eventually(func() bool { return watcher.running }, 1*time.Second, 10*time.Millisecond)

	watcher.Stop()
	s.False(watcher.running)
}

func (s *PodDeletionWatcherTestSuite) TestStartStop() {
	watcher := NewPodDeletionWatcher(s.client, func(pod *corev1.Pod) {})
	s.False(watcher.running)

	watcher.Start()
	s.Eventually(func() bool { return watcher.running }, 1*time.Second, 10*time.Millisecond)

	watcher.Stop()
	s.False(watcher.running)
}

func (s *PodDeletionWatcherTestSuite) TestStopBeforeStart() {
	watcher := NewPodDeletionWatcher(s.client, func(pod *corev1.Pod) {})
	watcher.Stop()
	s.False(watcher.running)
}

func (s *PodDeletionWatcherTestSuite) TestStopIdempotent() {
	watcher := NewPodDeletionWatcher(s.client, func(pod *corev1.Pod) {})
	watcher.Start()
	watcher.Stop()
	watcher.Stop()
	s.False(watcher.running)
}

func TestPodDeletionWatcherTestSuite(t *testing.T) {
	suite.Run(t, new(PodDeletionWatcherTestSuite))
}
