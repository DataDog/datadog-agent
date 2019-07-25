// +build linux

package checks

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/gopsutil/cpu"
	"github.com/DataDog/gopsutil/process"
)

// TestRandomizeMessage generates some processes and containers, then do a deep dive on return messages and make sure the chunk logic holds
func TestRandomizeMessages(t *testing.T) {
	for i, tc := range []struct {
		testName                                string
		pCount, cCount, cProcs, maxSize, chunks int
	}{
		{
			testName: "no-containers",
			pCount:   100,
			cCount:   0,
			cProcs:   0,
			maxSize:  30,
			chunks:   4,
		},
		{
			testName: "no-processes",
			pCount:   0,
			cCount:   30,
			cProcs:   0,
			maxSize:  10,
			chunks:   1,
		},
		{
			testName: "container-process-mixed-1",
			pCount:   100,
			cCount:   30,
			cProcs:   60,
			maxSize:  30,
			chunks:   3,
		},
		{
			testName: "container-process-mixed-2",
			pCount:   100,
			cCount:   10,
			cProcs:   60,
			maxSize:  10,
			chunks:   5,
		},
		{
			testName: "container-process-mixed-3",
			pCount:   100,
			cCount:   100,
			cProcs:   100,
			maxSize:  10,
			chunks:   1,
		},
		{
			testName: "container-process-mixed-4",
			pCount:   100,
			cCount:   17,
			cProcs:   78,
			maxSize:  10,
			chunks:   4,
		},
	} {

		t.Run(tc.testName, func(t *testing.T) {
			procs, ctrs := procCtrGenerator(tc.pCount, tc.cCount, tc.cProcs)
			procsByPid := procsToHash(procs)

			lastRun := time.Now().Add(-5 * time.Second)
			syst1, syst2 := cpu.TimesStat{}, cpu.TimesStat{}
			cfg := config.NewDefaultAgentConfig()
			sysInfo := &model.SystemInfo{}
			lastCtrRates := util.ExtractContainerRateMetric(ctrs)

			cfg.MaxPerMessage = tc.maxSize
			processes := fmtProcesses(cfg, procsByPid, procsByPid, ctrs, syst2, syst1, lastRun)
			containers := fmtContainers(ctrs, lastCtrRates, lastRun)
			messages, totalProcs, totalContainers := createProcCtrMessages(processes, containers, cfg, sysInfo, int32(i))

			assert.Equal(t, totalProcs, tc.pCount)
			assert.Equal(t, totalContainers, tc.cCount)
			procMsgsVerification(t, messages, ctrs, procs, tc.maxSize)
		})
	}
}

// TestBasicProcessMessages tests basic cases for creating payloads by hard-coded scenarios
func TestBasicProcessMessages(t *testing.T) {
	p := []*process.FilledProcess{
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
	cfg := config.NewDefaultAgentConfig()
	sysInfo := &model.SystemInfo{}
	lastCtrRates := util.ExtractContainerRateMetric(c)

	for i, tc := range []struct {
		testName        string
		cur, last       map[int32]*process.FilledProcess
		containers      []*containers.Container
		maxSize         int
		blacklist       []string
		expectedChunks  int
		totalProcs      int
		totalContainers int
	}{
		{
			testName:        "no containers",
			cur:             map[int32]*process.FilledProcess{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			last:            map[int32]*process.FilledProcess{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			maxSize:         2,
			containers:      []*containers.Container{},
			blacklist:       []string{},
			expectedChunks:  2,
			totalProcs:      3,
			totalContainers: 0,
		},
		{
			testName:        "container processes",
			cur:             map[int32]*process.FilledProcess{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			last:            map[int32]*process.FilledProcess{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			maxSize:         1,
			containers:      []*containers.Container{c[0]},
			blacklist:       []string{},
			expectedChunks:  2,
			totalProcs:      3,
			totalContainers: 1,
		},
		{
			testName:        "non-container processes chunked",
			cur:             map[int32]*process.FilledProcess{p[2].Pid: p[2], p[3].Pid: p[3], p[4].Pid: p[4]},
			last:            map[int32]*process.FilledProcess{p[2].Pid: p[2], p[3].Pid: p[3], p[4].Pid: p[4]},
			maxSize:         1,
			containers:      []*containers.Container{c[1]},
			blacklist:       []string{},
			expectedChunks:  3,
			totalProcs:      3,
			totalContainers: 1,
		},
		{
			testName:        "non-container processes not chunked",
			cur:             map[int32]*process.FilledProcess{p[2].Pid: p[2], p[3].Pid: p[3], p[4].Pid: p[4]},
			last:            map[int32]*process.FilledProcess{p[2].Pid: p[2], p[3].Pid: p[3], p[4].Pid: p[4]},
			maxSize:         3,
			containers:      []*containers.Container{c[1]},
			blacklist:       []string{},
			expectedChunks:  2,
			totalProcs:      3,
			totalContainers: 1,
		},
		{
			testName:        "no non-container processes",
			cur:             map[int32]*process.FilledProcess{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			last:            map[int32]*process.FilledProcess{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			maxSize:         1,
			containers:      []*containers.Container{c[0], c[1]},
			blacklist:       []string{},
			expectedChunks:  1,
			totalProcs:      3,
			totalContainers: 2,
		},
		{
			testName:        "all container processes skipped",
			cur:             map[int32]*process.FilledProcess{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
			last:            map[int32]*process.FilledProcess{p[0].Pid: p[0], p[1].Pid: p[1], p[2].Pid: p[2]},
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

			procs := fmtProcesses(cfg, tc.cur, tc.last, tc.containers, syst2, syst1, lastRun)
			containers := fmtContainers(tc.containers, lastCtrRates, lastRun)
			messages, totalProcs, totalContainers := createProcCtrMessages(procs, containers, cfg, sysInfo, int32(i))

			assert.Equal(t, tc.expectedChunks, len(messages))
			assert.Equal(t, tc.totalProcs, totalProcs)
			assert.Equal(t, tc.totalContainers, totalContainers)
		})
	}
}
