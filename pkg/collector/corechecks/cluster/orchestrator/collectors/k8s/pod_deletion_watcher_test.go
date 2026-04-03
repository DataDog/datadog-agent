// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package k8s

import (
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
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

// testParams holds all per-test parameters. This is made necessary by the lack of proper go routine management in
// client-go retry watcher. To work around this we want to be able to hot swap the entire set of test parameters at
// once, otherwise the retry watcher receive routine created from a previous test could concurrently access parameters
// during the next test setup.
type testParams struct {
	fakeClient *fake.Clientset
	fakeStore  *fakeStore
	fakeWatch  *watch.FakeWatcher
	mu         sync.Mutex
	podChan    chan *corev1.Pod
	recorder   *actionRecorder
	stopCh     chan struct{}
	watcher    *PodDeletionWatcher
}

func (p *testParams) createPod(name, namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			ResourceVersion: p.getFakeStore().Update(),
		},
	}
}

func (p *testParams) getFakeStore() *fakeStore {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fakeStore
}

func (p *testParams) getFakeWatcher() *watch.FakeWatcher {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.fakeWatch
}

func (p *testParams) receivePod() *corev1.Pod {
	return <-p.podChan
}

func (p *testParams) setFakeWatcher(w *watch.FakeWatcher) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fakeWatch = w
}

// PodDeletionWatcherTestSuite is a test suite for PodDeletionWatcher.
type PodDeletionWatcherTestSuite struct {
	suite.Suite
	mu     sync.Mutex
	params *testParams
}

func (s *PodDeletionWatcherTestSuite) getParams() *testParams {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.params
}

func (s *PodDeletionWatcherTestSuite) setParams(p *testParams) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.params = p
}

// SetupTest runs before each test to create a fresh fake client and watcher.
func (s *PodDeletionWatcherTestSuite) SetupTest() {
	p := &testParams{
		fakeClient: fake.NewSimpleClientset(),
		fakeStore:  NewFakeStore(),
		fakeWatch:  watch.NewFake(),
		podChan:    make(chan *corev1.Pod, 10),
		recorder:   newActionRecorder(),
		stopCh:     make(chan struct{}),
	}

	// Intercept list calls and return the store resource version
	p.fakeClient.PrependReactor("list", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		p.recorder.RecordAction(action)
		podList := &corev1.PodList{
			ListMeta: metav1.ListMeta{
				ResourceVersion: p.getFakeStore().ResourceVersion(),
			},
		}
		return true, podList, nil
	})

	// Intercept watch calls and return our fake watcher
	p.fakeClient.PrependWatchReactor("pods", func(action k8stesting.Action) (bool, watch.Interface, error) {
		p.recorder.RecordAction(action)
		return true, p.getFakeWatcher(), nil
	})

	eventHandler := func(pod *corev1.Pod) {
		p.podChan <- pod
	}

	// Create watcher with event handler (starts automatically)
	p.watcher = NewPodDeletionWatcher(p.fakeClient, eventHandler, p.stopCh)
	s.setParams(p)
}

// TearDownTest runs after each test to clean up resources.
func (s *PodDeletionWatcherTestSuite) TearDownTest() {
	p := s.getParams()
	close(p.stopCh)
	p.watcher.AwaitStop()
}

// TestFiltersNonDeleteEvents verifies that the watcher only processes
// delete events and ignores add, modify, and bookmark events.
func (s *PodDeletionWatcherTestSuite) TestFiltersNonDeleteEvents() {
	p := s.getParams()
	pod := p.createPod("pod-name-1", "pod-namespace-1")

	// Send non-delete events which should be filtered
	p.getFakeWatcher().Add(pod)
	p.getFakeWatcher().Modify(pod)

	s.assertNoPod(p)
}

// TestHandles401Unauthorized verifies that the watcher handles 401 Unauthorized errors
// by reconnecting with backoff.
func (s *PodDeletionWatcherTestSuite) TestHandles401Unauthorized() {
	p := s.getParams()
	pod1 := p.createPod("pod-name-1", "pod-namespace-1")
	p.getFakeWatcher().Delete(pod1)
	s.Equal(pod1, p.receivePod())

	// Send 401 Unauthorized error
	p.getFakeWatcher().Error(&metav1.Status{
		Code:   http.StatusUnauthorized,
		Reason: metav1.StatusReasonUnauthorized,
	})

	// Stop old fake watcher and create a new one for reconnection
	p.getFakeWatcher().Stop()
	p.setFakeWatcher(watch.NewFake())

	// Verify watcher reconnected by sending event on new fake watcher
	pod2 := p.createPod("pod-name-2", "pod-namespace-2")
	p.getFakeWatcher().Delete(pod2)
	s.Equal(pod2, p.receivePod())
}

