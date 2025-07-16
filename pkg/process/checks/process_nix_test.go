// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package checks

import (
	"fmt"
	"math/rand/v2"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	wmimpl "github.com/DataDog/datadog-agent/comp/core/workloadmeta/impl"
	gpusubscriberfxmock "github.com/DataDog/datadog-agent/comp/process/gpusubscriber/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/process/metadata"
	probeMocks "github.com/DataDog/datadog-agent/pkg/process/procutil/mocks"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/shirou/gopsutil/v4/cpu"

	"github.com/DataDog/datadog-agent/pkg/process/metadata/parser"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
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
			procs := fmtProcesses(procutil.NewDefaultDataScrubber(), disallowList, tc.processes, tc.processes, tc.pidToCid, syst2, syst1, lastRun, nil, false, ex, nil, now)
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
			processes := fmtProcesses(procutil.NewDefaultDataScrubber(), nil, procsByPid, procsByPid, pidToCid, syst2, syst1, lastRun, nil, false, ex, nil, now)
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
			procs := fmtProcesses(procutil.NewDefaultDataScrubber(), nil, tc.processes, tc.processes, nil, syst2, syst1, lastRun, nil, false, ex, tc.pidToGPUTags, now)

			assert.Len(t, procs, 1)
			assert.Equal(t, tc.expectedProcs, len(procs[""]))
			for _, proc := range procs[""] {
				assert.Equal(t, proc.Tags, tc.pidToGPUTags[proc.Pid])
			}
		})
	}
}

func processCheckWithMockProbeWLM(t *testing.T, elevatedSystemProbePermissions bool) (*ProcessCheck, *probeMocks.Probe, *wmimpl.MockWLM) {
	t.Helper()
	probe := probeMocks.NewProbe(t)
	sysInfo := &model.SystemInfo{
		Cpus: []*model.CPUInfo{
			{CoreId: "1"},
			{CoreId: "2"},
			{CoreId: "3"},
			{CoreId: "4"},
		},
	}
	hostInfo := &HostInfo{
		SystemInfo: sysInfo,
	}
	serviceExtractorEnabled := true
	useWindowsServiceName := true
	useImprovedAlgorithm := false
	serviceExtractor := parser.NewServiceExtractor(serviceExtractorEnabled, useWindowsServiceName, useImprovedAlgorithm)

	mockGpuSubscriber := gpusubscriberfxmock.SetupMockGpuSubscriber(t)

	// Workload meta + tagger
	mockWLM := wmimpl.NewMockWLM(t)

	return &ProcessCheck{
		wmeta:                   mockWLM,
		useWLMProcessCollection: true,
		probe:                   probe,
		scrubber:                procutil.NewDefaultDataScrubber(),
		hostInfo:                hostInfo,
		containerProvider:       mockContainerProvider(t),
		sysProbeConfig: &SysProbeConfig{
			ProcessModuleEnabled: elevatedSystemProbePermissions,
		},
		checkCount:       0,
		skipAmount:       2,
		serviceExtractor: serviceExtractor,
		extractors:       []metadata.Extractor{serviceExtractor},
		gpuSubscriber:    mockGpuSubscriber,
		statsd:           &statsd.NoOpClient{},
		timer:            realClock{},
	}, probe, mockWLM
}

func wlmProcessWithCreateTime(pid int32, spaceSeparatedCmdline string, creationTime int64) *wmdef.Process {
	return &wmdef.Process{
		EntityID: wmdef.EntityID{
			ID:   fmt.Sprintf("%d", pid),
			Kind: wmdef.KindProcess,
		},
		Pid:          pid,
		Cmdline:      strings.Split(spaceSeparatedCmdline, " "),
		CreationTime: time.Unix(creationTime, 0),
	}
}

func procutilCPUTimeToCPUTimeStat(t *procutil.CPUTimesStat) cpu.TimesStat {
	return cpu.TimesStat{
		CPU:       "cpu-total", // hardcoded to cpu-total when percpu is False - cpuTimer.Times(false)
		User:      t.User,
		System:    t.System,
		Idle:      t.Idle,
		Nice:      t.Nice,
		Iowait:    t.Iowait,
		Irq:       t.Irq,
		Softirq:   t.Softirq,
		Steal:     t.Steal,
		Guest:     t.Guest,
		GuestNice: t.GuestNice,
		// Other fields (like Stolen and Timestamp) are not in TimesStat
	}
}

