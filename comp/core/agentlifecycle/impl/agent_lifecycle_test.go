// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package agentlifecycleimpl

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentlifecycle "github.com/DataDog/datadog-agent/comp/core/agentlifecycle/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

const (
	selfPodUID = "new-pod-uid"
	daemonUID  = "daemonset-uid"
)

type scriptedPodSource struct {
	mu        sync.Mutex
	responses []podResponse
	calls     chan struct{}
}

type podResponse struct {
	pods []localPod
	err  error
}

func (s *scriptedPodSource) ListLocalPods(context.Context) ([]localPod, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case s.calls <- struct{}{}:
	default:
	}
	if len(s.responses) == 0 {
		return nil, errors.New("no scripted kubelet response")
	}
	response := s.responses[0]
	if len(s.responses) > 1 {
		s.responses = s.responses[1:]
	}
	return response.pods, response.err
}

func (s *scriptedPodSource) setResponses(responses ...podResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses = responses
}

func TestDisabledLifecycleIsNoop(t *testing.T) {
	deps := dependencies{Config: config.NewMock(t), Log: logmock.New(t)}
	comp, err := newComponent(deps, nil, "linux")
	require.NoError(t, err)
	require.NoError(t, comp.Wait(context.Background()))
	require.NoError(t, comp.Close())
}

func TestFreshPodStartsAfterTwoSuccessfulObservations(t *testing.T) {
	comp, source := newEnabledComponent(t, podResponse{pods: []localPod{selfPod()}})
	require.NoError(t, comp.Wait(context.Background()))
	require.GreaterOrEqual(t, len(source.calls), freshInstallObservations)
}

func TestReplacementWaitsForSameContainerToTerminate(t *testing.T) {
	old := siblingPod("old-pod-uid", "old-agent")
	old.containers = []localContainer{{name: "test-agent"}}
	comp, source := newEnabledComponent(t, podResponse{pods: []localPod{selfPod(), old}})

	result := make(chan error, 1)
	go func() { result <- comp.Wait(context.Background()) }()
	<-source.calls
	<-source.calls
	select {
	case err := <-result:
		t.Fatalf("replacement started while old container was running: %v", err)
	default:
	}

	now := time.Now()
	terminatedOld := siblingPod("old-pod-uid", "old-agent")
	terminatedOld.deletionTimestamp = &now
	terminatedOld.containers = []localContainer{{name: "test-agent", terminated: true}}
	source.setResponses(podResponse{pods: []localPod{selfPod(), terminatedOld}})
	require.NoError(t, <-result)
}

func TestCrashedOlderContainerDoesNotReleaseGate(t *testing.T) {
	old := siblingPod("old-pod-uid", "old-agent")
	old.containers = []localContainer{{name: "test-agent", terminated: true}}
	known := map[string]bool{old.uid: false}
	missing := map[string]int{}
	require.False(t, olderContainersStopped([]localPod{old}, known, missing, "test-agent"))

	now := time.Now()
	old.deletionTimestamp = &now
	known[old.uid] = true
	require.True(t, olderContainersStopped([]localPod{old}, known, missing, "test-agent"))
}

func TestContainersHandOffIndependently(t *testing.T) {
	old := siblingPod("old-pod-uid", "old-agent")
	old.containers = []localContainer{
		{name: "agent", terminated: true},
		{name: "system-probe"},
	}
	now := time.Now()
	old.deletionTimestamp = &now
	known := map[string]bool{old.uid: true}
	missing := map[string]int{}
	require.True(t, olderContainersStopped([]localPod{old}, known, missing, "agent"))
	require.False(t, olderContainersStopped([]localPod{old}, known, missing, "system-probe"))
}

func TestNonCoreWaitsForReplacementCoreReadiness(t *testing.T) {
	old := siblingPod("old-pod-uid", "old-agent")
	old.containers = []localContainer{{name: "test-agent", terminated: true}}
	now := time.Now()
	old.deletionTimestamp = &now
	self := selfPod()
	self.containers[0].ready = false
	comp, source := newEnabledComponent(t, podResponse{pods: []localPod{self, old}})

	result := make(chan error, 1)
	go func() { result <- comp.Wait(context.Background()) }()
	<-source.calls
	<-source.calls
	select {
	case err := <-result:
		t.Fatalf("non-core component started before replacement core was ready: %v", err)
	default:
	}
	readySelf := selfPod()
	readySelf.containers[0].ready = true
	source.setResponses(podResponse{pods: []localPod{readySelf, old}})
	require.NoError(t, <-result)
}

func TestDisappearedOlderPodReleasesGateAfterDeletionWasObserved(t *testing.T) {
	old := siblingPod("old-pod-uid", "old-agent")
	old.containers = []localContainer{{name: "test-agent"}}
	known := map[string]bool{old.uid: false}
	missing := map[string]int{}
	require.False(t, olderContainersStopped([]localPod{old}, known, missing, "test-agent"))

	now := time.Now()
	old.deletionTimestamp = &now
	known[old.uid] = true
	require.False(t, olderContainersStopped([]localPod{old}, known, missing, "test-agent"))
	require.True(t, olderContainersStopped(nil, known, missing, "test-agent"))
}

