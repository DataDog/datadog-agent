// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package k8s

import (
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// PodDeletionWatcherTestSuite provides a test suite for PodDeletionWatcher with common setup.
type PodDeletionWatcherTestSuite struct {
	suite.Suite
	fakeClient *fake.Clientset
	fakeWatch  *watch.FakeWatcher
	mu         sync.Mutex
	podChan    chan *corev1.Pod
	stopCh     chan struct{}
	watcher    *PodDeletionWatcher
}

// SetupTest runs before each test to create a fresh fake client and watcher.
func (s *PodDeletionWatcherTestSuite) SetupTest() {
	s.fakeClient = fake.NewSimpleClientset()
	s.setFakeWatcher(watch.NewFake())

	// Intercept list calls to return a resource version
	// nolint:revive
	s.fakeClient.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		podList := &corev1.PodList{
			ListMeta: metav1.ListMeta{
				ResourceVersion: "1000",
			},
		}
		return true, podList, nil
	})

	// Intercept watch calls and return our fake watcher
	// nolint:revive
	s.fakeClient.PrependWatchReactor("pods", func(action k8stesting.Action) (bool, watch.Interface, error) {
		return true, s.getFakeWatcher(), nil
	})

	// Create pod channel for receiving deletion events
	s.podChan = make(chan *corev1.Pod, 10)

	// Create stop channel for watcher lifecycle
	s.stopCh = make(chan struct{})

	eventHandler := func(pod *corev1.Pod) {
		s.podChan <- pod
	}

	// Create watcher with event handler (starts automatically)
	s.watcher = NewPodDeletionWatcher(s.fakeClient, eventHandler, s.stopCh)
}

// TearDownTest runs after each test to clean up resources.
func (s *PodDeletionWatcherTestSuite) TearDownTest() {
	close(s.stopCh)
	s.watcher.AwaitStop()
}

// TestFiltersNonDeleteEvents verifies that the watcher only processes
// delete events and ignores add, modify, and bookmark events.
func (s *PodDeletionWatcherTestSuite) TestFiltersNonDeleteEvents() {
	pod := s.createPod("pod-name-1", "pod-namespace-1", "1")

	// Send non-delete events which should be filtered
	s.getFakeWatcher().Add(pod)
	s.getFakeWatcher().Modify(pod)

	s.assertNoPod()
}

// TestHandles401Unauthorized verifies that the watcher handles 401 Unauthorized errors
// by reconnecting with backoff.
func (s *PodDeletionWatcherTestSuite) TestHandles401Unauthorized() {
	pod1 := s.createPod("pod-name-1", "pod-namespace-1", "1")
	s.getFakeWatcher().Delete(pod1)
	s.Equal(pod1, s.receivePod())

	// Send 401 Unauthorized error
	s.getFakeWatcher().Error(&metav1.Status{
		Code:   http.StatusUnauthorized,
		Reason: metav1.StatusReasonUnauthorized,
	})

	// Stop old fake watcher and create a new one for reconnection
	s.getFakeWatcher().Stop()
	s.setFakeWatcher(watch.NewFake())

	// Verify watcher reconnected by sending event on new fake watcher
	pod2 := s.createPod("pod-name-2", "pod-namespace-2", "2")
	s.getFakeWatcher().Delete(pod2)
	s.Equal(pod2, s.receivePod())
}

// TestHandles403Forbidden verifies that the watcher handles 403 Forbidden errors
// by reconnecting with backoff.
func (s *PodDeletionWatcherTestSuite) TestHandles403Forbidden() {
	pod1 := s.createPod("pod-name-1", "pod-namespace-1", "1")
	s.getFakeWatcher().Delete(pod1)
	s.Equal(pod1, s.receivePod())

	// Send 403 Forbidden error
	s.getFakeWatcher().Error(&metav1.Status{
		Code:   http.StatusForbidden,
		Reason: metav1.StatusReasonForbidden,
	})

	// Stop old fake watcher and create a new one for reconnection
	s.getFakeWatcher().Stop()
	s.setFakeWatcher(watch.NewFake())

	// Verify watcher reconnected by sending event on new fake watcher
	pod2 := s.createPod("pod-name-2", "pod-namespace-2", "2")
	s.getFakeWatcher().Delete(pod2)
	s.Equal(pod2, s.receivePod())
}

// TestHandles410Gone verifies that the watcher detects 410 Gone errors
// and reconnects successfully.
func (s *PodDeletionWatcherTestSuite) TestHandles410Gone() {
	pod1 := s.createPod("pod-name-1", "pod-namespace-1", "1")
	s.getFakeWatcher().Delete(pod1)
	s.Equal(pod1, s.receivePod())

	// Send 410 Gone error and the watcher should reconnect
	s.getFakeWatcher().Error(&metav1.Status{
		Code:   http.StatusGone,
		Reason: metav1.StatusReasonExpired,
	})

	// Stop old fake watcher and create a new one for reconnection
	s.getFakeWatcher().Stop()
	s.setFakeWatcher(watch.NewFake())

	// Verify watcher reconnected by sending event on new fake watcher
	pod2 := s.createPod("pod-name-2", "pod-namespace-2", "2")
	s.getFakeWatcher().Delete(pod2)
	s.Equal(pod2, s.receivePod())
}