// TestHandles403Forbidden verifies that the watcher handles 403 Forbidden errors
// by reconnecting with backoff.
func (s *PodDeletionWatcherTestSuite) TestHandles403Forbidden() {
	p := s.getParams()
	pod1 := p.createPod("pod-name-1", "pod-namespace-1")
	p.getFakeWatcher().Delete(pod1)
	s.Equal(pod1, p.receivePod())

	// Send 403 Forbidden error
	p.getFakeWatcher().Error(&metav1.Status{
		Code:   http.StatusForbidden,
		Reason: metav1.StatusReasonForbidden,
	})

	// Stop old fake watcher and create a new one for reconnection
	p.getFakeWatcher().Stop()
	p.setFakeWatcher(watch.NewFake())

	// Verify watcher reconnected by sending event on new fake watcher
	pod2 := p.createPod("pod-name-2", "pod-namespace-2")
	p.getFakeWatcher().Delete(pod2)
	s.Equal(pod2, p.receivePod())
}

// TestHandles410Gone verifies that the watcher detects 410 Gone errors
// and reconnects successfully.
func (s *PodDeletionWatcherTestSuite) TestHandles410Gone() {
	p := s.getParams()
	pod1 := p.createPod("pod-name-1", "pod-namespace-1")
	p.getFakeWatcher().Delete(pod1)
	s.Equal(pod1, p.receivePod())

	// Send 410 Gone error and the watcher should reconnect
	p.getFakeWatcher().Error(&metav1.Status{
		Code:   http.StatusGone,
		Reason: metav1.StatusReasonExpired,
	})

	// Stop old fake watcher and create a new one for reconnection
	p.getFakeWatcher().Stop()
	p.setFakeWatcher(watch.NewFake())

	// Verify watcher reconnected by sending event on new fake watcher
	pod2 := p.createPod("pod-name-2", "pod-namespace-2")
	p.getFakeWatcher().Delete(pod2)
	s.Equal(pod2, p.receivePod())
}

// TestHandles429TooManyRequests verifies that the watcher handles 429 errors
// by reconnecting with backoff.
func (s *PodDeletionWatcherTestSuite) TestHandles429TooManyRequests() {
	p := s.getParams()
	pod1 := p.createPod("pod-name-1", "pod-namespace-1")
	p.getFakeWatcher().Delete(pod1)
	s.Equal(pod1, p.receivePod())

	// Send 429 Too Many Requests error
	p.getFakeWatcher().Error(&metav1.Status{
		Code:   http.StatusTooManyRequests,
		Reason: metav1.StatusReasonTooManyRequests,
	})

	// Stop old fake watcher and create a new one for reconnection
	p.getFakeWatcher().Stop()
	p.setFakeWatcher(watch.NewFake())

	// Verify watcher reconnected by sending event on new fake watcher
	pod2 := p.createPod("pod-name-2", "pod-namespace-2")
	p.getFakeWatcher().Delete(pod2)
	s.Equal(pod2, p.receivePod())
}

// TestHandles500InternalError verifies that the watcher handles 500 errors
// by reconnecting with backoff.
func (s *PodDeletionWatcherTestSuite) TestHandles500InternalError() {
	p := s.getParams()
	pod1 := p.createPod("pod-name-1", "pod-namespace-1")
	p.getFakeWatcher().Delete(pod1)
	s.Equal(pod1, p.receivePod())

	// Send 500 Internal Server Error
	p.getFakeWatcher().Error(&metav1.Status{
		Code:   http.StatusInternalServerError,
		Reason: metav1.StatusReasonInternalError,
	})

	// Stop old fake watcher and create a new one for reconnection
	p.getFakeWatcher().Stop()
	p.setFakeWatcher(watch.NewFake())

	// Verify watcher reconnected by sending event on new fake watcher
	pod2 := p.createPod("pod-name-2", "pod-namespace-2")
	p.getFakeWatcher().Delete(pod2)
	s.Equal(pod2, p.receivePod())
}

// TestHandles504GatewayTimeout verifies that the watcher handles 504 errors
// by reconnecting with backoff.
func (s *PodDeletionWatcherTestSuite) TestHandles504GatewayTimeout() {
	p := s.getParams()
	pod1 := p.createPod("pod-name-1", "pod-namespace-1")
	p.getFakeWatcher().Delete(pod1)
	s.Equal(pod1, p.receivePod())

	// Send 504 Gateway Timeout error
	p.getFakeWatcher().Error(&metav1.Status{
		Code:   http.StatusGatewayTimeout,
		Reason: metav1.StatusReasonTimeout,
	})

	// Stop old fake watcher and create a new one for reconnection
	p.getFakeWatcher().Stop()
	p.setFakeWatcher(watch.NewFake())

	// Verify watcher reconnected by sending event on new fake watcher
	pod2 := p.createPod("pod-name-2", "pod-namespace-2")
	p.getFakeWatcher().Delete(pod2)
	s.Equal(pod2, p.receivePod())
}