func modelProcFromWLMProc(wlmProc *wmdef.Process, stats *procutil.Stats, lastStats *procutil.Stats, processContext []string, now time.Time, lastRunTime time.Time) *model.Process {

	return &model.Process{
		Pid:   wlmProc.Pid,
		NsPid: wlmProc.NsPid,
		//Host:                   nil, // not set
		Command: &model.Command{
			Args: wlmProc.Cmdline,
			Cwd:  wlmProc.Cwd,
			//Root:   "", // not used
			//OnDisk: false, // not used
			Ppid: wlmProc.Ppid,
			//Pgroup: 0, // not used
			Exe:  wlmProc.Exe,
			Comm: wlmProc.Comm,
		},
		User:   &model.ProcessUser{},
		Memory: formatMemory(stats),
		// TODO: test when cpu time is nil
		Cpu:        formatCPU(stats, lastStats, procutilCPUTimeToCPUTimeStat(stats.CPUTime), procutilCPUTimeToCPUTimeStat(lastStats.CPUTime)),
		CreateTime: wlmProc.CreationTime.Unix(), // process check uses stats.create_time but this should be equal to wlmProc.creation_time
		//Container:              nil, // TODO: container data not yet queried from wlm
		OpenFdCount: stats.OpenFdCount,
		State:       model.ProcessState(model.ProcessState_value[stats.Status]),
		IoStat:      formatIO(stats, lastStats.IOStat, now, lastRunTime),
		//ContainerId:            "", // TODO: container data not yet queried from wlm
		//ContainerKey:           0, // TODO: container data not yet queried from wlm
		VoluntaryCtxSwitches:   uint64(stats.CtxSwitches.Voluntary),
		InvoluntaryCtxSwitches: uint64(stats.CtxSwitches.Involuntary),
		ByteKey:                nil, // not used
		ContainerByteKey:       nil, // not used
		Networks:               nil, // not used
		ProcessContext:         processContext,
		Tags:                   nil,
	}
}

// randRange returns a random number between min and max inclusive [min, max]
func randRange(min, max int) int {
	return rand.IntN(max+1-min) + min
}

// randomUnprivilegedProcessStats returns process stats with reasonable randomized data
func randomProcessStats(createTime int64, withPriviledgedData bool) *procutil.Stats {
	proc := &procutil.Stats{CreateTime: createTime, // 1 second to 1 day ago
		Status:     []string{"U", "D", "R", "S", "T", "W", "Z"}[rand.IntN(7)], // Valid process statuses
		Nice:       int32(randRange(-20, 19)),                                 // -20 (highest priority) to 19 (lowest)
		NumThreads: int32(randRange(1, 500)),                                  // Most processes use <100 threads, upper bound for heavy apps

		CPUPercent: &procutil.CPUPercentStat{
			UserPct:   rand.Float64() * 100, // Simulate 0-100% user CPU usage
			SystemPct: rand.Float64() * 100, // Simulate 0-100% system CPU usage
		},

		CPUTime: &procutil.CPUTimesStat{
			User:      rand.Float64() * 10000, // Seconds spent in user mode
			System:    rand.Float64() * 5000,  // Seconds spent in kernel mode
			Idle:      rand.Float64() * 20000, // Idle time on thread pools or waiting
			Nice:      rand.Float64() * 1000,  // Niceness-adjusted CPU time
			Iowait:    rand.Float64() * 1000,  // Waiting on IO
			Irq:       rand.Float64() * 500,   // Time servicing IRQs
			Softirq:   rand.Float64() * 500,   // Time servicing soft IRQs
			Steal:     rand.Float64() * 100,   // Time stolen by hypervisor
			Guest:     rand.Float64() * 50,    // Guest VM time (if applicable)
			GuestNice: rand.Float64() * 50,    // Guest time with nice value
			Stolen:    rand.Float64() * 10,    // Time stolen from a virtual CPU
			Timestamp: time.Now().Unix(),      // Capture time
		},

		MemInfo: &procutil.MemoryInfoStat{
			RSS:  uint64(randRange(1<<20, 500<<20)), // 1MB–500MB resident memory
			VMS:  uint64(randRange(10<<20, 5<<30)),  // 10MB–5GB virtual memory
			Swap: uint64(randRange(0, 1<<30)),       // 0–1GB swap
		},

		MemInfoEx: &procutil.MemoryInfoExStat{
			RSS:    uint64(randRange(1<<20, 500<<20)), // should be the same as MemInfo.RSS
			VMS:    uint64(randRange(10<<20, 5<<30)),  // should be the same as MemInfo.VMS
			Shared: uint64(randRange(0, 100<<20)),     // Shared memory (e.g. mmap)
			Text:   uint64(randRange(0, 50<<20)),      // Executable code
			Lib:    uint64(randRange(0, 50<<20)),      // Shared libraries
			Data:   uint64(randRange(1<<20, 2<<30)),   // Heap/stack/data
			Dirty:  uint64(randRange(0, 10<<20)),      // Pages waiting to be written
		},

		CtxSwitches: &procutil.NumCtxSwitchesStat{
			Voluntary:   int64(randRange(0, 1_000_000)), // Caused by blocking or waiting
			Involuntary: int64(randRange(0, 500_000)),   // Caused by CPU scheduler preemption
		},
	}
	if withPriviledgedData {
		proc.IOStat = &procutil.IOCountersStat{
			ReadCount:  int64(randRange(0, 1_000_000)), // Number of read syscalls
			WriteCount: int64(randRange(0, 1_000_000)), // Number of write syscalls
			ReadBytes:  int64(randRange(0, 10<<30)),    // Up to 10GB read
			WriteBytes: int64(randRange(0, 10<<30)),    // Up to 10GB written
		}
		// IORateStat is never populated. TODO: we should probably remove it
		//proc.IORateStat = &procutil.IOCountersRateStat{}
		proc.OpenFdCount = int32(randRange(0, 5000)) // 3 minimum (stdin/out/err) to thousands for busy daemons
	}
	return proc
}

