// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/mcp/types"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

func TestParseProcessParams(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("mcp.tools.process.max_processes_per_request", 1000)

	handler, err := NewProcessHandler(cfg)
	require.NoError(t, err)

	tests := []struct {
		name     string
		input    map[string]interface{}
		expected *ProcessParams
	}{
		{
			name:  "empty parameters",
			input: map[string]interface{}{},
			expected: &ProcessParams{
				IncludeStats: true,
				Limit:        100,
			},
		},
		{
			name: "with PIDs",
			input: map[string]interface{}{
				"pids": []interface{}{float64(1234), float64(5678)},
			},
			expected: &ProcessParams{
				PIDs:         []int32{1234, 5678},
				IncludeStats: true,
				Limit:        100,
			},
		},
		{
			name: "with process names",
			input: map[string]interface{}{
				"process_names": []interface{}{"python", "go"},
			},
			expected: &ProcessParams{
				ProcessNames: []string{"python", "go"},
				IncludeStats: true,
				Limit:        100,
			},
		},
		{
			name: "with regex filter",
			input: map[string]interface{}{
				"regex_filter": "^python.*",
			},
			expected: &ProcessParams{
				RegexFilter:  "^python.*",
				IncludeStats: true,
				Limit:        100,
			},
		},
		{
			name: "with custom limit",
			input: map[string]interface{}{
				"limit": float64(50),
			},
			expected: &ProcessParams{
				IncludeStats: true,
				Limit:        50,
			},
		},
		{
			name: "with sort options",
			input: map[string]interface{}{
				"sort_by":   "cpu",
				"ascending": true,
			},
			expected: &ProcessParams{
				IncludeStats: true,
				Limit:        100,
				SortBy:       "cpu",
				Ascending:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params, err := handler.parseProcessParams(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected.IncludeStats, params.IncludeStats)
			assert.Equal(t, tt.expected.Limit, params.Limit)
			assert.Equal(t, tt.expected.PIDs, params.PIDs)
			assert.Equal(t, tt.expected.ProcessNames, params.ProcessNames)
			assert.Equal(t, tt.expected.RegexFilter, params.RegexFilter)
			assert.Equal(t, tt.expected.SortBy, params.SortBy)
			assert.Equal(t, tt.expected.Ascending, params.Ascending)
		})
	}
}

func TestParseProcessParamsMaxLimit(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("mcp.tools.process.max_processes_per_request", 500)

	handler, err := NewProcessHandler(cfg)
	require.NoError(t, err)

	params, err := handler.parseProcessParams(map[string]interface{}{
		"limit": float64(1000),
	})
	require.NoError(t, err)
	assert.Equal(t, 500, params.Limit)
}

func TestApplyFilters(t *testing.T) {
	cfg := configmock.New(t)
	handler, err := NewProcessHandler(cfg)
	require.NoError(t, err)

	procs := []*procutil.Process{
		{Pid: 1, Name: "init", Cmdline: []string{"/sbin/init"}},
		{Pid: 100, Name: "python", Cmdline: []string{"python", "script.py"}},
		{Pid: 200, Name: "go", Cmdline: []string{"./myapp"}},
		{Pid: 300, Name: "python3", Cmdline: []string{"python3", "app.py"}},
	}

	tests := []struct {
		name     string
		params   *ProcessParams
		expected int
		checkPID int32
	}{
		{
			name:     "no filters",
			params:   &ProcessParams{},
			expected: 4,
		},
		{
			name: "filter by PID",
			params: &ProcessParams{
				PIDs: []int32{100, 300},
			},
			expected: 2,
			checkPID: 100,
		},
		{
			name: "filter by name",
			params: &ProcessParams{
				ProcessNames: []string{"python"},
			},
			expected: 1,
			checkPID: 100,
		},
		{
			name: "filter by regex",
			params: &ProcessParams{
				RegexFilter: "^python.*",
			},
			expected: 2,
			checkPID: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := handler.applyFilters(procs, tt.params)
			assert.Len(t, filtered, tt.expected)
			if tt.checkPID > 0 {
				found := false
				for _, p := range filtered {
					if p.Pid == tt.checkPID {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected PID %d not found", tt.checkPID)
			}
		})
	}
}

func TestSortProcesses(t *testing.T) {
	cfg := configmock.New(t)
	handler, err := NewProcessHandler(cfg)
	require.NoError(t, err)

	procs := []*procutil.Process{
		{Pid: 300, Name: "zebra"},
		{Pid: 100, Name: "apple"},
		{Pid: 200, Name: "banana"},
	}

	tests := []struct {
		name      string
		sortBy    string
		ascending bool
		firstPID  int32
	}{
		{
			name:      "sort by pid ascending",
			sortBy:    "pid",
			ascending: true,
			firstPID:  100,
		},
		{
			name:      "sort by pid descending",
			sortBy:    "pid",
			ascending: false,
			firstPID:  300,
		},
		{
			name:      "sort by name ascending",
			sortBy:    "name",
			ascending: true,
			firstPID:  100,
		},
		{
			name:      "sort by name descending",
			sortBy:    "name",
			ascending: false,
			firstPID:  300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy
			testProcs := make([]*procutil.Process, len(procs))
			copy(testProcs, procs)

			params := &ProcessParams{
				SortBy:    tt.sortBy,
				Ascending: tt.ascending,
			}
			handler.sortProcesses(testProcs, params)
			assert.Equal(t, tt.firstPID, testProcs[0].Pid)
		})
	}
}