func TestMissingOlderPodWithoutObservedDeletionRequiresTwoSnapshots(t *testing.T) {
	known := map[string]bool{"old-pod-uid": false}
	missing := map[string]int{}
	require.False(t, olderContainersStopped(nil, known, missing, "test-agent"))
	require.True(t, olderContainersStopped(nil, known, missing, "test-agent"))
}

func TestKubeletErrorsFailClosed(t *testing.T) {
	comp, source := newEnabledComponent(t, podResponse{err: errors.New("kubelet unavailable")})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- comp.Wait(ctx) }()
	<-source.calls
	<-source.calls
	select {
	case err := <-result:
		t.Fatalf("kubelet error opened the gate: %v", err)
	default:
	}
	cancel()
	require.ErrorIs(t, <-result, context.Canceled)
}

func TestMissingContainerStatusRequiresPodDeletion(t *testing.T) {
	old := siblingPod("old-pod-uid", "old-agent")
	old.declaredContainers = []string{"test-agent"}
	known := map[string]bool{old.uid: false}
	missing := map[string]int{}
	require.False(t, olderContainersStopped([]localPod{old}, known, missing, "test-agent"))
	now := time.Now()
	old.deletionTimestamp = &now
	known[old.uid] = true
	require.False(t, olderContainersStopped([]localPod{old}, known, missing, "test-agent"))
}

func TestComponentAddedByNewRevisionDoesNotWait(t *testing.T) {
	old := siblingPod("old-pod-uid", "old-agent")
	old.declaredContainers = []string{"agent"}
	known := map[string]bool{old.uid: false}
	require.True(t, olderContainersStopped([]localPod{old}, known, map[string]int{}, "system-probe"))
}

func TestDifferentDaemonSetAndNewerPodDoNotBlock(t *testing.T) {
	otherDaemon := siblingPod("other-daemon-pod", "other-daemon")
	otherDaemon.owners[0].uid = "other-daemonset-uid"
	newer := siblingPod("newer-pod", "newer-agent")
	newer.createdAt = time.Unix(300, 0)

	older, err := olderSiblingPods([]localPod{selfPod(), otherDaemon, newer}, selfPodUID)
	require.NoError(t, err)
	require.Empty(t, older)
}

func TestSameTimestampFailsClosed(t *testing.T) {
	other := siblingPod("other-pod", "other-agent")
	other.createdAt = selfPod().createdAt
	_, err := olderSiblingPods([]localPod{selfPod(), other}, selfPodUID)
	require.ErrorContains(t, err, "same-timestamp")
}

func TestLifecycleRequiresValidConfiguration(t *testing.T) {
	tests := map[string]struct {
		component string
		podUID    string
		platform  string
	}{
		"missing component": {podUID: selfPodUID, platform: "linux"},
		"unsafe component":  {component: "../agent", podUID: selfPodUID, platform: "linux"},
		"missing Pod UID":   {component: "test-agent", platform: "linux"},
		"non Linux":         {component: "test-agent", podUID: selfPodUID, platform: "windows"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			deps := dependencies{
				Config: config.NewMockWithOverrides(t, map[string]interface{}{
					rolloutEnabledKey: true,
					rolloutPodUIDKey:  test.podUID,
				}),
				Log:    logmock.New(t),
				Params: agentlifecycle.Params{ComponentName: test.component},
			}
			_, err := newComponent(deps, &scriptedPodSource{}, test.platform)
			require.Error(t, err)
		})
	}
}

func newEnabledComponent(t *testing.T, responses ...podResponse) (*component, *scriptedPodSource) {
	t.Helper()
	source := &scriptedPodSource{responses: responses, calls: make(chan struct{}, 20)}
	comp, err := newComponent(enabledDependencies(t), source, "linux")
	require.NoError(t, err)
	result := comp.(*component)
	result.pollInterval = time.Millisecond
	return result, source
}

func enabledDependencies(t *testing.T) dependencies {
	return dependencies{
		Config: config.NewMockWithOverrides(t, map[string]interface{}{
			rolloutEnabledKey: true,
			rolloutPodUIDKey:  selfPodUID,
		}),
		Log:    logmock.New(t),
		Params: agentlifecycle.Params{ComponentName: "test-agent"},
	}
}

func selfPod() localPod {
	return localPod{
		uid:        selfPodUID,
		name:       "new-agent",
		namespace:  "datadog-agent",
		createdAt:  time.Unix(200, 0),
		owners:     []podOwner{{kind: "DaemonSet", uid: daemonUID, controller: true}},
		containers: []localContainer{{name: "agent", ready: true}},
	}
}

func siblingPod(uid, name string) localPod {
	pod := selfPod()
	pod.uid = uid
	pod.name = name
	pod.createdAt = time.Unix(100, 0)
	return pod
}
