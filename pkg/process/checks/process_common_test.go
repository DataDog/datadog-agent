package checks

import (
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/gopsutil/cpu"
)

//nolint:unused
func makeContainer(id string) *containers.Container {
	return &containers.Container{
		ID: id,
		ContainerMetrics: metrics.ContainerMetrics{
			CPU:    &metrics.ContainerCPUStats{},
			Memory: &metrics.ContainerMemStats{},
			IO:     &metrics.ContainerIOStats{},
		},
	}
}

//nolint:deadcode,unused
func procCtrGenerator(pCount int, cCount int, containeredProcs int) ([]*procutil.Process, []*containers.Container) {
	procs := make([]*procutil.Process, 0, pCount)
	for i := 0; i < pCount; i++ {
		procs = append(procs, makeProcess(int32(i), strconv.Itoa(i)))
	}

	ctrs := make([]*containers.Container, 0, cCount)
	for i := 0; i < cCount; i++ {
		ctrs = append(ctrs, makeContainer(strconv.Itoa(i)))
	}

	// build container process relationship
	ctrIdx := 0
	for i := 0; i < containeredProcs; i++ {
		// reset to 0 if hit the last one
		if ctrIdx == cCount {
			ctrIdx = 0
		}
		ctrs[ctrIdx].Pids = append(ctrs[ctrIdx].Pids, procs[i].Pid)
		ctrIdx++
	}

	return procs, ctrs
}

func containersByPid(ctrs []*containers.Container) map[int32]string {
	ctrsByPid := make(map[int32]string)
	for _, c := range ctrs {
		for _, p := range c.Pids {
			ctrsByPid[p] = c.ID
		}
	}

	return ctrsByPid
}

//nolint:deadcode,unused
func procsToHash(procs []*procutil.Process) (procsByPid map[int32]*procutil.Process) {
	procsByPid = make(map[int32]*procutil.Process)
	for _, p := range procs {
		procsByPid[p.Pid] = p
	}
	return
}

func makeProcess(pid int32, cmdline string) *procutil.Process {
	return &procutil.Process{
		Pid:     pid,
		Cmdline: strings.Split(cmdline, " "),
		Stats: &procutil.Stats{
			CPUTime:     &procutil.CPUTimesStat{},
			MemInfo:     &procutil.MemoryInfoStat{},
			MemInfoEx:   &procutil.MemoryInfoExStat{},
			IOStat:      &procutil.IOCountersStat{},
			CtxSwitches: &procutil.NumCtxSwitchesStat{},
		},
	}
}

//nolint:deadcode,unused
// procMsgsVerification takes raw containers and processes and make sure the chunked messages have all data, and each chunk has the correct grouping
func procMsgsVerification(t *testing.T, msgs []model.MessageBody, rawContainers []*containers.Container, rawProcesses []*procutil.Process, maxSize int, cfg *config.AgentConfig) {
	actualProcs := 0
	for _, msg := range msgs {
		payload := msg.(*model.CollectorProc)

		if len(payload.Containers) > 0 {
			// assume no blacklist involved
			assert.Equal(t, len(rawContainers), len(payload.Containers))

			procsByPid := make(map[int32]struct{}, len(payload.Processes))
			for _, p := range payload.Processes {
				procsByPid[p.Pid] = struct{}{}
			}

			// make sure all containerized processes are in the payload
			containeredProcs := 0
			for _, ctr := range rawContainers {
				for _, pid := range ctr.Pids {
					assert.Contains(t, procsByPid, pid)
					containeredProcs++
				}
			}
			assert.Equal(t, len(payload.Processes), containeredProcs)

			actualProcs += containeredProcs
		} else {
			assert.True(t, len(payload.Processes) <= maxSize)
			actualProcs += len(payload.Processes)
		}
		assert.Equal(t, cfg.ContainerHostType, payload.ContainerHostType)
	}
	assert.Equal(t, len(rawProcesses), actualProcs)
}