// TestHandles429TooManyRequests verifies that the watcher handles 429 errors
// by reconnecting with backoff.
func (s *PodDeletionWatcherTestSuite) TestHandles429TooManyRequests() {
	pod1 := s.createPod("pod-name-1", "pod-namespace-1", "1")
	s.getFakeWatcher().Delete(pod1)
	s.Equal(pod1, s.receivePod())

	// Send 429 Too Many Requests error
	s.getFakeWatcher().Error(&metav1.Status{
		Code:   http.StatusTooManyRequests,
		Reason: metav1.StatusReasonTooManyRequests,
	})

	// Stop old fake watcher and create a new one for reconnection
	s.getFakeWatcher().Stop()
	s.setFakeWatcher(watch.NewFake())

	// Verify watcher reconnected by sending event on new fake watcher
	pod2 := s.createPod("pod-name-2", "pod-namespace-2", "2")
	s.getFakeWatcher().Delete(pod2)
	s.Equal(pod2, s.receivePod())
}

// TestHandles500InternalError verifies that the watcher handles 500 errors
// by reconnecting with backoff.
func (s *PodDeletionWatcherTestSuite) TestHandles500InternalError() {
	pod1 := s.createPod("pod-name-1", "pod-namespace-1", "1")
	s.getFakeWatcher().Delete(pod1)
	s.Equal(pod1, s.receivePod())

	// Send 500 Internal Server Error
	s.getFakeWatcher().Error(&metav1.Status{
		Code:   http.StatusInternalServerError,
		Reason: metav1.StatusReasonInternalError,
	})

	// Stop old fake watcher and create a new one for reconnection
	s.getFakeWatcher().Stop()
	s.setFakeWatcher(watch.NewFake())

	// Verify watcher reconnected by sending event on new fake watcher
	pod2 := s.createPod("pod-name-2", "pod-namespace-2", "2")
	s.getFakeWatcher().Delete(pod2)
	s.Equal(pod2, s.receivePod())
}

// TestHandles504GatewayTimeout verifies that the watcher handles 504 errors
// by reconnecting with backoff.
func (s *PodDeletionWatcherTestSuite) TestHandles504GatewayTimeout() {
	pod1 := s.createPod("pod-name-1", "pod-namespace-1", "1")
	s.getFakeWatcher().Delete(pod1)
	s.Equal(pod1, s.receivePod())

	// Send 504 Gateway Timeout error
	s.getFakeWatcher().Error(&metav1.Status{
		Code:   http.StatusGatewayTimeout,
		Reason: metav1.StatusReasonTimeout,
	})

	// Stop old fake watcher and create a new one for reconnection
	s.getFakeWatcher().Stop()
	s.setFakeWatcher(watch.NewFake())

	// Verify watcher reconnected by sending event on new fake watcher
	pod2 := s.createPod("pod-name-2", "pod-namespace-2", "2")
	s.getFakeWatcher().Delete(pod2)
	s.Equal(pod2, s.receivePod())
}

// TestHandlesMultipleDeletions verifies that the watcher can handle
// multiple pod deletion events in sequence.
func (s *PodDeletionWatcherTestSuite) TestHandlesMultipleDeletions() {
	pods := []*corev1.Pod{
		s.createPod("pod-name-1", "pod-namespace-1", "1"),
		s.createPod("pod-name-2", "pod-namespace-2", "2"),
		s.createPod("pod-name-3", "pod-namespace-3", "3"),
	}

	for _, pod := range pods {
		s.getFakeWatcher().Delete(pod)
	}

	for _, pod := range pods {
		s.Equal(pod, s.receivePod())
	}
}

// TestHandlesNonPodDeleteEvent verifies that the watcher gracefully
// handles delete events with non-pod objects.
func (s *PodDeletionWatcherTestSuite) TestHandlesNonPodDeleteEvent() {
	// Send a delete event with a non-pod object
	status := &metav1.Status{Message: "not a pod"}
	s.getFakeWatcher().Delete(status)

	// Verify handler was NOT called
	s.assertNoPod()
}

// TestReceivesDeleteEvents verifies that the watcher receives and processes
// pod deletion events.
func (s *PodDeletionWatcherTestSuite) TestReceivesDeleteEvents() {
	pod := s.createPod("pod-name-1", "pod-namespace-1", "1")
	s.getFakeWatcher().Delete(pod)
	s.Equal(pod, s.receivePod())
}

func (s *PodDeletionWatcherTestSuite) assertNoPod() {
	select {
	case <-s.podChan:
		s.Fail("Pod received")
	case <-time.After(1 * time.Second):
		return
	}
}

func (s *PodDeletionWatcherTestSuite) createPod(name, namespace, resourceVersion string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			ResourceVersion: resourceVersion,
		},
	}
}

func (s *PodDeletionWatcherTestSuite) getFakeWatcher() *watch.FakeWatcher {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fakeWatch
}

func (s *PodDeletionWatcherTestSuite) receivePod() *corev1.Pod {
	return <-s.podChan
}

func (s *PodDeletionWatcherTestSuite) setFakeWatcher(watcher *watch.FakeWatcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fakeWatch = watcher
}

// TestPodDeletionWatcher runs the test suite.
func TestPodDeletionWatcher(t *testing.T) {
	suite.Run(t, new(PodDeletionWatcherTestSuite))
}
