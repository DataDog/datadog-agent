// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_logs

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestListProcessLogs_HasLogFiles(t *testing.T) {
	mock := &mockWorkloadMeta{
		processes: []*workloadmeta.Process{
			{
				Pid:  1234,
				Name: "myapp",
				Exe:  "/usr/bin/myapp",
				Service: &workloadmeta.Service{
					GeneratedName: "my-service",
					LogFiles:      []string{"/var/log/myapp.log"},
				},
			},
		},
	}

	handler := NewListProcessLogsHandler(mock)
	result, err := handler.Run(context.Background(), nil, nil)
	require.NoError(t, err)

	output, ok := result.(*ListProcessLogsOutputs)
	require.True(t, ok)
	require.Len(t, output.Processes, 1)

	p := output.Processes[0]
	assert.Equal(t, int32(1234), p.PID)
	assert.Equal(t, "myapp", p.Name)
	assert.Equal(t, "/usr/bin/myapp", p.Exe)
	assert.Equal(t, "my-service", p.ServiceName)
	assert.Equal(t, []string{"/var/log/myapp.log"}, p.LogFiles)
}

func TestListProcessLogs_SkipsNilService(t *testing.T) {
	mock := &mockWorkloadMeta{
		processes: []*workloadmeta.Process{
			{
				Pid:     5678,
				Name:    "no-service",
				Service: nil,
			},
		},
	}

	handler := NewListProcessLogsHandler(mock)
	result, err := handler.Run(context.Background(), nil, nil)
	require.NoError(t, err)

	output, ok := result.(*ListProcessLogsOutputs)
	require.True(t, ok)
	assert.Empty(t, output.Processes)
}

func TestListProcessLogs_SkipsEmptyLogFiles(t *testing.T) {
	mock := &mockWorkloadMeta{
		processes: []*workloadmeta.Process{
			{
				Pid:  9999,
				Name: "no-logs",
				Service: &workloadmeta.Service{
					GeneratedName: "empty-svc",
					LogFiles:      []string{},
				},
			},
		},
	}

	handler := NewListProcessLogsHandler(mock)
	result, err := handler.Run(context.Background(), nil, nil)
	require.NoError(t, err)

	output, ok := result.(*ListProcessLogsOutputs)
	require.True(t, ok)
	assert.Empty(t, output.Processes)
}

func TestListProcessLogs_EmptyProcesses(t *testing.T) {
	mock := &mockWorkloadMeta{
		processes: []*workloadmeta.Process{},
	}

	handler := NewListProcessLogsHandler(mock)
	result, err := handler.Run(context.Background(), nil, nil)
	require.NoError(t, err)

	output, ok := result.(*ListProcessLogsOutputs)
	require.True(t, ok)
	assert.NotNil(t, output.Processes)
	assert.Empty(t, output.Processes)
}

func TestListProcessLogs_MultipleProcesses(t *testing.T) {
	mock := &mockWorkloadMeta{
		processes: []*workloadmeta.Process{
			{
				Pid:  1,
				Name: "proc1",
				Exe:  "/usr/bin/proc1",
				Service: &workloadmeta.Service{
					GeneratedName: "svc1",
					LogFiles:      []string{"/var/log/proc1.log", "/var/log/proc1-error.log"},
				},
			},
			{
				Pid:     2,
				Name:    "proc2",
				Service: nil,
			},
			{
				Pid:  3,
				Name: "proc3",
				Exe:  "/usr/bin/proc3",
				Service: &workloadmeta.Service{
					GeneratedName: "svc3",
					LogFiles:      []string{"/var/log/proc3.log"},
				},
			},
		},
	}

	handler := NewListProcessLogsHandler(mock)
	result, err := handler.Run(context.Background(), nil, nil)
	require.NoError(t, err)

	output, ok := result.(*ListProcessLogsOutputs)
	require.True(t, ok)
	require.Len(t, output.Processes, 2)

	assert.Equal(t, int32(1), output.Processes[0].PID)
	assert.Equal(t, "svc1", output.Processes[0].ServiceName)
	assert.Equal(t, []string{"/var/log/proc1.log", "/var/log/proc1-error.log"}, output.Processes[0].LogFiles)

	assert.Equal(t, int32(3), output.Processes[1].PID)
	assert.Equal(t, "svc3", output.Processes[1].ServiceName)
}

// --- mockWorkloadMeta implements the subset of workloadmeta.Component we need ---

type mockWorkloadMeta struct {
	workloadmeta.Component
	processes []*workloadmeta.Process
}

func (m *mockWorkloadMeta) ListProcesses() []*workloadmeta.Process {
	return m.processes
}
