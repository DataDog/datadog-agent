// +build linux_bpf

package tracer

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/stretchr/testify/require"
)

var (
	bpfToolPath       string
	currKernelVersion kernel.Version
	readVars          sync.Once
)

type bpftoolStats struct {
	RunTimeNS float64 `json:"run_time_ns"`
	RunCnt    float64 `json:"run_cnt"`

	XlatedBytes float64 `json:"bytes_xlated"`
	JITedBytes  float64 `json:"bytes_jited"`
}

func readGlobalVars() {
	var err error
	currKernelVersion, err = kernel.HostVersion()
	if err != nil {
		log.Fatalf("unable to read kernel version: %s\n", err)
	}
	bpfToolPath, err = exec.LookPath("bpftool")
	if err != nil {
		log.Fatalf("unable to find bpftool in PATH: %s\n", err)
	}
}

func setBPFStatsCollection(b *testing.B, enable bool) error {
	if currKernelVersion < kernel.VersionCode(5, 1, 0) {
		b.Skip("kernel version must be at least 5.1.0 for kernel-level eBPF stats collection")
		return nil
	}

	flag := 0
	if enable {
		flag = 1
	}
	cmd := exec.Command("sysctl", "-w", fmt.Sprintf("kernel.bpf_stats_enabled=%d", flag))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error setting sysctl kernel.bpf_stats_enabled: %s %w", out, err)
	}
	return nil
}

func collectBPFToolStats(progID uint32) (*bpftoolStats, error) {
	cmd := exec.Command(bpfToolPath, "--json", "prog", "show", "id", fmt.Sprintf("%d", progID))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("error collecting bpf stats: %s %w", out, err)
	}
	stats := bpftoolStats{}
	err = json.Unmarshal(out, &stats)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling json from bpftool: %s", err)
	}
	return &stats, nil
}

func getAllProbeStats(t *Tracer) (map[string]*bpftoolStats, error) {
	ids, err := t.getProbeProgramIDs()
	if err != nil {
		return nil, err
	}

	allStats := make(map[string]*bpftoolStats, len(ids))
	for name, id := range ids {
		stats, err := collectBPFToolStats(id)
		if err != nil {
			return nil, err
		}
		allStats[name] = stats
	}
	return allStats, nil
}

func RunEBPFBenchmark(b *testing.B, fn func(*testing.B)) {
	readVars.Do(readGlobalVars)

	err := setBPFStatsCollection(b, true)
	if err != nil {
		b.Fatal(err)
	}
	defer setBPFStatsCollection(b, false)

	// Enable BPF-based system probe
	t, err := NewTracer(testConfig())
	if err != nil {
		b.Fatal(err)
	}
	defer t.Stop()

	var prs map[string]*testing.BenchmarkResult
	b.Run("probes", func(b *testing.B) {
		baseline, err := getAllProbeStats(t)
		require.NoError(b, err)

		b.ResetTimer()
		fn(b)
		b.StopTimer()

		post, err := getAllProbeStats(t)
		require.NoError(b, err)

		// override outer variable here so we only report on the last run of results
		prs = make(map[string]*testing.BenchmarkResult, len(baseline))
		for probeName, base := range baseline {
			p := post[probeName]
			runTime := p.RunTimeNS - base.RunTimeNS
			runCount := p.RunCnt - base.RunCnt
			prs[probeName] = &testing.BenchmarkResult{
				N: int(runCount),
				T: time.Duration(runTime) * time.Nanosecond,
			}
		}
	})
	fmt.Print(prettyPrintEBPFResults(b.Name(), prs))
}

func prettyPrintEBPFResults(name string, probeResults map[string]*testing.BenchmarkResult) string {
	maxLen := 0
	var probeNames []string
	for pName := range probeResults {
		if len(pName) > maxLen {
			maxLen = len(pName)
		}
		probeNames = append(probeNames, pName)
	}
	sort.Strings(probeNames)
	buf := new(strings.Builder)
	for _, probeName := range probeNames {
		pr := probeResults[probeName]
		if pr.N == 0 {
			continue
		}
		fmt.Fprintf(buf, "%s/%-*s\t%s\n", name, maxLen, probeName, pr.String())
	}
	return buf.String()
}
