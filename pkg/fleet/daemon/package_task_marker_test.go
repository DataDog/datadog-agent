// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"encoding/json"
	"errors"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	stableAgentVersion     = "7.84.1-1"
	experimentAgentVersion = "7.84.4-1"
	experimentAgentURL     = "oci://example.com/datadog-agent@sha256:2fa082d512a120a814e32ddb80454efce56595b5c84a37cc1a9f90cf9cc7ba85"
)

type markerRecorder struct {
	sync.Mutex
	taskIDs     []string
	stateAtMark []pbgo.TaskState
}

func (r *markerRecorder) recorded() ([]string, []pbgo.TaskState) {
	r.Lock()
	defer r.Unlock()
	return append([]string(nil), r.taskIDs...), append([]pbgo.TaskState(nil), r.stateAtMark...)
}

func withTaskMarkerRecorder(t *testing.T, i *testInstaller) *markerRecorder {
	recorder := &markerRecorder{}
	previous := recordTaskMarker
	recordTaskMarker = func(taskID string) error {
		tasks, err := i.taskDB.GetTasksState()
		assert.NoError(t, err)
		recorder.Lock()
		recorder.taskIDs = append(recorder.taskIDs, taskID)
		recorder.stateAtMark = append(recorder.stateAtMark, tasks[datadogAgentPackage].State)
		recorder.Unlock()
		return nil
	}
	t.Cleanup(func() { recordTaskMarker = previous })
	return recorder
}

func newPackageExperimentTestInstaller(t *testing.T) (*testInstaller, *envRecorder, *markerRecorder) {
	i, envs := newDeferredRestartTestInstaller(t)
	i.rcc.SubmitCatalog(catalog{Packages: []Package{{
		Name:     datadogAgentPackage,
		Version:  experimentAgentVersion,
		URL:      experimentAgentURL,
		Platform: runtime.GOOS,
		Arch:     runtime.GOARCH,
	}}})
	return i, envs, withTaskMarkerRecorder(t, i)
}

func (i *testInstaller) expectPackageState(experiment string) {
	i.pm.On("State", mock.Anything, datadogAgentPackage).Return(repository.State{Stable: stableAgentVersion, Experiment: experiment}, nil).Once()
	i.pm.On("ConfigState", mock.Anything, datadogAgentPackage).Return(repository.State{Stable: stableConfig}, nil).Once()
}

func (i *testInstaller) submitPackageRequest(t *testing.T, id string, method string, experiment string) {
	params, err := json.Marshal(experimentTaskParams{Version: experimentAgentVersion})
	require.NoError(t, err)
	i.rcc.SubmitRequest(remoteAPIRequest{
		ID:      id,
		Method:  method,
		Package: datadogAgentPackage,
		ExpectedState: expectedState{
			InstallerVersion: version.AgentVersion,
			Stable:           stableAgentVersion,
			Experiment:       experiment,
			StableConfig:     stableConfig,
			ClientID:         i.rcc.GetClientID(),
		},
		Params: params,
	})
	i.requestsWG.Wait()
}

func taskIDIs(id string) any {
	return mock.MatchedBy(func(e *env.Env) bool { return e.TaskID == id })
}

func TestTaskMarkerRequired(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		method   string
		pkg      string
		expected bool
	}{
		{name: "start package experiment", enabled: true, method: methodStartExperiment, pkg: datadogAgentPackage, expected: true},
		{name: "stop package experiment", enabled: true, method: methodStopExperiment, pkg: datadogAgentPackage, expected: true},
		{name: "promote does not restart", enabled: true, method: methodPromoteExperiment, pkg: datadogAgentPackage},
		{name: "config experiments use the deferred launch", enabled: true, method: methodStartConfigExperiment, pkg: datadogAgentPackage},
		{name: "other package", enabled: true, method: methodStartExperiment, pkg: "datadog-apm-library-dotnet"},
		{name: "platform without the race", enabled: false, method: methodStartExperiment, pkg: datadogAgentPackage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withDeferAgentRestartToDaemon(t, tt.enabled)
			assert.Equal(t, tt.expected, taskMarkerRequired(tt.method, tt.pkg))
		})
	}
}

