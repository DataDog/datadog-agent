// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package coat

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapSystemdUnitState(t *testing.T) {
	tests := []struct {
		name        string
		loadState   string
		activeState string
		want        string
	}{
		{"not found", "not-found", "inactive", ProcessStateNotInstalled},
		{"active", "loaded", "active", ProcessStateRunning},
		{"activating", "loaded", "activating", ProcessStateStarting},
		{"deactivating", "loaded", "deactivating", ProcessStateStopping},
		{"failed", "loaded", "failed", ProcessStateFailed},
		{"inactive", "loaded", "inactive", ProcessStateStopped},
		{"maintenance", "loaded", "maintenance", ProcessStateStopped},
		{"unknown active", "loaded", "reloading", ProcessStateUnknown},
		{"empty active", "loaded", "", ProcessStateUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, mapSystemdUnitState(test.loadState, test.activeState))
		})
	}
}

func TestMapSystemdShowOutput(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
	}{
		{
			name: "loaded active",
			out:  "LoadState=loaded\nActiveState=active\n",
			want: ProcessStateRunning,
		},
		{
			name: "not found",
			out:  "LoadState=not-found\nActiveState=inactive\n",
			want: ProcessStateNotInstalled,
		},
		{
			name: "failed",
			out:  "ActiveState=failed\nLoadState=loaded\n",
			want: ProcessStateFailed,
		},
		{
			name: "empty",
			out:  "",
			want: ProcessStateUnknown,
		},
		{
			name: "garbage",
			out:  "something went wrong\n",
			want: ProcessStateUnknown,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, mapSystemdShowOutput(test.out))
		})
	}
}

func TestMapWindowsSCMState(t *testing.T) {
	tests := []struct {
		name  string
		state uint32
		want  string
	}{
		{"running", scmRunning, ProcessStateRunning},
		{"start pending", scmStartPending, ProcessStateStarting},
		{"stop pending", scmStopPending, ProcessStateStopping},
		{"stopped", scmStopped, ProcessStateStopped},
		{"paused", scmPaused, ProcessStateStopped},
		{"pause pending", scmPausePending, ProcessStateStopped},
		{"continue pending", scmContinuePending, ProcessStateStopped},
		{"unknown", 99, ProcessStateUnknown},
		{"zero", 0, ProcessStateUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, mapWindowsSCMState(test.state))
		})
	}
}

func TestSelectDaemonServiceState(t *testing.T) {
	tests := []struct {
		name    string
		results []daemonServiceCandidateResult
		want    string
	}{
		{
			name:    "empty results",
			results: nil,
			want:    ProcessStateNotInstalled,
		},
		{
			name: "all missing",
			results: []daemonServiceCandidateResult{
				{daemonCandidateStable, ProcessStateNotInstalled},
				{daemonCandidateExp, ProcessStateNotInstalled},
				{daemonCandidateLegacy, ProcessStateNotInstalled},
				{daemonCandidateLegacyExp, ProcessStateNotInstalled},
			},
			want: ProcessStateNotInstalled,
		},
		{
			name: "active stable beats inactive exp",
			results: []daemonServiceCandidateResult{
				{daemonCandidateStable, ProcessStateRunning},
				{daemonCandidateExp, ProcessStateStopped},
			},
			want: ProcessStateRunning,
		},
		{
			name: "running exp when stable stopped",
			results: []daemonServiceCandidateResult{
				{daemonCandidateStable, ProcessStateStopped},
				{daemonCandidateExp, ProcessStateRunning},
			},
			want: ProcessStateRunning,
		},
		{
			name: "sole starting preferred over stopped stable",
			results: []daemonServiceCandidateResult{
				{daemonCandidateStable, ProcessStateStopped},
				{daemonCandidateExp, ProcessStateStarting},
			},
			want: ProcessStateStarting,
		},
		{
			name: "sole stopping preferred over failed",
			results: []daemonServiceCandidateResult{
				{daemonCandidateStable, ProcessStateFailed},
				{daemonCandidateExp, ProcessStateStopping},
			},
			want: ProcessStateStopping,
		},
		{
			name: "two transitional fall through to stable loaded",
			results: []daemonServiceCandidateResult{
				{daemonCandidateStable, ProcessStateStarting},
				{daemonCandidateExp, ProcessStateStopping},
			},
			want: ProcessStateStarting,
		},
		{
			name: "prefer stable loaded when all terminal",
			results: []daemonServiceCandidateResult{
				{daemonCandidateStable, ProcessStateStopped},
				{daemonCandidateExp, ProcessStateFailed},
				{daemonCandidateLegacy, ProcessStateStopped},
			},
			want: ProcessStateStopped,
		},
		{
			name: "prefer exp when stable missing",
			results: []daemonServiceCandidateResult{
				{daemonCandidateStable, ProcessStateNotInstalled},
				{daemonCandidateExp, ProcessStateFailed},
				{daemonCandidateLegacy, ProcessStateStopped},
			},
			want: ProcessStateFailed,
		},
		{
			name: "prefer legacy when stable and exp missing",
			results: []daemonServiceCandidateResult{
				{daemonCandidateStable, ProcessStateNotInstalled},
				{daemonCandidateExp, ProcessStateNotInstalled},
				{daemonCandidateLegacy, ProcessStateStopped},
				{daemonCandidateLegacyExp, ProcessStateFailed},
			},
			want: ProcessStateStopped,
		},
		{
			name: "prefer legacy exp when only it is loaded",
			results: []daemonServiceCandidateResult{
				{daemonCandidateStable, ProcessStateNotInstalled},
				{daemonCandidateExp, ProcessStateNotInstalled},
				{daemonCandidateLegacy, ProcessStateNotInstalled},
				{daemonCandidateLegacyExp, ProcessStateFailed},
			},
			want: ProcessStateFailed,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, selectDaemonServiceState(test.results))
		})
	}
}

func TestDaemonServiceStateOneHot(t *testing.T) {
	for _, current := range daemonServiceStates {
		t.Run(current, func(t *testing.T) {
			active := 0
			for _, state := range daemonServiceStates {
				if daemonServiceStateIsActive(current, state) {
					active++
				}
			}
			assert.Equal(t, 1, active)
		})
	}
	assert.False(t, daemonServiceStateIsActive("", ProcessStateRunning))
	assert.False(t, daemonServiceStateIsActive(ProcessStateRunning, ProcessStateStopped))
}

func TestCollectSetsDaemonServiceStateWhenConnectFails(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("daemon service state is only collected on linux/windows")
	}

	collector := NewCollectorWithClient(t.TempDir(), &mockClient{connectErr: os.ErrNotExist})
	snapshot := collector.Collect(context.Background())

	assert.False(t, snapshot.Daemon.Reachable)
	assert.False(t, snapshot.Daemon.Ready)
	require.NotEmpty(t, snapshot.Daemon.ServiceState)
	assert.Contains(t, daemonServiceStates, snapshot.Daemon.ServiceState)
}

func TestCollectSkipsDaemonServiceStateOnOtherPlatforms(t *testing.T) {
	if runtime.GOOS == "linux" || runtime.GOOS == "windows" {
		t.Skip("other-platform stub is not exercised on linux/windows")
	}

	collector := NewCollectorWithClient(t.TempDir(), &mockClient{connectErr: os.ErrNotExist})
	snapshot := collector.Collect(context.Background())
	assert.Empty(t, snapshot.Daemon.ServiceState)
}
