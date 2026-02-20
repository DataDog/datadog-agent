// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package checks

import (
	"fmt"
	"regexp"
	"sort"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/process/metadata/parser"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func makeContainer(id string) *model.Container {
	return &model.Container{
		Id: id,
	}
}

// TestBasicProcessMessages tests basic cases for creating payloads by hard-coded scenarios
func TestBasicProcessMessages(t *testing.T) {
	const maxBatchBytes = 1000000
	p := []*procutil.Process{
		makeProcess(1, "git clone google.com"),
		makeProcess(2, "mine-bitcoins -all -x"),
		makeProcess(3, "foo --version"),
		makeProcess(4, "foo -bar -bim"),
		makeProcess(5, "datadog-process-agent --cfgpath datadog.conf"),
	}
	c := []*model.Container{
		makeContainer("foo"),
		makeContainer("bar"),
	}
	now := time.Now()
	lastRun := time.Now().Add(-5 * time.Second)
	syst1, syst2 := cpu.TimesStat{}, cpu.TimesStat{}
	sysInfo := &model.SystemInfo{}
	hostInfo := &HostInfo{SystemInfo: sysInfo}

	for i, tc := range []struct {
		testName           string
		processes          map[int32]*procutil.Process
		containers         []*model.Container
		pidToCid           map[int]string
		maxSize            int
		disallowList       []string
		expectedChunks     int
		expectedProcs      int
		expectedContainers int
	}{
		{
			testName:           "no containers",
			processes:          map[int32]*procutil.Process{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			maxSize:            2,
			containers:         []*model.Container{},
			pidToCid:           nil,
			disallowList:       []string{},
			expectedChunks:     2,
			expectedProcs:      3,
			expectedContainers: 0,
		},
		{
			testName:           "container processes",
			processes:          map[int32]*procutil.Process{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			maxSize:            2,
			containers:         []*model.Container{c[0]},
			pidToCid:           map[int]string{1: "foo", 2: "foo"},
			disallowList:       []string{},
			expectedChunks:     2,
			expectedProcs:      3,
			expectedContainers: 1,
		},
		{
			testName:           "container processes separate",
			processes:          map[int32]*procutil.Process{p[2].Pid: p[2], p[3].Pid: p[3], p[4].Pid: p[4]},
			maxSize:            1,
			containers:         []*model.Container{c[1]},
			pidToCid:           map[int]string{3: "bar"},
			disallowList:       []string{},
			expectedChunks:     3,
			expectedProcs:      3,
			expectedContainers: 1,
		},
		{
			testName:           "no non-container processes",
			processes:          map[int32]*procutil.Process{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			maxSize:            2,
			containers:         []*model.Container{c[0], c[1]},
			pidToCid:           map[int]string{1: "foo", 2: "foo", 3: "bar"},
			disallowList:       []string{},
			expectedChunks:     2,
			expectedProcs:      3,
			expectedContainers: 2,
		},
		{
			testName:           "foo processes skipped",
			processes:          map[int32]*procutil.Process{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			maxSize:            2,
			containers:         []*model.Container{c[1]},
			pidToCid:           map[int]string{3: "bar"},
			disallowList:       []string{"foo"},
			expectedChunks:     1,
			expectedProcs:      2,
			expectedContainers: 1,
		},
	} {
		t.Run(tc.testName, func(t *testing.T) {
			disallowList := make([]*regexp.Regexp, 0, len(tc.disallowList))
			for _, s := range tc.disallowList {
				disallowList = append(disallowList, regexp.MustCompile(s))
			}
			serviceExtractorEnabled := true
			useWindowsServiceName := true
			useImprovedAlgorithm := false
			ex := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
			taggerMock := fxutil.Test[taggermock.Mock](t, core.MockBundle(), taggerfxmock.MockModule(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))
			procs, _ := fmtProcesses(procutil.NewDefaultDataScrubber(), disallowList, tc.processes, tc.processes, tc.pidToCid, syst2, syst1, lastRun, nil, false, ex, nil, taggerMock, now)
			messages, totalProcs, totalContainers := createProcCtrMessages(hostInfo, procs, tc.containers, tc.maxSize, maxBatchBytes, int32(i), "nid", 0)

			assert.Equal(t, tc.expectedChunks, len(messages))

			assert.Equal(t, tc.expectedProcs, totalProcs)
			assert.Equal(t, tc.expectedContainers, totalContainers)
		})
	}
}

type ctrProc struct {
	ctrID   string
	pCounts int
}

// TestContainerProcessChunking generates processes and containers and tests that they are properly chunked
func TestContainerProcessChunking(t *testing.T) {
	const maxBatchBytes = 1000000

	for i, tc := range []struct {
		testName                            string
		ctrProcs                            []ctrProc
		expectedBatches                     []map[string]int
		expectedCtrCount, expectedProcCount int
		maxSize                             int
	}{
		{
			testName: "no containers",
			ctrProcs: []ctrProc{
				{ctrID: "", pCounts: 3},
			},
			expectedBatches: []map[string]int{
				{"": 3},
			},
			expectedProcCount: 3,
			maxSize:           10,
		},
		{
			testName: "non-container processes are chunked",
			ctrProcs: []ctrProc{
				{ctrID: "", pCounts: 8},
			},
			expectedBatches: []map[string]int{
				{"": 2},
				{"": 2},
				{"": 2},
				{"": 2},
			},
			expectedProcCount: 8,
			maxSize:           2,
		},
		{
			testName: "remaining container processes are batched",
			ctrProcs: []ctrProc{
				{ctrID: "1", pCounts: 100},
				{ctrID: "2", pCounts: 20},
				{ctrID: "3", pCounts: 30},
			},
			expectedBatches: []map[string]int{
				{"1": 50},
				{"1": 50},
				{"2": 20, "3": 30},
			},
			expectedCtrCount:  3,
			expectedProcCount: 150,
			maxSize:           50,
		},
		{
			testName: "non-container and container process are batched together",
			ctrProcs: []ctrProc{
				{ctrID: "", pCounts: 3},
				{ctrID: "1", pCounts: 4},
			},
			expectedBatches: []map[string]int{
				{"": 3, "1": 4},
			},
			expectedCtrCount:  1,
			expectedProcCount: 7,
			maxSize:           10,
		},
		{
			testName: "container process batched to size",
			ctrProcs: []ctrProc{
				{ctrID: "1", pCounts: 5},
				{ctrID: "2", pCounts: 4},
				{ctrID: "3", pCounts: 1},
				{ctrID: "4", pCounts: 1},
				{ctrID: "5", pCounts: 4},
				{ctrID: "6", pCounts: 2},
				{ctrID: "7", pCounts: 9},
			},
			expectedBatches: []map[string]int{
				{"1": 5, "2": 4, "3": 1},
				{"4": 1, "5": 4, "6": 2, "7": 3},
				{"7": 6},
			},
			expectedCtrCount:  7,
			expectedProcCount: 26,
			maxSize:           10,
		},
	} {
		t.Run(tc.testName, func(t *testing.T) {
			procs, ctrs, pidToCid := generateCtrProcs(tc.ctrProcs)
			procsByPid := procsToHash(procs)

			now := time.Now()
			lastRun := time.Now().Add(-5 * time.Second)
			syst1, syst2 := cpu.TimesStat{}, cpu.TimesStat{}
			sysInfo := &model.SystemInfo{}
			hostInfo := &HostInfo{SystemInfo: sysInfo}
			serviceExtractorEnabled := true
			useWindowsServiceName := true
			useImprovedAlgorithm := false
			ex := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
			taggerMock := fxutil.Test[taggermock.Mock](t, core.MockBundle(), taggerfxmock.MockModule(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))
			processes, _ := fmtProcesses(procutil.NewDefaultDataScrubber(), nil, procsByPid, procsByPid, pidToCid, syst2, syst1, lastRun, nil, false, ex, nil, taggerMock, now)
			messages, totalProcs, totalContainers := createProcCtrMessages(hostInfo, processes, ctrs, tc.maxSize, maxBatchBytes, int32(i), "nid", 0)

			assert.Equal(t, tc.expectedProcCount, totalProcs)
			assert.Equal(t, tc.expectedCtrCount, totalContainers)

			// sort and verify messages
			sortMsgs(messages)
			verifyBatchedMsgs(t, hostInfo, tc.expectedBatches, messages)
		})
	}
}

// sortMsgs sorts the CollectorProc messages so they can be validated deterministically
func sortMsgs(m []model.MessageBody) {
	// sort the processes and containers of each message
	for i := range m {
		payload := m[i].(*model.CollectorProc)
		sort.SliceStable(payload.Containers, func(i, j int) bool {
			return payload.Containers[i].Id < payload.Containers[j].Id
		})
		sort.SliceStable(payload.Processes, func(i, j int) bool {
			return payload.Processes[i].Pid < payload.Processes[j].Pid
		})
	}

	// sort all the messages by containers
	sort.SliceStable(m, func(i, j int) bool {
		cI := m[i].(*model.CollectorProc).Containers
		cJ := m[j].(*model.CollectorProc).Containers

		if cI == nil {
			return true
		}
		if cJ == nil {
			return false
		}

		return cI[0].Id <= cJ[0].Id
	})
}

func verifyBatchedMsgs(t *testing.T, hostInfo *HostInfo, expected []map[string]int, msgs []model.MessageBody) {
	assert := assert.New(t)

	assert.Equal(len(expected), len(msgs), "Number of messages created")

	for i, msg := range msgs {
		payload := msg.(*model.CollectorProc)

		assert.Equal(hostInfo.ContainerHostType, payload.ContainerHostType)

		actualCtrPIDCounts := map[string]int{}

		// verify number of processes for each container
		for _, proc := range payload.Processes {
			actualCtrPIDCounts[proc.ContainerId]++
		}

		assert.EqualValues(expected[i], actualCtrPIDCounts)
	}
}

// generateCtrProcs generates groups of processes for linking with containers
func generateCtrProcs(ctrProcs []ctrProc) ([]*procutil.Process, []*model.Container, map[int]string) {
	var procs []*procutil.Process
	var ctrs []*model.Container
	pidToCid := make(map[int]string)
	pid := 1

	for _, ctrProc := range ctrProcs {
		ctr := makeContainer(ctrProc.ctrID)
		if ctr.Id != emptyCtrID {
			ctrs = append(ctrs, ctr)
		}

		for i := 0; i < ctrProc.pCounts; i++ {
			proc := makeProcess(int32(pid), fmt.Sprintf("cmd %d", pid))
			procs = append(procs, proc)
			pidToCid[pid] = ctr.Id
			pid++
		}
	}
	return procs, ctrs, pidToCid
}

func TestFormatCPUTimes(t *testing.T) {
	oldHostCPUCount := hostCPUCount
	hostCPUCount = func() int {
		return 4
	}
	defer func() {
		hostCPUCount = oldHostCPUCount
	}()

	for name, test := range map[string]struct {
		statsNow   *procutil.Stats
		statsPrev  *procutil.CPUTimesStat
		timeNow    cpu.TimesStat
		timeBefore cpu.TimesStat
		expected   *model.CPUStat
	}{
		"times": {
			statsNow: &procutil.Stats{
				CPUTime: &procutil.CPUTimesStat{
					User:   101.01,
					System: 202.02,
				},
				NumThreads: 4,
				Nice:       5,
			},
			statsPrev: &procutil.CPUTimesStat{
				User:   11,
				System: 22,
			},
			timeNow:    cpu.TimesStat{User: 5000},
			timeBefore: cpu.TimesStat{User: 2500},
			expected: &model.CPUStat{
				LastCpu:    "cpu",
				TotalPct:   43.2048,
				UserPct:    14.4016,
				SystemPct:  28.8032,
				NumThreads: 4,
				Cpus:       []*model.SingleCPUStat{},
				Nice:       5,
				UserTime:   101,
				SystemTime: 202,
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, formatCPUTimes(
				test.statsNow, test.statsNow.CPUTime, test.statsPrev, test.timeNow, test.timeBefore,
			))
		})
	}
}
func TestProcessGPUTagging(t *testing.T) {
	p := []*procutil.Process{
		makeProcess(1, "git clone google.com"),
		makeProcess(2, "mine-bitcoins -all -x"),
		makeProcess(3, "foo --version"),
	}
	now := time.Now()
	lastRun := time.Now().Add(-5 * time.Second)
	syst1, syst2 := cpu.TimesStat{}, cpu.TimesStat{}
	for _, tc := range []struct {
		testName      string
		processes     map[int32]*procutil.Process
		pidToGPUTags  map[int32][]string
		expectedProcs int
	}{
		{
			testName:      "no active processes",
			processes:     map[int32]*procutil.Process{p[0].Pid: p[0]},
			expectedProcs: 1,
		},
		{
			testName:  "no matching active processes",
			processes: map[int32]*procutil.Process{p[0].Pid: p[0]},
			pidToGPUTags: map[int32][]string{
				2: {"gpu_uuid:gpu-2", "gpu_device:tesla-v100", "gpu_vendor:nvidia"},
			},
			expectedProcs: 1,
		},
		{
			testName:  "matching active processes",
			processes: map[int32]*procutil.Process{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			pidToGPUTags: map[int32][]string{
				1: {"gpu_uuid:gpu-1", "gpu_device:tesla-v100", "gpu_vendor:nvidia"},
				3: {"gpu_uuid:gpu-3", "gpu_device:tesla-v105", "gpu_vendor:nvidia"},
			},
			expectedProcs: 3,
		},
	} {
		t.Run(tc.testName, func(t *testing.T) {
			serviceExtractorEnabled := true
			useWindowsServiceName := true
			useImprovedAlgorithm := false
			ex := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)
			taggerMock := fxutil.Test[taggermock.Mock](t, core.MockBundle(), taggerfxmock.MockModule(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))
			procs, _ := fmtProcesses(procutil.NewDefaultDataScrubber(), nil, tc.processes, tc.processes, nil, syst2, syst1, lastRun, nil, false, ex, tc.pidToGPUTags, taggerMock, now)

			assert.Len(t, procs, 1)
			assert.Equal(t, tc.expectedProcs, len(procs[""]))
			for _, proc := range procs[""] {
				assert.Equal(t, proc.Tags, tc.pidToGPUTags[proc.Pid])
			}
		})
	}
}