func TestStartPackageExperimentRecordsTaskBeforeMarker(t *testing.T) {
	withDeferAgentRestartToDaemon(t, true)
	i, _, markers := newPackageExperimentTestInstaller(t)

	i.expectPackageState("")
	i.bm.On("InstallExperiment", mock.Anything, taskIDIs("start-1"), experimentAgentURL).Return(nil).Once()

	i.submitPackageRequest(t, "start-1", methodStartExperiment, "")

	i.bm.AssertExpectations(t)
	ids, states := markers.recorded()
	assert.Equal(t, []string{"start-1"}, ids)
	assert.Equal(t, []pbgo.TaskState{pbgo.TaskState_DONE}, states, "the marker must only be written once the result is persisted")
}

func TestStopPackageExperimentRecordsTaskBeforeMarker(t *testing.T) {
	withDeferAgentRestartToDaemon(t, true)
	i, envs, markers := newPackageExperimentTestInstaller(t)

	i.expectPackageState(experimentAgentVersion)
	i.pm.On("RemoveExperiment", mock.Anything, datadogAgentPackage).Return(nil).Once()

	i.submitPackageRequest(t, "stop-1", methodStopExperiment, experimentAgentVersion)

	i.pm.AssertExpectations(t)
	ids, states := markers.recorded()
	assert.Equal(t, []string{"stop-1"}, ids)
	assert.Equal(t, []pbgo.TaskState{pbgo.TaskState_DONE}, states)
	assert.Contains(t, envs.taskIDs(), "stop-1", "the stop experiment installer must receive the task ID")
}

func TestPackageExperimentFailureStillWritesMarker(t *testing.T) {
	withDeferAgentRestartToDaemon(t, true)
	i, _, markers := newPackageExperimentTestInstaller(t)

	i.expectPackageState("")
	i.bm.On("InstallExperiment", mock.Anything, taskIDIs("start-2"), experimentAgentURL).Return(errors.New("msi failed")).Once()

	i.submitPackageRequest(t, "start-2", methodStartExperiment, "")

	ids, states := markers.recorded()
	assert.Equal(t, []string{"start-2"}, ids, "a waiting restart must not be left hanging when the request fails")
	assert.Equal(t, []pbgo.TaskState{pbgo.TaskState_ERROR}, states)
}

func TestPackageExperimentWithoutMarkerOutsideWindows(t *testing.T) {
	withDeferAgentRestartToDaemon(t, false)
	i, _, markers := newPackageExperimentTestInstaller(t)

	i.expectPackageState("")
	i.bm.On("InstallExperiment", mock.Anything, taskIDIs(""), experimentAgentURL).Return(nil).Once()

	i.submitPackageRequest(t, "start-3", methodStartExperiment, "")

	i.bm.AssertExpectations(t)
	ids, _ := markers.recorded()
	assert.Empty(t, ids)
	assert.Equal(t, pbgo.TaskState_DONE, i.persistedTask(t).State)
}

func TestPackageExperimentStateMismatchWritesNoMarker(t *testing.T) {
	withDeferAgentRestartToDaemon(t, true)
	i, _, markers := newPackageExperimentTestInstaller(t)

	i.expectPackageState(experimentAgentVersion)

	i.submitPackageRequest(t, "start-4", methodStartExperiment, "")

	i.bm.AssertNotCalled(t, "InstallExperiment", mock.Anything, mock.Anything, mock.Anything)
	ids, _ := markers.recorded()
	assert.Empty(t, ids)
	assert.Equal(t, pbgo.TaskState_INVALID_STATE, i.persistedTask(t).State)
}