func TestProcessChunking(t *testing.T) {
	p := []*procutil.Process{
		makeProcess(1, "git clone google.com"),
		makeProcess(2, "mine-bitcoins -all -x"),
		makeProcess(3, "datadog-process-agent -ddconfig datadog.conf"),
		makeProcess(4, "foo -bar -bim"),
	}
	containers := []*containers.Container{}
	lastRun := time.Now().Add(-5 * time.Second)
	syst1, syst2 := cpu.TimesStat{}, cpu.TimesStat{}
	cfg := config.NewDefaultAgentConfig(false)

	for i, tc := range []struct {
		cur, last          []*procutil.Process
		maxSize            int
		blacklist          []string
		expectedProcTotal  int
		expectedProcChunks int
		// stats might have different numbers than full Process because stats don't collect command line,
		// thus doesn't have the ability to filter by blacklist. This is not a real problem as when process-agent runs,
		// RTProcess will rely on the PIDs that ProcessCheck collected, which already skipped blacklist processes
		expectedStatTotal  int
		expectedStatChunks int
	}{
		{
			cur:                []*procutil.Process{p[0], p[1], p[2]},
			last:               []*procutil.Process{p[0], p[1], p[2]},
			maxSize:            1,
			blacklist:          []string{},
			expectedProcTotal:  3,
			expectedProcChunks: 3,
			expectedStatTotal:  3,
			expectedStatChunks: 3,
		},
		{
			cur:                []*procutil.Process{p[0], p[1], p[2]},
			last:               []*procutil.Process{p[0], p[2]},
			maxSize:            1,
			blacklist:          []string{},
			expectedProcTotal:  2,
			expectedProcChunks: 2,
			expectedStatTotal:  2,
			expectedStatChunks: 2,
		},
		{
			cur:                []*procutil.Process{p[0], p[1], p[2], p[3]},
			last:               []*procutil.Process{p[0], p[1], p[2], p[3]},
			maxSize:            10,
			blacklist:          []string{"git", "datadog"},
			expectedProcTotal:  2,
			expectedProcChunks: 1,
			expectedStatTotal:  4,
			expectedStatChunks: 1,
		},
		{
			cur:                []*procutil.Process{p[0], p[1], p[2], p[3]},
			last:               []*procutil.Process{p[0], p[1], p[2], p[3]},
			maxSize:            10,
			blacklist:          []string{"git", "datadog", "foo", "mine"},
			expectedProcTotal:  0,
			expectedProcChunks: 0,
			expectedStatTotal:  4,
			expectedStatChunks: 1,
		},
	} {
		bl := make([]*regexp.Regexp, 0, len(tc.blacklist))
		for _, s := range tc.blacklist {
			bl = append(bl, regexp.MustCompile(s))
		}
		cfg.Blacklist = bl
		cfg.MaxPerMessage = tc.maxSize

		cur := make(map[int32]*procutil.Process)
		for _, c := range tc.cur {
			cur[c.Pid] = c
		}
		last := make(map[int32]*procutil.Process)
		for _, c := range tc.last {
			last[c.Pid] = c
		}
		curStats := make(map[int32]*procutil.Stats)
		for _, c := range tc.cur {
			curStats[c.Pid] = c.Stats
		}
		lastStats := make(map[int32]*procutil.Stats)
		for _, c := range tc.last {
			lastStats[c.Pid] = c.Stats
		}
		networks := make(map[int32][]*model.Connection)

		procs := fmtProcesses(cfg, cur, last, containersByPid(containers), syst2, syst1, lastRun, networks)
		// only deal with non-container processes
		chunked := chunkProcesses(procs[emptyCtrID], cfg.MaxPerMessage)
		assert.Len(t, chunked, tc.expectedProcChunks, "len %d", i)
		total := 0
		for _, c := range chunked {
			total += len(c)
		}
		assert.Equal(t, tc.expectedProcTotal, total, "total test %d", i)

		chunkedStat := fmtProcessStats(cfg, curStats, lastStats, containers, syst2, syst1, lastRun, networks)
		assert.Len(t, chunkedStat, tc.expectedStatChunks, "len stat %d", i)
		total = 0
		for _, c := range chunkedStat {
			total += len(c)
		}
		assert.Equal(t, tc.expectedStatTotal, total, "total stat test %d", i)
	}
}

func TestPercentCalculation(t *testing.T) {
	// Capping at NUM CPU * 100 if we get odd values for delta-{Proc,Time}
	assert.True(t, floatEquals(calculatePct(100, 50, 1), 100))

	// Zero deltaTime case
	assert.True(t, floatEquals(calculatePct(100, 0, 8), 0.0))

	assert.True(t, floatEquals(calculatePct(0, 8.08, 8), 0.0))
	if runtime.GOOS != "windows" {
		assert.True(t, floatEquals(calculatePct(100, 200, 2), 100))
		assert.True(t, floatEquals(calculatePct(0.04, 8.08, 8), 3.960396))
		assert.True(t, floatEquals(calculatePct(1.09, 8.08, 8), 107.920792))
	}
}