// createTestProcessData creates 5 test processes with associated random stats
func createTestWLMProcessData(createTime time.Time, elevatedPermissions bool) ([]*wmdef.Process, map[int32]*procutil.Stats) {
	wlmProcs := createTestWLMProcesses(createTime)
	statsByPid := createTestWLMProcessStats(wlmProcs, elevatedPermissions)
	return wlmProcs, statsByPid
}

func createTestWLMProcesses(createTime time.Time) []*wmdef.Process {
	nowSeconds := createTime.Unix()
	proc1 := wlmProcessWithCreateTime(1, "git clone google.com", nowSeconds)
	proc2 := wlmProcessWithCreateTime(2, "mine-bitcoins -all -x", nowSeconds+1)
	proc3 := wlmProcessWithCreateTime(3, "foo --version", nowSeconds+2)
	proc4 := wlmProcessWithCreateTime(4, "foo -bar -bim", nowSeconds+3)
	proc5 := wlmProcessWithCreateTime(5, "datadog-agent --cfgpath datadog.conf", nowSeconds+2)
	return []*wmdef.Process{proc1, proc2, proc3, proc4, proc5}
}

func createTestWLMProcessStats(wlmProcs []*wmdef.Process, elevatedPermissions bool) map[int32]*procutil.Stats {
	statsByPid := make(map[int32]*procutil.Stats, len(wlmProcs))
	for _, wlmProc := range wlmProcs {
		statsByPid[wlmProc.Pid] = randomProcessStats(wlmProc.CreationTime.Unix(), elevatedPermissions)
	}
	return statsByPid
}

type constantClock struct {
	time time.Time
}

func newConstantClock(time time.Time) *constantClock {
	return &constantClock{time: time}
}

func (c *constantClock) Now() time.Time {
	return c.time
}

