// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"encoding/json"
	"errors"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// TestPlatformMethodGate pins the per-platform supported set. macOS executes the three
// configuration methods and declines the three version ones, which nothing on macOS can carry out
// yet; Linux and Windows implement all eight methods and must keep doing so.
func TestPlatformMethodGate(t *testing.T) {
	darwinSupported := map[string]bool{
		methodStartConfigExperiment:   true,
		methodStopConfigExperiment:    true,
		methodPromoteConfigExperiment: true,
	}

	gate := newMethodGate()
	for _, method := range allMethods {
		if runtime.GOOS == "darwin" {
			assert.Equal(t, darwinSupported[method], gate.Supported(method), "unexpected gate decision for %s on darwin", method)
		} else {
			assert.True(t, gate.Supported(method), "%s must stay supported on %s", method, runtime.GOOS)
		}
	}
}

// TestAllMethodsCoversEveryTaskMethod guards allMethods, which the non-darwin gate is built from,
// against a method being added to the protocol and silently left out of the gate.
func TestAllMethodsCoversEveryTaskMethod(t *testing.T) {
	assert.ElementsMatch(t, []string{
		"install_package",
		"uninstall_package",
		"start_experiment",
		"stop_experiment",
		"promote_experiment",
		"start_experiment_config",
		"stop_experiment_config",
		"promote_experiment_config",
	}, allMethods)
}

func TestSupportedMethodsDecline(t *testing.T) {
	gate := supportedMethods{methodStartConfigExperiment: true}

	assert.True(t, gate.Supported(methodStartConfigExperiment))
	assert.False(t, gate.Supported(methodStartExperiment))

	err := gate.Decline(methodStartExperiment)
	require.Error(t, err)
	assert.ErrorIs(t, err, errMethodNotSupported)
	assert.Contains(t, err.Error(), methodStartExperiment)
}

// TestDeclinedMethodIsNotAcknowledged is the property the gate exists for. Reporting a declined method
// as done would be a false statement the backend acts on by advancing the deployment, so the
// request must reach the backend as an error and never as ApplyStateAcknowledged.
func TestDeclinedMethodIsNotAcknowledged(t *testing.T) {
	daemon := &daemonImpl{
		gate:     supportedMethods{},
		requests: make(chan remoteAPIRequest, 1),
	}

	var statuses []state.ApplyStatus
	handler := handleUpdaterTaskUpdate(daemon.scheduleRemoteAPIRequest)
	handler(map[string]state.RawConfig{
		"test": {Config: testRemoteAPIRequestJSON},
	}, func(_ string, status state.ApplyStatus) {
		statuses = append(statuses, status)
	})

	require.Len(t, statuses, 1)
	assert.Equal(t, state.ApplyStateError, statuses[0].State)
	assert.NotEqual(t, state.ApplyStateAcknowledged, statuses[0].State)
	assert.Contains(t, statuses[0].Error, "start_experiment")

	// The request must not have been queued for execution either: an acknowledgement is not the
	// only way to tell the backend a task ran.
	assert.Empty(t, daemon.requests, "a declined request was queued for dispatch")
}

// TestSupportedMethodPassesTheGate is the other half of the above: gating must not break the
// platforms that do implement the method.
func TestSupportedMethodPassesTheGate(t *testing.T) {
	daemon := &daemonImpl{
		gate:     supportedMethods{methodStartExperiment: true},
		requests: make(chan remoteAPIRequest, 1),
	}

	var statuses []state.ApplyStatus
	handler := handleUpdaterTaskUpdate(daemon.scheduleRemoteAPIRequest)
	handler(map[string]state.RawConfig{
		"test": {Config: testRemoteAPIRequestJSON},
	}, func(_ string, status state.ApplyStatus) {
		statuses = append(statuses, status)
	})

	require.Len(t, statuses, 1)
	assert.Equal(t, state.ApplyStateAcknowledged, statuses[0].State)
	assert.Len(t, daemon.requests, 1)
}

// TestDeclinedRequestAbortsTheRestOfTheSet pins the batch semantics handleUpdaterTaskUpdate has
// always had: the first request that does not go through ends the pass, so the requests behind it
// get no applyStateCallback and are only reconsidered on the next remote-config update.
//
// Both requests here carry a declined method, because handleUpdaterTaskUpdate walks a map: with a
// mixed set it is the iteration order, not the gate, that decides how many requests are reached.
func TestDeclinedRequestAbortsTheRestOfTheSet(t *testing.T) {
	first := requestJSON(t, "first", methodStartExperiment)
	second := requestJSON(t, "second", methodStartExperiment)

	daemon := &daemonImpl{
		gate:     supportedMethods{methodStartConfigExperiment: true},
		requests: make(chan remoteAPIRequest, 2),
	}

	statuses := map[string]state.ApplyStatus{}
	handler := handleUpdaterTaskUpdate(daemon.scheduleRemoteAPIRequest)
	handler(map[string]state.RawConfig{
		"first":  {Config: first},
		"second": {Config: second},
	}, func(id string, status state.ApplyStatus) {
		statuses[id] = status
	})

	require.Len(t, statuses, 1, "the pass continued past the declined request")
	for id, status := range statuses {
		assert.Equal(t, state.ApplyStateError, status.State, "request %s", id)
		assert.Contains(t, status.Error, methodStartExperiment, "request %s", id)
	}
	assert.Empty(t, daemon.requests, "a declined request was queued for dispatch")
}

// TestFailedRequestAbortsTheRestOfTheSet is the same property for an ordinary failure, which shares
// the code path the decline takes.
func TestFailedRequestAbortsTheRestOfTheSet(t *testing.T) {
	first := requestJSON(t, "first", methodStartExperiment)
	second := requestJSON(t, "second", methodStartExperiment)

	executed := 0
	statuses := map[string]state.ApplyStatus{}
	handler := handleUpdaterTaskUpdate(func(remoteAPIRequest) error {
		executed++
		return errors.New("boom")
	})
	handler(map[string]state.RawConfig{
		"first":  {Config: first},
		"second": {Config: second},
	}, func(id string, status state.ApplyStatus) {
		statuses[id] = status
	})

	assert.Equal(t, 1, executed, "the pass continued past the failed request")
	require.Len(t, statuses, 1)
	for id, status := range statuses {
		assert.Equal(t, state.ApplyStateError, status.State, "request %s", id)
		assert.Equal(t, "boom", status.Error, "request %s", id)
	}
}

func requestJSON(t *testing.T, id string, method string) []byte {
	t.Helper()
	raw, err := json.Marshal(remoteAPIRequest{
		ID:     id,
		Method: method,
		Params: json.RawMessage(`{"version":"7.32.0"}`),
	})
	require.NoError(t, err)
	return raw
}