func TestRateCalculation(t *testing.T) {
	now := time.Now()
	prev := now.Add(-1 * time.Second)
	var empty time.Time
	assert.True(t, floatEquals(calculateRate(5, 1, prev), 4))
	assert.True(t, floatEquals(calculateRate(5, 1, prev.Add(-2*time.Second)), float32(1.33333333)))
	assert.True(t, floatEquals(calculateRate(5, 1, now), 0))
	assert.True(t, floatEquals(calculateRate(5, 0, prev), 0))
	assert.True(t, floatEquals(calculateRate(5, 1, empty), 0))

	// Underflow on cur - prev
	assert.True(t, floatEquals(calculateRate(0, 1, prev), 0))
}

func TestFormatIO(t *testing.T) {
	fp := &procutil.Stats{
		IOStat: &procutil.IOCountersStat{
			ReadCount:  6,
			WriteCount: 8,
			ReadBytes:  10,
			WriteBytes: 12,
		},
	}

	last := &procutil.IOCountersStat{
		ReadCount:  1,
		WriteCount: 2,
		ReadBytes:  3,
		WriteBytes: 4,
	}

	// fp.IOStat is nil
	assert.NotNil(t, formatIO(&procutil.Stats{}, last, time.Now().Add(-2*time.Second)))

	// IOStats have 0 values
	result := formatIO(&procutil.Stats{IOStat: &procutil.IOCountersStat{}}, last, time.Now().Add(-2*time.Second))
	assert.Equal(t, float32(0), result.ReadRate)
	assert.Equal(t, float32(0), result.WriteRate)
	assert.Equal(t, float32(0), result.ReadBytesRate)
	assert.Equal(t, float32(0), result.WriteBytesRate)

	// Elapsed time < 1s
	assert.NotNil(t, formatIO(fp, last, time.Now()))

	// IOStats have permission problem
	result = formatIO(&procutil.Stats{IOStat: &procutil.IOCountersStat{
		ReadCount:  -1,
		WriteCount: -1,
		ReadBytes:  -1,
		WriteBytes: -1,
	}}, last, time.Now().Add(-1*time.Second))
	assert.Equal(t, float32(-1), result.ReadRate)
	assert.Equal(t, float32(-1), result.WriteRate)
	assert.Equal(t, float32(-1), result.ReadBytesRate)
	assert.Equal(t, float32(-1), result.WriteBytesRate)

	result = formatIO(fp, last, time.Now().Add(-1*time.Second))
	require.NotNil(t, result)
	assert.Equal(t, float32(5), result.ReadRate)
	assert.Equal(t, float32(6), result.WriteRate)
	assert.Equal(t, float32(7), result.ReadBytesRate)
	assert.Equal(t, float32(8), result.WriteBytesRate)

}

func TestFormatNetworks(t *testing.T) {
	for _, tc := range []struct {
		connsByPID map[int32][]*model.Connection
		interval   int
		pid        int32
		expected   *model.ProcessNetworks
	}{
		{
			connsByPID: map[int32][]*model.Connection{
				1: yieldConnections(10),
			},
			interval: 2,
			pid:      1,
			expected: &model.ProcessNetworks{ConnectionRate: 5, BytesRate: 150},
		},
		{
			connsByPID: map[int32][]*model.Connection{
				1: yieldConnections(10),
			},
			interval: 10,
			pid:      1,
			expected: &model.ProcessNetworks{ConnectionRate: 1, BytesRate: 30},
		},
		{
			connsByPID: map[int32][]*model.Connection{
				1: yieldConnections(10),
			},
			interval: 20,
			pid:      1,
			expected: &model.ProcessNetworks{ConnectionRate: 0.5, BytesRate: 15},
		},
		{
			connsByPID: nil,
			interval:   20,
			pid:        1,
			expected:   &model.ProcessNetworks{ConnectionRate: 0, BytesRate: 0},
		},
		{
			connsByPID: map[int32][]*model.Connection{
				1: yieldConnections(10),
			},
			interval: 10,
			pid:      2,
			expected: &model.ProcessNetworks{ConnectionRate: 0, BytesRate: 0},
		},
	} {
		result := formatNetworks(tc.connsByPID[tc.pid], tc.interval)
		assert.EqualValues(t, tc.expected, result)
	}
}

func floatEquals(a, b float32) bool {
	var e float32 = 0.00000001 // Difference less than some epsilon
	return a-b < e && b-a < e
}

func yieldConnections(count int) []*model.Connection {
	result := make([]*model.Connection, count)
	for i := 0; i < count; i++ {
		result[i] = &model.Connection{LastBytesReceived: 10, LastBytesSent: 20}
	}
	return result
}
