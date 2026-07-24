// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package agentlifecycleimpl

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	comp, err := newComponent(deps, nil, "linux", testProcessIdentity)
	require.NoError(t, err)
	require.NoError(t, comp.Wait(context.Background()))
	require.NoError(t, comp.MarkActive())
	require.NoError(t, comp.Close())
}

func TestFreshPodActivatesAfterConstruction(t *testing.T) {
	comp, _, statePath := newEnabledComponent(t, podResponse{pods: []localPod{selfPod()}})

	require.NoError(t, comp.Wait(context.Background()))
	require.Equal(t, agentlifecycle.StateActivating, readState(t, statePath))
	require.NoError(t, comp.MarkActive())
	require.Equal(t, agentlifecycle.StateActive, readState(t, statePath))
	require.NoError(t, comp.Close())
	require.Equal(t, agentlifecycle.StateStopped, readState(t, statePath))
	require.NoError(t, comp.Close(), "Close must be idempotent")
}

func TestConstructionClearsStalePreparedState(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state", "test-agent.state")
	require.NoError(t, os.MkdirAll(filepath.Dir(statePath), 0o755))
	require.NoError(t, os.WriteFile(statePath, []byte(agentlifecycle.StatePrepared), 0o644))
	deps := dependencies{
		Config: config.NewMockWithOverrides(t, map[string]interface{}{
			rolloutEnabledKey:   true,
			rolloutPodUIDKey:    selfPodUID,
			rolloutStatePathKey: statePath,
		}),
		Log:    logmock.New(t),
		Params: agentlifecycle.Params{ComponentName: "test-agent"},
	}
	_, err := newComponent(deps, &scriptedPodSource{}, "linux", testProcessIdentity)
	require.NoError(t, err)
	require.NoFileExists(t, statePath)
}

func TestStateIsBoundToCurrentProcessGeneration(t *testing.T) {
	comp, _, statePath := newEnabledComponent(t, podResponse{pods: []localPod{selfPod()}})
	require.NoError(t, comp.Wait(context.Background()))

	contents, err := os.ReadFile(statePath)
	require.NoError(t, err)
	fields := strings.Fields(string(contents))
	require.Len(t, fields, 3)
	require.Equal(t, agentlifecycle.StateActivating, fields[0])
	pid, started, err := testProcessIdentity()
	require.NoError(t, err)
	require.Equal(t, strconv.Itoa(pid), fields[1])
	require.Equal(t, started, fields[2])
}