func TestProcessCheckRunWLM3Times(t *testing.T) {
	for _, tc := range []struct {
		name                string
		elevatedPermissions bool
		realtimeCollection  bool
	}{
		{
			name:                "realtime DISABLED, elevated permissions DISABLED",
			elevatedPermissions: false,
			realtimeCollection:  false,
		},
		{
			name:                "realtime DISABLED, elevated permissions ENABLED",
			elevatedPermissions: true,
			realtimeCollection:  false,
		},
		{
			name:                "realtime ENABLED, elevated permissions DISABLED",
			elevatedPermissions: false,
			realtimeCollection:  true,
		},
		{
			name:                "realtime ENABLED, elevated permissions ENABLED",
			elevatedPermissions: true,
			realtimeCollection:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {

			processCheck, mockProbe, mockWLM := processCheckWithMockProbeWLM(t, tc.elevatedPermissions)
			now := time.Now()
			constantClock1 := newConstantClock(now)
			processCheck.timer = constantClock1

			// CREATE TEST DATA
			wlmProcs, statsByPid1 := createTestWLMProcessData(now, tc.elevatedPermissions)
			proc1, proc2, proc3, proc4, proc5 := wlmProcs[0], wlmProcs[1], wlmProcs[2], wlmProcs[3], wlmProcs[4]

			// MOCK WLM
			mockWLM.On("ListProcesses").Return(wlmProcs)

			// MOCK PROBE FOR STATS
			pids := make([]int32, len(wlmProcs))
			for i, wlmProc := range wlmProcs {
				pids[i] = wlmProc.Pid
			}
			mockProbe.On("StatsForPIDs", pids, mock.Anything).Return(statsByPid1, nil).Once()

			// TEST FUNCTION
			actual, err := processCheck.runWLM(0, tc.realtimeCollection)

			// The first run returns nothing because processes must be observed on two consecutive runs
			expected1 := CombinedRunResult{}
			require.NoError(t, err)
			assert.Equal(t, expected1, actual)
			// creation times should be the same
			for _, wlmProc := range wlmProcs {
				assert.Equal(t, wlmProc.CreationTime.Unix(), statsByPid1[wlmProc.Pid].CreateTime)
			}

			// SECOND RUN
			constantClock2 := newConstantClock(now.Add(10 * time.Second))
			processCheck.timer = constantClock2
			statsByPid2 := createTestWLMProcessStats(wlmProcs, tc.elevatedPermissions)
			mockProbe.On("StatsForPIDs", pids, mock.Anything).Return(statsByPid2, nil).Once()

			expected2 := []model.MessageBody{
				&model.CollectorProc{
					Processes: []*model.Process{modelProcFromWLMProc(proc1, statsByPid2[proc1.Pid], statsByPid1[proc1.Pid], []string{"process_context:git"}, constantClock2.Now(), constantClock1.Now())},
					GroupSize: int32(len(wlmProcs)),
					Info:      processCheck.hostInfo.SystemInfo,
					Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
				},
				&model.CollectorProc{
					Processes: []*model.Process{modelProcFromWLMProc(proc2, statsByPid2[proc2.Pid], statsByPid1[proc2.Pid], []string{"process_context:mine-bitcoins"}, constantClock2.Now(), constantClock1.Now())},
					GroupSize: int32(len(wlmProcs)),
					Info:      processCheck.hostInfo.SystemInfo,
					Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
				},
				&model.CollectorProc{
					Processes: []*model.Process{modelProcFromWLMProc(proc3, statsByPid2[proc3.Pid], statsByPid1[proc3.Pid], []string{"process_context:foo"}, constantClock2.Now(), constantClock1.Now())},
					GroupSize: int32(len(wlmProcs)),
					Info:      processCheck.hostInfo.SystemInfo,
					Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
				},
				&model.CollectorProc{
					Processes: []*model.Process{modelProcFromWLMProc(proc4, statsByPid2[proc4.Pid], statsByPid1[proc4.Pid], []string{"process_context:foo"}, constantClock2.Now(), constantClock1.Now())},
					GroupSize: int32(len(wlmProcs)),
					Info:      processCheck.hostInfo.SystemInfo,
					Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
				},
				&model.CollectorProc{
					Processes: []*model.Process{modelProcFromWLMProc(proc5, statsByPid2[proc5.Pid], statsByPid1[proc5.Pid], []string{"process_context:datadog-agent"}, constantClock2.Now(), constantClock1.Now())},
					GroupSize: int32(len(wlmProcs)),
					Info:      processCheck.hostInfo.SystemInfo,
					Hints:     &model.CollectorProc_HintMask{HintMask: 0b1},
				},
			}
			actual, err = processCheck.runWLM(0, tc.realtimeCollection)
			require.NoError(t, err)
			assert.ElementsMatch(t, expected2, actual.Payloads())
			// creation times should be the same
			for _, wlmProc := range wlmProcs {
				assert.Equal(t, wlmProc.CreationTime.Unix(), statsByPid2[wlmProc.Pid].CreateTime)
			}
			// realtime collection
			if tc.realtimeCollection {
				expectedStats := expectedRealtimeStats(wlmProcs, statsByPid2, statsByPid1, constantClock2.Now(), constantClock1.Now())
				require.Len(t, actual.RealtimePayloads(), 1)
				rt := actual.RealtimePayloads()[0].(*model.CollectorRealTime)
				assert.ElementsMatch(t, expectedStats, rt.Stats)
				assert.Equal(t, int32(1), rt.GroupSize)
				assert.Equal(t, int32(len(processCheck.hostInfo.SystemInfo.Cpus)), rt.NumCpus)
			} else {
				assert.Nil(t, actual.RealtimePayloads())
			}

			// THIRD RUN
			constantClock3 := newConstantClock(now.Add(20 * time.Second))
			processCheck.timer = constantClock3
			statsByPid3 := createTestWLMProcessStats(wlmProcs, tc.elevatedPermissions)
			mockProbe.On("StatsForPIDs", pids, mock.Anything).Return(statsByPid3, nil).Once()

			expected3 := []model.MessageBody{
				&model.CollectorProc{
					Processes: []*model.Process{modelProcFromWLMProc(proc1, statsByPid3[proc1.Pid], statsByPid2[proc1.Pid], []string{"process_context:git"}, constantClock3.Now(), constantClock2.Now())},
					GroupSize: int32(len(wlmProcs)),
					Info:      processCheck.hostInfo.SystemInfo,
					Hints:     &model.CollectorProc_HintMask{HintMask: 0b0},
				},
				&model.CollectorProc{
					Processes: []*model.Process{modelProcFromWLMProc(proc2, statsByPid3[proc2.Pid], statsByPid2[proc2.Pid], []string{"process_context:mine-bitcoins"}, constantClock3.Now(), constantClock2.Now())},
					GroupSize: int32(len(wlmProcs)),
					Info:      processCheck.hostInfo.SystemInfo,
					Hints:     &model.CollectorProc_HintMask{HintMask: 0b0},
				},
				&model.CollectorProc{
					Processes: []*model.Process{modelProcFromWLMProc(proc3, statsByPid3[proc3.Pid], statsByPid2[proc3.Pid], []string{"process_context:foo"}, constantClock3.Now(), constantClock2.Now())},
					GroupSize: int32(len(wlmProcs)),
					Info:      processCheck.hostInfo.SystemInfo,
					Hints:     &model.CollectorProc_HintMask{HintMask: 0b0},
				},
				&model.CollectorProc{
					Processes: []*model.Process{modelProcFromWLMProc(proc4, statsByPid3[proc4.Pid], statsByPid2[proc4.Pid], []string{"process_context:foo"}, constantClock3.Now(), constantClock2.Now())},
					GroupSize: int32(len(wlmProcs)),
					Info:      processCheck.hostInfo.SystemInfo,
					Hints:     &model.CollectorProc_HintMask{HintMask: 0b0},
				},
				&model.CollectorProc{
					Processes: []*model.Process{modelProcFromWLMProc(proc5, statsByPid3[proc5.Pid], statsByPid2[proc5.Pid], []string{"process_context:datadog-agent"}, constantClock3.Now(), constantClock2.Now())},
					GroupSize: int32(len(wlmProcs)),
					Info:      processCheck.hostInfo.SystemInfo,
					Hints:     &model.CollectorProc_HintMask{HintMask: 0b0},
				},
			}
			actual, err = processCheck.runWLM(0, tc.realtimeCollection)
			require.NoError(t, err)
			assert.ElementsMatch(t, expected3, actual.Payloads())
			// creation times should be the same
			for _, wlmProc := range wlmProcs {
				assert.Equal(t, wlmProc.CreationTime.Unix(), statsByPid3[wlmProc.Pid].CreateTime)
			}
			if tc.realtimeCollection {
				expectedStats := expectedRealtimeStats(wlmProcs, statsByPid3, statsByPid2, constantClock3.Now(), constantClock2.Now())
				require.Len(t, actual.RealtimePayloads(), 1)
				rt := actual.RealtimePayloads()[0].(*model.CollectorRealTime)
				assert.ElementsMatch(t, expectedStats, rt.Stats)
				assert.Equal(t, int32(1), rt.GroupSize)
				assert.Equal(t, int32(len(processCheck.hostInfo.SystemInfo.Cpus)), rt.NumCpus)
			} else {
				assert.Nil(t, actual.RealtimePayloads())
			}
		})
	}
}

