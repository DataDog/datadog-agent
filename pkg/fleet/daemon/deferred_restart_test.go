// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/exec"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/version"
)

const (
	stableConfig     = "cfg-stable"
	experimentConfig = "cfg-experiment"
)

func withDeferAgentRestartToDaemon(t *testing.T, enabled bool) {
	previous := deferAgentRestartToDaemon
	deferAgentRestartToDaemon = enabled
	t.Cleanup(func() { deferAgentRestartToDaemon = previous })
}

type envRecorder struct {
	sync.Mutex
	envs []env.Env
}

func (r *envRecorder) deferredCount() int {
	r.Lock()
	defer r.Unlock()
	count := 0
	for _, e := range r.envs {
		if e.DeferAgentRestart {
			count++
		}
	}
	return count
}

func (r *envRecorder) taskIDs() []string {
	r.Lock()
	defer r.Unlock()
	var ids []string
	for _, e := range r.envs {
		if e.TaskID != "" {
			ids = append(ids, e.TaskID)
		}
	}
	return ids
}

func newDeferredRestartTestInstaller(t *testing.T) (*testInstaller, *envRecorder) {
	i := newTestInstaller(t)
	t.Cleanup(i.Stop)
	recorder := &envRecorder{}
	i.m.Lock()
	i.installer = func(e *env.Env) installer.Installer {
		recorder.Lock()
		recorder.envs = append(recorder.envs, *e)
		recorder.Unlock()
		return i.pm
	}
	i.m.Unlock()
	i.rcc.SubmitCatalog(catalog{})
	return i, recorder
}

func (i *testInstaller) expectConfigState(experiment string) {
	i.pm.On("State", mock.Anything, datadogAgentPackage).Return(repository.State{Stable: "7.0.0"}, nil).Once()
	i.pm.On("ConfigState", mock.Anything, datadogAgentPackage).Return(repository.State{Stable: stableConfig, Experiment: experiment}, nil).Once()
}

func (i *testInstaller) submitConfigRequest(id string, method string, experiment string) {
	i.rcc.SubmitRequest(remoteAPIRequest{
		ID:      id,
		Method:  method,
		Package: datadogAgentPackage,
		ExpectedState: expectedState{
			InstallerVersion: version.AgentVersion,
			Stable:           "7.0.0",
			StableConfig:     stableConfig,
			ExperimentConfig: experiment,
			ClientID:         i.rcc.GetClientID(),
		},
	})
	i.requestsWG.Wait()
}

// persistedTaskFromCallback is persistedTask for mock callbacks, which run on the daemon goroutine
// where require must not be used.
func (i *testInstaller) persistedTaskFromCallback(t *testing.T) requestState {
	tasks, err := i.taskDB.GetTasksState()
	assert.NoError(t, err)
	return tasks[datadogAgentPackage]
}

func (i *testInstaller) persistedTask(t *testing.T) requestState {
	tasks, err := i.taskDB.GetTasksState()
	require.NoError(t, err)
	return tasks[datadogAgentPackage]
}

func TestDeferredAgentRestartCommand(t *testing.T) {
	tests := []struct {
		name     string
		enabled  bool
		method   string
		pkg      string
		expected string
	}{
		{name: "start config experiment", enabled: true, method: methodStartConfigExperiment, pkg: datadogAgentPackage, expected: exec.PostStartConfigExperimentBackgroundCommand},
		{name: "stop config experiment", enabled: true, method: methodStopConfigExperiment, pkg: datadogAgentPackage, expected: exec.PreStopConfigExperimentBackgroundCommand},
		{name: "promote config experiment", enabled: true, method: methodPromoteConfigExperiment, pkg: datadogAgentPackage, expected: exec.PostPromoteConfigExperimentBackgroundCommand},
		{name: "package experiment keeps hook restart", enabled: true, method: methodStartExperiment, pkg: datadogAgentPackage},
		{name: "install keeps hook restart", enabled: true, method: methodInstallPackage, pkg: datadogAgentPackage},
		{name: "other package", enabled: true, method: methodPromoteConfigExperiment, pkg: "datadog-apm-inject"},
		{name: "platform without deferral", enabled: false, method: methodPromoteConfigExperiment, pkg: datadogAgentPackage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withDeferAgentRestartToDaemon(t, tt.enabled)
			assert.Equal(t, tt.expected, deferredAgentRestartCommand(tt.method, tt.pkg))
		})
	}
}