func TestConvertProcess(t *testing.T) {
	cfg := configmock.New(t)
	handler, err := NewProcessHandler(cfg)
	require.NoError(t, err)

	proc := &procutil.Process{
		Pid:      1234,
		Ppid:     1,
		Name:     "test-process",
		Exe:      "/usr/bin/test",
		Cmdline:  []string{"/usr/bin/test", "--arg"},
		Username: "testuser",
		Uids:     []int32{1000},
		Gids:     []int32{1000},
		Stats: &procutil.Stats{
			CreateTime:  1234567890,
			Status:      "R",
			OpenFdCount: 10,
			NumThreads:  5,
		},
	}

	info := handler.convertProcess(proc)
	assert.Equal(t, proc.Pid, info.PID)
	assert.Equal(t, proc.Ppid, info.PPID)
	assert.Equal(t, proc.Name, info.Name)
	assert.Equal(t, proc.Exe, info.Executable)
	assert.Equal(t, proc.Cmdline, info.CommandLine)
	assert.Equal(t, proc.Username, info.Username)
	assert.Equal(t, proc.Uids[0], info.UserID)
	assert.Equal(t, proc.Gids[0], info.GroupID)
	assert.Equal(t, proc.Stats.CreateTime, info.CreateTime)
	assert.Equal(t, proc.Stats.Status, info.Status)
	assert.Equal(t, proc.Stats.OpenFdCount, info.OpenFiles)
	assert.Equal(t, proc.Stats.NumThreads, info.NumThreads)
}

func TestHandleToolRequest(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetWithoutSource("mcp.tools.process.scrub_args", false)
	cfg.SetWithoutSource("mcp.tools.process.max_processes_per_request", 1000)

	handler, err := NewProcessHandler(cfg)
	require.NoError(t, err)
	defer handler.Close()

	req := &types.ToolRequest{
		ToolName: "GetProcessSnapshot",
		Parameters: map[string]interface{}{
			"limit": float64(10),
		},
		RequestID: "test-123",
	}

	resp, err := handler.Handle(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "GetProcessSnapshot", resp.ToolName)
	assert.Equal(t, "test-123", resp.RequestID)
	assert.Empty(t, resp.Error)
	assert.NotNil(t, resp.Result)

	// Verify the result is a ProcessSnapshot
	snapshot, ok := resp.Result.(*ProcessSnapshot)
	require.True(t, ok, "Result should be a ProcessSnapshot")
	assert.NotNil(t, snapshot.HostInfo)
	assert.NotNil(t, snapshot.Processes)
	assert.GreaterOrEqual(t, snapshot.TotalCount, 0)
}

func TestHandleToolRequestInvalidParams(t *testing.T) {
	cfg := configmock.New(t)
	handler, err := NewProcessHandler(cfg)
	require.NoError(t, err)
	defer handler.Close()

	req := &types.ToolRequest{
		ToolName: "GetProcessSnapshot",
		Parameters: map[string]interface{}{
			"limit": "invalid", // Should be a number
		},
		RequestID: "test-123",
	}

	// This shouldn't error because we have default handling
	resp, err := handler.Handle(context.Background(), req)
	require.NoError(t, err)
	assert.NotEmpty(t, resp.ToolName)
}
