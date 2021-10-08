// +build linux

package checks

import (
	"fmt"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/gopsutil/cpu"
)

// TestBasicProcessMessages tests basic cases for creating payloads by hard-coded scenarios
func TestBasicProcessMessages(t *testing.T) {
	p := []*procutil.Process{
		makeProcess(1, "git clone google.com"),
		makeProcess(2, "mine-bitcoins -all -x"),
		makeProcess(3, "foo --version"),
		makeProcess(4, "foo -bar -bim"),
		makeProcess(5, "datadog-process-agent -ddconfig datadog.conf"),
	}
	c := []*containers.Container{
		makeContainer("foo"),
		makeContainer("bar"),
	}
	// first container runs pid1 and pid2
	c[0].Pids = []int32{1, 2}
	c[1].Pids = []int32{3}
	lastRun := time.Now().Add(-5 * time.Second)
	syst1, syst2 := cpu.TimesStat{}, cpu.TimesStat{}
	cfg := config.NewDefaultAgentConfig(false)
	sysInfo := &model.SystemInfo{}
	lastCtrRates := util.ExtractContainerRateMetric(c)

	for i, tc := range []struct {
		testName        string
		cur, last       map[int32]*procutil.Process
		containers      []*containers.Container
		maxSize         int
		blacklist       []string
		expectedChunks  int
		totalProcs      int
		totalContainers int
	}{
		{
			testName:        "no containers",
			cur:             map[int32]*procutil.Process{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			last:            map[int32]*procutil.Process{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			maxSize:         2,
			containers:      []*containers.Container{},
			blacklist:       []string{},
			expectedChunks:  2,
			totalProcs:      3,
			totalContainers: 0,
		},
		{
			testName:        "container processes",
			cur:             map[int32]*procutil.Process{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			last:            map[int32]*procutil.Process{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			maxSize:         1,
			containers:      []*containers.Container{c[0]},
			blacklist:       []string{},
			expectedChunks:  2,
			totalProcs:      3,
			totalContainers: 1,
		},
		{
			testName:        "non-container processes chunked",
			cur:             map[int32]*procutil.Process{p[2].Pid: p[2], p[3].Pid: p[3], p[4].Pid: p[4]},
			last:            map[int32]*procutil.Process{p[2].Pid: p[2], p[3].Pid: p[3], p[4].Pid: p[4]},
			maxSize:         1,
			containers:      []*containers.Container{c[1]},
			blacklist:       []string{},
			expectedChunks:  3,
			totalProcs:      3,
			totalContainers: 1,
		},
		{
			testName:        "non-container processes not chunked",
			cur:             map[int32]*procutil.Process{p[2].Pid: p[2], p[3].Pid: p[3], p[4].Pid: p[4]},
			last:            map[int32]*procutil.Process{p[2].Pid: p[2], p[3].Pid: p[3], p[4].Pid: p[4]},
			maxSize:         3,
			containers:      []*containers.Container{c[1]},
			blacklist:       []string{},
			expectedChunks:  2,
			totalProcs:      3,
			totalContainers: 1,
		},
		{
			testName:        "container processes chunked",
			cur:             map[int32]*procutil.Process{p[2].Pid: p[2], p[3].Pid: p[3], p[4].Pid: p[4]},
			last:            map[int32]*procutil.Process{p[2].Pid: p[2], p[3].Pid: p[3], p[4].Pid: p[4]},
			maxSize:         1,
			containers:      []*containers.Container{c[0], c[1]},
			blacklist:       []string{},
			expectedChunks:  3,
			totalProcs:      3,
			totalContainers: 2,
		},
		{
			testName:        "no non-container processes",
			cur:             map[int32]*procutil.Process{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			last:            map[int32]*procutil.Process{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			maxSize:         1,
			containers:      []*containers.Container{c[0], c[1]},
			blacklist:       []string{},
			expectedChunks:  1,
			totalProcs:      3,
			totalContainers: 2,
		},
		{
			testName:        "all container processes skipped",
			cur:             map[int32]*procutil.Process{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			last:            map[int32]*procutil.Process{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			maxSize:         2,
			containers:      []*containers.Container{c[1]},
			blacklist:       []string{"foo"},
			expectedChunks:  2,
			totalProcs:      2,
			totalContainers: 1,
		},
	} {
		t.Run(tc.testName, func(t *testing.T) {
			bl := make([]*regexp.Regexp, 0, len(tc.blacklist))
			for _, s := range tc.blacklist {
				bl = append(bl, regexp.MustCompile(s))
			}
			cfg.Blacklist = bl
			cfg.MaxPerMessage = tc.maxSize
			networks := make(map[int32][]*model.Connection)

			procs := fmtProcesses(cfg, tc.cur, tc.last, containersByPid(tc.containers), syst2, syst1, lastRun, networks)
			containers := fmtContainers(tc.containers, lastCtrRates, lastRun)
			messages, totalProcs, totalContainers := createProcCtrMessages(procs, containers, cfg, sysInfo, int32(i), "nid")

			assert.Equal(t, tc.expectedChunks, len(messages))
			assert.Equal(t, tc.totalProcs, totalProcs)
			assert.Equal(t, tc.totalContainers, totalContainers)
		})
	}
}

type ctrProc struct {
	ctrID   string
	pCounts int
}

// TestContainerProcessChunking generates processes and containers and tests that they are properly chunked
func TestContainerProcessChunking(t *testing.T) {
	for i, tc := range []struct {
		testName                            string
		ctrProcs                            []ctrProc
		expectedBatches                     []map[string]int
		expectedCtrCount, expectedProcCount int
		maxSize, maxCtrProcSize             int
		containerHostType                   model.ContainerHostType
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
			containerHostType: model.ContainerHostType_notSpecified,
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
			containerHostType: model.ContainerHostType_notSpecified,
		},
		{
			testName: "remaining container processes are batched",
			ctrProcs: []ctrProc{
				{ctrID: "1", pCounts: 100},
				{ctrID: "2", pCounts: 20},
				{ctrID: "3", pCounts: 30},
			},
			expectedBatches: []map[string]int{
				{"1": 100},
				{"2": 20, "3": 30},
			},
			expectedCtrCount:  3,
			expectedProcCount: 150,
			maxSize:           10,
			maxCtrProcSize:    100,
			containerHostType: model.ContainerHostType_notSpecified,
		},
		{
			testName: "non-container and container process are batched separately",
			ctrProcs: []ctrProc{
				{ctrID: "", pCounts: 3},
				{ctrID: "1", pCounts: 4},
			},
			expectedBatches: []map[string]int{
				{"": 3},
				{"1": 4},
			},
			expectedCtrCount:  1,
			expectedProcCount: 7,
			maxSize:           10,
			maxCtrProcSize:    100,
			containerHostType: model.ContainerHostType_notSpecified,
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
				{"4": 1, "5": 4, "6": 2},
				{"7": 9},
			},
			expectedCtrCount:  7,
			expectedProcCount: 26,
			maxSize:           10,
			maxCtrProcSize:    10,
			containerHostType: model.ContainerHostType_notSpecified,
		},
		{
			testName: "container with many processes gets chunked",
			ctrProcs: []ctrProc{
				{ctrID: "1", pCounts: 99},
				{ctrID: "2", pCounts: 109},
			},
			expectedBatches: []map[string]int{
				{"1": 99},
				{"2": 109},
			},
			expectedCtrCount:  2,
			expectedProcCount: 208,
			maxSize:           10,
			maxCtrProcSize:    100,
			containerHostType: model.ContainerHostType_notSpecified,
		},
		{
			testName: "container process batched with container over batch size",
			ctrProcs: []ctrProc{
				{ctrID: "", pCounts: 3},
				{ctrID: "1", pCounts: 40},
				{ctrID: "2", pCounts: 110},
				{ctrID: "3", pCounts: 80},
				{ctrID: "4", pCounts: 10},
				{ctrID: "5", pCounts: 40},
				{ctrID: "6", pCounts: 20},
				{ctrID: "7", pCounts: 90},
			},
			expectedBatches: []map[string]int{
				{"": 3},
				{"1": 40},
				{"2": 110},
				{"3": 80, "4": 10},
				{"5": 40, "6": 20},
				{"7": 90},
			},
			expectedCtrCount:  7,
			expectedProcCount: 393,
			maxSize:           10,
			maxCtrProcSize:    100,
			containerHostType: model.ContainerHostType_notSpecified,
		},
		{
			testName: "container process over batch size",
			ctrProcs: []ctrProc{
				{ctrID: "1", pCounts: 24},
				{ctrID: "2", pCounts: 45},
				{ctrID: "3", pCounts: 209},
				{ctrID: "4", pCounts: 30},
				{ctrID: "5", pCounts: 1},
				{ctrID: "6", pCounts: 1},
				{ctrID: "7", pCounts: 30},
			},
			expectedBatches: []map[string]int{
				{"1": 24, "2": 45, "4": 30, "5": 1},
				{"3": 209},
				{"6": 1, "7": 30},
			},
			expectedCtrCount:  7,
			expectedProcCount: 340,
			maxSize:           10,
			maxCtrProcSize:    100,
			containerHostType: model.ContainerHostType_notSpecified,
		},
	} {
		t.Run(tc.testName, func(t *testing.T) {
			networks := make(map[int32][]*model.Connection)
			procs, ctrs := generateCtrProcs(tc.ctrProcs)
			procsByPid := procsToHash(procs)

			lastRun := time.Now().Add(-5 * time.Second)
			syst1, syst2 := cpu.TimesStat{}, cpu.TimesStat{}
			cfg := config.NewDefaultAgentConfig(false)
			sysInfo := &model.SystemInfo{}
			lastCtrRates := util.ExtractContainerRateMetric(ctrs)
			cfg.MaxPerMessage = tc.maxSize
			cfg.MaxCtrProcessesPerMessage = tc.maxCtrProcSize
			cfg.ContainerHostType = tc.containerHostType

			processes := fmtProcesses(cfg, procsByPid, procsByPid, ctrIDForPID(ctrs), syst2, syst1, lastRun, networks)
			containers := fmtContainers(ctrs, lastCtrRates, lastRun)
			messages, totalProcs, totalContainers := createProcCtrMessages(processes, containers, cfg, sysInfo, int32(i), "nid")

			assert.Equal(t, tc.expectedProcCount, totalProcs)
			assert.Equal(t, tc.expectedCtrCount, totalContainers)

			// sort and verify messages
			sortMsgs(messages)
			verifyBatchedMsgs(t, cfg, tc.expectedBatches, messages)
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

func verifyBatchedMsgs(t *testing.T, cfg *config.AgentConfig, expected []map[string]int, msgs []model.MessageBody) {
	assert := assert.New(t)

	assert.Equal(len(expected), len(msgs), "Number of messages created")

	for i, msg := range msgs {
		payload := msg.(*model.CollectorProc)

		assert.Equal(cfg.ContainerHostType, payload.ContainerHostType)

		var actualCtrPIDCounts = map[string]int{}

		// verify number of processes for each container
		for _, proc := range payload.Processes {
			actualCtrPIDCounts[proc.ContainerId]++
		}

		assert.EqualValues(expected[i], actualCtrPIDCounts)
	}
}

// generateCtrProcs generates groups of processes for linking with containers
func generateCtrProcs(ctrProcs []ctrProc) ([]*procutil.Process, []*containers.Container) {
	var procs []*procutil.Process
	var ctrs []*containers.Container
	pid := 1

	for _, ctrProc := range ctrProcs {
		ctr := makeContainer(ctrProc.ctrID)
		if ctr.ID != emptyCtrID {
			ctrs = append(ctrs, ctr)
		}

		for i := 0; i < ctrProc.pCounts; i++ {
			proc := makeProcess(int32(pid), fmt.Sprintf("cmd %d", pid))
			ctr.Pids = append(ctr.Pids, proc.Pid)
			procs = append(procs, proc)
			pid++
		}
	}
	return procs, ctrs
}
