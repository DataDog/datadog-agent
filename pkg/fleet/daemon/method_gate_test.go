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

// TestDeclinedRequestDoesNotSuppressTheRestOfTheSet covers a task set carrying a mix of methods:
// declining one must not cost the backend an answer about the others.
func TestDeclinedRequestDoesNotSuppressTheRestOfTheSet(t *testing.T) {
	declined := requestJSON(t, "declined", methodStartExperiment)
	supported := requestJSON(t, "supported", methodStartConfigExperiment)

	daemon := &daemonImpl{
		gate:     supportedMethods{methodStartConfigExperiment: true},
		requests: make(chan remoteAPIRequest, 2),
	}

	statuses := map[string]state.ApplyStatus{}
	handler := handleUpdaterTaskUpdate(daemon.scheduleRemoteAPIRequest)
	handler(map[string]state.RawConfig{
		"declined":  {Config: declined},
		"supported": {Config: supported},
	}, func(id string, status state.ApplyStatus) {
		statuses[id] = status
	})

	require.Len(t, statuses, 2, "the declined request suppressed the rest of the set")
	assert.Equal(t, state.ApplyStateError, statuses["declined"].State)
	assert.Equal(t, state.ApplyStateAcknowledged, statuses["supported"].State)
	assert.Len(t, daemon.requests, 1)
}

// TestFailedRequestDoesNotSuppressTheRestOfTheSet is the same property for an ordinary failure,
// which shares the code path the decline takes.
func TestFailedRequestDoesNotSuppressTheRestOfTheSet(t *testing.T) {
	first := requestJSON(t, "first", methodStartExperiment)
	second := requestJSON(t, "second", methodStartExperiment)

	statuses := map[string]state.ApplyStatus{}
	handler := handleUpdaterTaskUpdate(func(remoteAPIRequest) error {
		return errors.New("boom")
	})
	handler(map[string]state.RawConfig{
		"first":  {Config: first},
		"second": {Config: second},
	}, func(id string, status state.ApplyStatus) {
		statuses[id] = status
	})

	require.Len(t, statuses, 2)
	for id, status := range statuses {
		assert.Equal(t, state.ApplyStateError, status.State, "request %s", id)
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