func TestProcessCheckChunkingWLM(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		noChunking            bool
		expectedPayloadLength int
		elevatedPermissions   bool
	}{
		{
			name:                  "Chunking, no permissions",
			noChunking:            false,
			expectedPayloadLength: 5,
			elevatedPermissions:   false,
		},
		{
			name:                  "No chunking, no permissions",
			noChunking:            true,
			expectedPayloadLength: 1,
			elevatedPermissions:   false,
		},
		{
			name:                  "Chunking, yes permissions",
			noChunking:            false,
			expectedPayloadLength: 5,
			elevatedPermissions:   true,
		},
		{
			name:                  "No chunking, yes permissions",
			noChunking:            true,
			expectedPayloadLength: 1,
			elevatedPermissions:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// mock processes
			now := time.Now()
			processCheck, mockProbe, mockWLM := processCheckWithMockProbeWLM(&testing.T{}, tc.elevatedPermissions)
			wlmProcs, statsByPid := createTestWLMProcessData(now, tc.elevatedPermissions)

			// MOCK WLM
			mockWLM.On("ListProcesses").Return(wlmProcs)

			// MOCK PROBE FOR STATS
			pids := make([]int32, len(wlmProcs))
			for i, wlmProc := range wlmProcs {
				pids[i] = wlmProc.Pid
			}
			mockProbe.On("StatsForPIDs", pids, mock.Anything).Return(statsByPid, nil)

			// Set small chunk size to force chunking behavior
			processCheck.maxBatchBytes = 0
			processCheck.maxBatchSize = 0

			// Test second check runs without error and has correct number of chunks
			processCheck.Run(testGroupID(0), getChunkingOption(tc.noChunking))
			actual, err := processCheck.Run(testGroupID(0), getChunkingOption(tc.noChunking))
			require.NoError(t, err)
			assert.Len(t, actual.Payloads(), tc.expectedPayloadLength)
		})
	}
}