// TestHandlesMultipleDeletions verifies that the watcher can handle
// multiple pod deletion events in sequence.
func (s *PodDeletionWatcherTestSuite) TestHandlesMultipleDeletions() {
	p := s.getParams()
	pods := []*corev1.Pod{
		p.createPod("pod-name-1", "pod-namespace-1"),
		p.createPod("pod-name-2", "pod-namespace-2"),
		p.createPod("pod-name-3", "pod-namespace-3"),
	}

	for _, pod := range pods {
		p.getFakeWatcher().Delete(pod)
	}

	for _, pod := range pods {
		s.Equal(pod, p.receivePod())
	}
}

// TestHandlesNonPodDeleteEvent verifies that the watcher gracefully
// handles delete events with non-pod objects.
func (s *PodDeletionWatcherTestSuite) TestHandlesNonPodDeleteEvent() {
	p := s.getParams()

	// Send a delete event with a non-pod object
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "service-account-name-1",
			Namespace: "service-account-namespace-1",
		},
	}
	p.getFakeWatcher().Delete(sa)

	// Verify handler was NOT called
	s.assertNoPod(p)
}

// TestReceivesDeleteEvents verifies that the watcher receives and processes
// pod deletion events.
func (s *PodDeletionWatcherTestSuite) TestReceivesDeleteEvents() {
	p := s.getParams()
	pod := p.createPod("pod-name-1", "pod-namespace-1")
	p.getFakeWatcher().Delete(pod)
	s.Equal(pod, p.receivePod())
}

func (s *PodDeletionWatcherTestSuite) TestResourceVersionAdvances() {
	p := s.getParams()
	pod := p.createPod("pod-name-1", "pod-namespace-1")
	p.getFakeWatcher().Delete(pod)
	s.Equal(pod, p.receivePod())

	p.recorder.Enable()

	// Close fake watcher result channel forces reconnect
	p.getFakeWatcher().Stop()

	action, received := p.recorder.ConsumeWatchAction()
	s.Require().True(received)
	s.Require().Equal(p.getFakeStore().ResourceVersion(), action.GetWatchRestrictions().ResourceVersion)
}

func (s *PodDeletionWatcherTestSuite) assertNoPod(p *testParams) {
	select {
	case <-p.podChan:
		s.Fail("Pod received")
	case <-time.After(1 * time.Second):
		return
	}
}

const (
	actionRecorderBufferSize     = 100
	actionRecorderReceiveTimeout = 5 * time.Second
)

// actionRecorder is a component for recording actions from fake client reactors.
// By default it drops all actions. Call Record to start buffering them
// into internal channels that can be later consumed.
type actionRecorder struct {
	isRecording  atomic.Bool
	listActions  chan k8stesting.ListAction
	watchActions chan k8stesting.WatchAction
}

// newActionRecorder creates a new action recorder that drops all actions until Record is called.
func newActionRecorder() *actionRecorder {
	return &actionRecorder{
		listActions:  make(chan k8stesting.ListAction, actionRecorderBufferSize),
		watchActions: make(chan k8stesting.WatchAction, actionRecorderBufferSize),
	}
}

// ConsumeListAction returns the next recorded list action, blocking until one is available or the internal timeout is
// reached. Returns the action and true if one was received, or a zero value and false on timeout.
func (r *actionRecorder) ConsumeListAction() (k8stesting.ListAction, bool) {
	select {
	case action := <-r.listActions:
		return action, true
	case <-time.After(actionRecorderReceiveTimeout):
		return nil, false
	}
}

// ConsumeWatchAction returns the next recorded watch action, blocking until one is available or the internal timeout is
// reached. Returns the action and true if one was received, or a zero value and false on timeout.
func (r *actionRecorder) ConsumeWatchAction() (k8stesting.WatchAction, bool) {
	select {
	case action := <-r.watchActions:
		return action, true
	case <-time.After(actionRecorderReceiveTimeout):
		return nil, false
	}
}

// Enable starts recording actions. Actions received before this call are dropped.
func (r *actionRecorder) Enable() {
	r.isRecording.Store(true)
}

// RecordAction should be called to record a List or Watch action.
func (r *actionRecorder) RecordAction(action k8stesting.Action) {
	if !r.isRecording.Load() {
		return
	}

	switch typedAction := action.(type) {
	case k8stesting.ListAction:
		r.listActions <- typedAction
	case k8stesting.WatchAction:
		r.watchActions <- typedAction
	}
}

const (
	storeInitialResourceVersion = 1000
)

type fakeStore struct {
	mu              sync.Mutex
	resourceVersion int
}

// NewFakeStore creates a new fake store.
func NewFakeStore() *fakeStore {
	return &fakeStore{
		resourceVersion: storeInitialResourceVersion,
	}
}

// ResourceVersion returns the store current resource version.
func (s *fakeStore) ResourceVersion() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strconv.Itoa(s.resourceVersion)
}

// Update advances the store resource version and returns the new value.
func (s *fakeStore) Update() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resourceVersion++
	return strconv.Itoa(s.resourceVersion)
}

// TestPodDeletionWatcher runs the test suite.
func TestPodDeletionWatcher(t *testing.T) {
	suite.Run(t, new(PodDeletionWatcherTestSuite))
}