func TestPromoteConfigExperimentLaunchesRestartAfterDone(t *testing.T) {
	withDeferAgentRestartToDaemon(t, true)
	i, recorder := newDeferredRestartTestInstaller(t)

	i.expectConfigState(experimentConfig)
	i.pm.On("PromoteConfigExperiment", mock.Anything, datadogAgentPackage).Return(nil).Once()
	i.pm.On("StartPackageCommandDetached", mock.Anything, datadogAgentPackage, exec.PostPromoteConfigExperimentBackgroundCommand).
		Run(func(mock.Arguments) {
			task := i.persistedTaskFromCallback(t)
			assert.Equal(t, "promote-1", task.ID)
			assert.Equal(t, pbgo.TaskState_DONE, task.State, "the result must be persisted before the restart can stop the daemon")
		}).
		Return(nil).Once()

	i.submitConfigRequest("promote-1", methodPromoteConfigExperiment, experimentConfig)

	i.pm.AssertExpectations(t)
	assert.Equal(t, pbgo.TaskState_DONE, i.persistedTask(t).State)
	assert.Equal(t, 1, recorder.deferredCount(), "only the promote subprocess should run with the restart deferred")
}

func TestStopConfigExperimentLaunchesRestartAfterDone(t *testing.T) {
	withDeferAgentRestartToDaemon(t, true)
	i, _ := newDeferredRestartTestInstaller(t)

	i.expectConfigState(experimentConfig)
	i.pm.On("RemoveConfigExperiment", mock.Anything, datadogAgentPackage).Return(nil).Once()
	i.pm.On("StartPackageCommandDetached", mock.Anything, datadogAgentPackage, exec.PreStopConfigExperimentBackgroundCommand).
		Run(func(mock.Arguments) {
			assert.Equal(t, pbgo.TaskState_DONE, i.persistedTaskFromCallback(t).State)
		}).
		Return(nil).Once()

	i.submitConfigRequest("stop-1", methodStopConfigExperiment, experimentConfig)

	i.pm.AssertExpectations(t)
}

func TestFailedStopConfigExperimentStillRestarts(t *testing.T) {
	withDeferAgentRestartToDaemon(t, true)
	i, _ := newDeferredRestartTestInstaller(t)

	i.expectConfigState(experimentConfig)
	i.pm.On("RemoveConfigExperiment", mock.Anything, datadogAgentPackage).Return(errors.New("could not remove experiment")).Once()
	i.pm.On("StartPackageCommandDetached", mock.Anything, datadogAgentPackage, exec.PreStopConfigExperimentBackgroundCommand).
		Run(func(mock.Arguments) {
			assert.Equal(t, pbgo.TaskState_ERROR, i.persistedTaskFromCallback(t).State)
		}).
		Return(nil).Once()

	i.submitConfigRequest("stop-2", methodStopConfigExperiment, experimentConfig)

	i.pm.AssertExpectations(t)
	task := i.persistedTask(t)
	assert.Equal(t, pbgo.TaskState_ERROR, task.State)
	assert.Contains(t, task.Err, "could not remove experiment")
}