func expectedRealtimeStats(wlmProcs []*wmdef.Process, statsByPid map[int32]*procutil.Stats, lastStatsByPid map[int32]*procutil.Stats, now time.Time, lastRunTime time.Time) []*model.ProcessStat {
	expectedStats := make([]*model.ProcessStat, len(wlmProcs))
	for i, wlmProc := range wlmProcs {
		stats := statsByPid[wlmProc.Pid]
		lastStats := lastStatsByPid[wlmProc.Pid]
		expectedStat := &model.ProcessStat{
			Pid:                    wlmProc.Pid,
			CreateTime:             stats.CreateTime,
			Memory:                 formatMemory(stats),
			Cpu:                    formatCPU(stats, lastStats, procutilCPUTimeToCPUTimeStat(stats.CPUTime), procutilCPUTimeToCPUTimeStat(lastStats.CPUTime)),
			Nice:                   stats.Nice,
			Threads:                stats.NumThreads,
			OpenFdCount:            stats.OpenFdCount,
			ProcessState:           model.ProcessState(model.ProcessState_value[stats.Status]),
			IoStat:                 formatIO(stats, lastStats.IOStat, now, lastRunTime),
			VoluntaryCtxSwitches:   uint64(stats.CtxSwitches.Voluntary),
			InvoluntaryCtxSwitches: uint64(stats.CtxSwitches.Involuntary),
			//ContainerId:            wlmProc.ContainerID, // TODO: current not tested, will be done when container data is added
		}
		expectedStats[i] = expectedStat
	}
	return expectedStats
}

// BenchmarkProcessCheckRUNWLM does not run with the typical dda inv test, you will have to edit the invoke task
// to include the bench flag in tasks/gotest.py, here are the previous results on an m3 max
// BenchmarkProcessCheckRunWLM-16             30969             37086 ns/op           27917 B/op        301 allocs/op
// BenchmarkProcessCheck-16                   41541             28592 ns/op           22013 B/op        243 allocs/op
func BenchmarkProcessCheckRunWLM(b *testing.B) {
	elevatedPermissions := true
	processCheck, mockProbe, mockWLM := processCheckWithMockProbeWLM(&testing.T{}, elevatedPermissions)
	wlmProcs, statsByPid := createTestWLMProcessData(time.Now(), elevatedPermissions)

	// MOCK WLM
	mockWLM.On("ListProcesses").Return(wlmProcs)

	// MOCK PROBE FOR STATS
	pids := make([]int32, len(wlmProcs))
	for i, wlmProc := range wlmProcs {
		pids[i] = wlmProc.Pid
	}
	mockProbe.On("StatsForPIDs", pids, mock.Anything).Return(statsByPid, nil)

	for n := 0; n < b.N; n++ {
		_, err := processCheck.runWLM(0, false)
		require.NoError(b, err)
	}
}
