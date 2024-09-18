// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/procutil/mocks"
)

func processDiscoveryCheckWithMockProbe(t *testing.T) (*ProcessDiscoveryCheck, *mocks.Probe) {
	probe := mocks.NewProbe(t)
	sysInfo := &model.SystemInfo{
		Cpus: []*model.CPUInfo{
			{CoreId: "1"},
			{CoreId: "2"},
			{CoreId: "3"},
			{CoreId: "4"},
		},
	}
	info := &HostInfo{
		SystemInfo: sysInfo,
	}

	return &ProcessDiscoveryCheck{
		probe:      probe,
		scrubber:   procutil.NewDefaultDataScrubber(),
		info:       info,
		userProbe:  &LookupIdProbe{},
		initCalled: true,
	}, probe
}

func TestProcessDiscoveryCheck(t *testing.T) {
	prev := getMaxBatchSize
	defer func() {
		getMaxBatchSize = prev
	}()

	maxBatchSize := 10
	getMaxBatchSize = func(pkgconfigmodel.Reader) int { return maxBatchSize }

	check := NewProcessDiscoveryCheck(configmock.New(t))
	check.Init(
		&SysProbeConfig{},
		&HostInfo{
			SystemInfo: &model.SystemInfo{
				Cpus:        []*model.CPUInfo{{Number: 0}},
				TotalMemory: 0,
			},
		},
		true,
	)

	// Test check runs without error
	result, err := check.Run(testGroupID(0), nil)
	assert.NoError(t, err)

	// Test that result has the proper number of chunks, and that those chunks are of the correct type
	for _, elem := range result.Payloads() {
		assert.IsType(t, &model.CollectorProcDiscovery{}, elem)
		collectorProcDiscovery := elem.(*model.CollectorProcDiscovery)
		for _, proc := range collectorProcDiscovery.ProcessDiscoveries {
			assert.Empty(t, proc.Host)
		}
		if len(collectorProcDiscovery.ProcessDiscoveries) > maxBatchSize {
			t.Errorf("Expected less than %d messages in chunk, got %d",
				maxBatchSize, len(collectorProcDiscovery.ProcessDiscoveries))
		}
	}
}

func TestProcessDiscoveryCheckChunking(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		noChunking            bool
		expectedPayloadLength int
	}{
		{
			name:                  "Chunking",
			noChunking:            false,
			expectedPayloadLength: 5,
		},
		{
			name:                  "No chunking",
			noChunking:            true,
			expectedPayloadLength: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			check, probe := processDiscoveryCheckWithMockProbe(t)

			// Set small chunk size to force chunking behavior
			check.maxBatchSize = 1

			// mock processes
			now := time.Now().Unix()
			proc1 := makeProcessWithCreateTime(1, "git clone google.com", now)
			proc2 := makeProcessWithCreateTime(2, "mine-bitcoins -all -x", now+1)
			proc3 := makeProcessWithCreateTime(3, "foo --version", now+2)
			proc4 := makeProcessWithCreateTime(4, "foo -bar -bim", now+3)
			proc5 := makeProcessWithCreateTime(5, "datadog-process-agent --cfgpath datadog.conf", now+2)

			processesByPid := map[int32]*procutil.Process{1: proc1, 2: proc2, 3: proc3, 4: proc4, 5: proc5}
			probe.On("ProcessesByPID", mock.Anything, mock.Anything).
				Return(processesByPid, nil)

			// Test second check runs without error and has correct number of chunks
			check.Run(testGroupID(0), getChunkingOption(tc.noChunking))
			actual, err := check.Run(testGroupID(0), getChunkingOption(tc.noChunking))
			require.NoError(t, err)
			assert.Len(t, actual.Payloads(), tc.expectedPayloadLength)
		})
	}
}

func TestProcessDiscoveryChunking(t *testing.T) {
	tests := []struct{ procs, chunkSize, expectedChunks int }{
		{100, 10, 10}, // Normal behavior
		{50, 30, 2},   // Number of chunks does not split cleanly
		{10, 100, 1},  // Larger chunk size than there are procs
		{0, 100, 0},   // No procs
	}

	for _, test := range tests {
		procs := make([]*model.ProcessDiscovery, test.procs)
		chunkedProcs := chunkProcessDiscoveries(procs, test.chunkSize)
		assert.Len(t, chunkedProcs, test.expectedChunks)
	}
}

func TestPidMapToProcDiscoveriesScrubbed(t *testing.T) {
	proc := &procutil.Process{
		Pid:      10,
		Ppid:     99,
		NsPid:    77,
		Name:     "test1",
		Cwd:      "cwd_test",
		Exe:      "exec_test",
		Comm:     "comm_test",
		Username: "usertest",
		Uids:     []int32{1, 2, 3, 4, 5, 6},
		Gids:     []int32{1, 2, 3, 4, 5, 6},
		Stats: &procutil.Stats{
			CreateTime: 1705688277,
		},
	}

	testCases := map[string]struct {
		cmdline  []string
		expected []string
	}{
		"replace sensitive word": {
			cmdline:  []string{"java", "apikey:838372"},
			expected: []string{"java", "apikey:********"},
		},
		"no replacements": {
			cmdline:  []string{"java", "key:838372"},
			expected: []string{"java", "key:838372"},
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			proc.Cmdline = testCase.cmdline
			pidMap := map[int32]*procutil.Process{
				1: proc,
			}
			scrubber := procutil.NewDefaultDataScrubber()
			rsul := pidMapToProcDiscoveries(pidMap, nil, scrubber)
			require.Len(t, rsul, 1)
			assert.Equal(t, testCase.expected, rsul[0].Command.Args)
		})
	}
}