func TestReplacementRemainsPreparedUntilSiblingDisappears(t *testing.T) {
	withOld := []localPod{selfPod(), siblingPod("old-pod-uid", "old-agent")}
	comp, source, statePath := newEnabledComponent(t, podResponse{pods: withOld})

	waitResult := make(chan error, 1)
	go func() { waitResult <- comp.Wait(context.Background()) }()

	<-source.calls
	requireStateEventually(t, statePath, agentlifecycle.StatePrepared)
	select {
	case err := <-waitResult:
		t.Fatalf("replacement activated while the old sibling was present: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	source.setResponses(podResponse{pods: []localPod{selfPod()}})
	require.NoError(t, <-waitResult)
	require.Equal(t, agentlifecycle.StateActivating, readState(t, statePath))
}

func TestKubeletErrorsFailClosedThenRecover(t *testing.T) {
	withOld := []localPod{selfPod(), siblingPod("old-pod-uid", "old-agent")}
	comp, source, statePath := newEnabledComponent(t, podResponse{err: errors.New("kubelet unavailable")})

	waitResult := make(chan error, 1)
	go func() { waitResult <- comp.Wait(context.Background()) }()
	<-source.calls
	require.NoFileExists(t, statePath, "a replacement must not become Ready before kubelet safety is established")
	select {
	case err := <-waitResult:
		t.Fatalf("kubelet failure opened the activation gate: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	source.setResponses(podResponse{pods: withOld})
	require.Eventually(t, func() bool {
		contents, err := os.ReadFile(statePath)
		return err == nil && len(strings.Fields(string(contents))) > 0 && strings.Fields(string(contents))[0] == agentlifecycle.StatePrepared
	}, time.Second, time.Millisecond)
	require.Equal(t, agentlifecycle.StatePrepared, readState(t, statePath))
	source.setResponses(podResponse{pods: []localPod{selfPod()}})
	require.NoError(t, <-waitResult)
}

func TestMissingSelfPodFailsClosed(t *testing.T) {
	comp, source, statePath := newEnabledComponent(t, podResponse{pods: []localPod{siblingPod("old-pod-uid", "old-agent")}})
	ctx, cancel := context.WithCancel(context.Background())
	waitResult := make(chan error, 1)
	go func() { waitResult <- comp.Wait(ctx) }()
	<-source.calls
	require.NoFileExists(t, statePath)
	cancel()
	require.ErrorIs(t, <-waitResult, context.Canceled)
}

func TestDifferentDaemonSetOrNamespaceDoesNotBlock(t *testing.T) {
	otherDaemon := siblingPod("other-daemon-pod", "other-daemon")
	otherDaemon.owners[0].uid = "other-daemonset-uid"
	otherNamespace := siblingPod("other-namespace-pod", "other-namespace")
	otherNamespace.namespace = "other"

	comp, _, _ := newEnabledComponent(t, podResponse{pods: []localPod{selfPod(), otherDaemon, otherNamespace}})
	require.NoError(t, comp.Wait(context.Background()))
}

func TestNonControllerOwnerDoesNotIdentifySelf(t *testing.T) {
	self := selfPod()
	self.owners[0].controller = false
	comp, source, statePath := newEnabledComponent(t, podResponse{pods: []localPod{self}})
	ctx, cancel := context.WithCancel(context.Background())
	waitResult := make(chan error, 1)
	go func() { waitResult <- comp.Wait(ctx) }()
	<-source.calls
	require.NoFileExists(t, statePath)
	cancel()
	require.ErrorIs(t, <-waitResult, context.Canceled)
}

func TestLifecycleWaitCancellation(t *testing.T) {
	withOld := []localPod{selfPod(), siblingPod("old-pod-uid", "old-agent")}
	comp, source, statePath := newEnabledComponent(t, podResponse{pods: withOld})
	ctx, cancel := context.WithCancel(context.Background())
	waitResult := make(chan error, 1)
	go func() { waitResult <- comp.Wait(ctx) }()

	<-source.calls
	requireStateEventually(t, statePath, agentlifecycle.StatePrepared)
	cancel()
	require.ErrorIs(t, <-waitResult, context.Canceled)
	require.NoError(t, comp.Close())
}

func TestLifecycleRequiresValidConfiguration(t *testing.T) {
	tests := map[string]map[string]interface{}{
		"missing Pod UID": {
			rolloutStatePathKey: filepath.Join(t.TempDir(), "test-agent.state"),
		},
		"relative state": {
			rolloutPodUIDKey:    selfPodUID,
			rolloutStatePathKey: "test-agent.state",
		},
		"shared state path": {
			rolloutPodUIDKey:    selfPodUID,
			rolloutStatePathKey: filepath.Join(t.TempDir(), "agent.state"),
		},
	}
	for name, overrides := range tests {
		t.Run(name, func(t *testing.T) {
			overrides[rolloutEnabledKey] = true
			deps := dependencies{
				Config: config.NewMockWithOverrides(t, overrides),
				Log:    logmock.New(t),
				Params: agentlifecycle.Params{ComponentName: "test-agent"},
			}
			_, err := newComponent(deps, &scriptedPodSource{}, "linux", testProcessIdentity)
			require.Error(t, err)
		})
	}
}

func TestComponentPathResolution(t *testing.T) {
	tests := map[string]struct {
		configured string
		component  string
		expected   string
	}{
		"template": {
			configured: "/var/run/datadog/{component}.state",
			component:  "core-agent",
			expected:   "/var/run/datadog/core-agent.state",
		},
		"operator expanded path": {
			configured: "/var/run/datadog/trace-agent.state",
			component:  "trace-agent",
			expected:   "/var/run/datadog/trace-agent.state",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			resolved, err := resolveComponentPath(test.configured, test.component, ".state", "test.path")
			require.NoError(t, err)
			require.Equal(t, test.expected, resolved)
		})
	}
}

func TestPreparedRolloutRejectsUnsupportedPlatform(t *testing.T) {
	require.ErrorContains(t, validatePlatform("windows"), "Linux-only")
	require.ErrorContains(t, validatePlatform("darwin"), "Linux-only")
	require.NoError(t, validatePlatform("linux"))
}

func TestComponentPathRejectsTraversalName(t *testing.T) {
	_, err := resolveComponentPath("/var/run/datadog/{component}.state", "..", ".state", "test.path")
	require.ErrorContains(t, err, "path-safe")
}

func TestMarkActiveRequiresSiblingCheck(t *testing.T) {
	comp, _, _ := newEnabledComponent(t, podResponse{pods: []localPod{selfPod()}})
	require.Error(t, comp.MarkActive())
}

func TestSiblingSelectionRejectsDuplicateSelf(t *testing.T) {
	_, err := siblingPods([]localPod{selfPod(), selfPod()}, selfPodUID)
	require.ErrorContains(t, err, "duplicate")
}

func TestOlderPodWinsAfterSimultaneousRestart(t *testing.T) {
	oldSelf := selfPod()
	oldSelf.createdAt = time.Unix(100, 0)
	newer := siblingPod("newer-pod-uid", "newer-agent")
	newer.createdAt = time.Unix(300, 0)

	blocking, err := siblingPods([]localPod{oldSelf, newer}, selfPodUID)
	require.NoError(t, err)
	require.Empty(t, blocking, "an older Pod must reactivate instead of deadlocking with a newer replacement")
}

func TestSameTimestampFailsClosed(t *testing.T) {
	self := selfPod()
	other := siblingPod("other-pod-uid", "other-agent")
	other.createdAt = self.createdAt

	_, err := siblingPods([]localPod{self, other}, selfPodUID)
	require.ErrorContains(t, err, "same-timestamp")
}

func newEnabledComponent(t *testing.T, responses ...podResponse) (agentlifecycle.Component, *scriptedPodSource, string) {
	t.Helper()
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state", "test-agent.state")
	source := &scriptedPodSource{responses: responses, calls: make(chan struct{}, 10)}
	deps := dependencies{
		Config: config.NewMockWithOverrides(t, map[string]interface{}{
			rolloutEnabledKey:   true,
			rolloutPodUIDKey:    selfPodUID,
			rolloutStatePathKey: statePath,
		}),
		Log:    logmock.New(t),
		Params: agentlifecycle.Params{ComponentName: "test-agent"},
	}
	comp, err := newComponent(deps, source, "linux", testProcessIdentity)
	require.NoError(t, err)
	comp.(*component).pollInterval = time.Millisecond
	return comp, source, statePath
}

func testProcessIdentity() (int, string, error) {
	return 4242, "123456", nil
}

func selfPod() localPod {
	return localPod{
		uid:       selfPodUID,
		name:      "new-agent",
		namespace: "datadog-agent",
		createdAt: time.Unix(200, 0),
		owners:    []podOwner{{kind: "DaemonSet", uid: daemonUID, controller: true}},
	}
}

func siblingPod(uid, name string) localPod {
	pod := selfPod()
	pod.uid = uid
	pod.name = name
	pod.createdAt = time.Unix(100, 0)
	return pod
}

func readState(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	fields := strings.Fields(string(contents))
	require.NotEmpty(t, fields)
	return fields[0]
}

func requireStateEventually(t *testing.T, path, expected string) {
	t.Helper()
	require.Eventually(t, func() bool {
		contents, err := os.ReadFile(path)
		return err == nil && len(strings.Fields(string(contents))) > 0 && strings.Fields(string(contents))[0] == expected
	}, time.Second, time.Millisecond)
}