func TestFailedStopConfigExperimentKeepsErrorWhenRestartFails(t *testing.T) {
	withDeferAgentRestartToDaemon(t, true)
	i, _ := newDeferredRestartTestInstaller(t)

	i.expectConfigState(experimentConfig)
	i.pm.On("RemoveConfigExperiment", mock.Anything, datadogAgentPackage).Return(errors.New("could not remove experiment")).Once()
	i.pm.On("StartPackageCommandDetached", mock.Anything, datadogAgentPackage, exec.PreStopConfigExperimentBackgroundCommand).
		Return(errors.New("access denied")).Once()

	i.submitConfigRequest("stop-3", methodStopConfigExperiment, experimentConfig)

	task := i.persistedTask(t)
	assert.Equal(t, pbgo.TaskState_ERROR, task.State)
	assert.Contains(t, task.Err, "could not remove experiment")
	assert.Contains(t, task.Err, "access denied")
}

func TestDeferredRestartLaunchFailureReportsError(t *testing.T) {
	withDeferAgentRestartToDaemon(t, true)
	i, _ := newDeferredRestartTestInstaller(t)

	i.expectConfigState(experimentConfig)
	i.pm.On("PromoteConfigExperiment", mock.Anything, datadogAgentPackage).Return(nil).Once()
	i.pm.On("StartPackageCommandDetached", mock.Anything, datadogAgentPackage, exec.PostPromoteConfigExperimentBackgroundCommand).
		Return(errors.New("access denied")).Once()

	i.submitConfigRequest("promote-2", methodPromoteConfigExperiment, experimentConfig)

	i.pm.AssertExpectations(t)
	task := i.persistedTask(t)
	assert.Equal(t, pbgo.TaskState_ERROR, task.State)
	assert.Contains(t, task.Err, "access denied")
}

func TestDeferredRestartSkippedWhenOperationFails(t *testing.T) {
	withDeferAgentRestartToDaemon(t, true)
	i, _ := newDeferredRestartTestInstaller(t)

	i.expectConfigState(experimentConfig)
	i.pm.On("PromoteConfigExperiment", mock.Anything, datadogAgentPackage).Return(errors.New("promote failed")).Once()

	i.submitConfigRequest("promote-3", methodPromoteConfigExperiment, experimentConfig)

	i.pm.AssertExpectations(t)
	i.pm.AssertNotCalled(t, "StartPackageCommandDetached", mock.Anything, mock.Anything, mock.Anything)
	assert.Equal(t, pbgo.TaskState_ERROR, i.persistedTask(t).State)
}

func TestDeferredRestartSkippedWhenStateDoesNotMatch(t *testing.T) {
	withDeferAgentRestartToDaemon(t, true)
	i, recorder := newDeferredRestartTestInstaller(t)

	i.expectConfigState("")

	i.submitConfigRequest("promote-4", methodPromoteConfigExperiment, experimentConfig)

	i.pm.AssertExpectations(t)
	i.pm.AssertNotCalled(t, "PromoteConfigExperiment", mock.Anything, mock.Anything)
	i.pm.AssertNotCalled(t, "StartPackageCommandDetached", mock.Anything, mock.Anything, mock.Anything)
	assert.Equal(t, pbgo.TaskState_INVALID_STATE, i.persistedTask(t).State)
	assert.Zero(t, recorder.deferredCount())
}

func TestConfigExperimentKeepsHookRestartWithoutDeferral(t *testing.T) {
	withDeferAgentRestartToDaemon(t, false)
	i, recorder := newDeferredRestartTestInstaller(t)

	i.expectConfigState(experimentConfig)
	i.pm.On("PromoteConfigExperiment", mock.Anything, datadogAgentPackage).Return(nil).Once()

	i.submitConfigRequest("promote-5", methodPromoteConfigExperiment, experimentConfig)

	i.pm.AssertExpectations(t)
	i.pm.AssertNotCalled(t, "StartPackageCommandDetached", mock.Anything, mock.Anything, mock.Anything)
	assert.Equal(t, pbgo.TaskState_DONE, i.persistedTask(t).State)
	assert.Zero(t, recorder.deferredCount())
}
