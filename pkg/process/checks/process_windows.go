// +build windows

package checks

import (
	"runtime"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/gopsutil/cpu"
	"github.com/DataDog/gopsutil/process"
)

func formatUser(fp *process.FilledProcess) *model.ProcessUser {
	return &model.ProcessUser{
		Name: fp.Username,
	}
}

func formatCPU(fp *process.FilledProcess, t2, t1, syst2, syst1 cpu.TimesStat) *model.CPUStat {
	numCPU := float64(runtime.NumCPU())
	deltaSys := float64(t2.Timestamp - t1.Timestamp)
	// under windows, utime & stime are number of 100-ns increments.  The elapsed time
	// is in nanoseconds.
	return &model.CPUStat{
		LastCpu:    t2.CPU,
		TotalPct:   calculatePct(((t2.User-t1.User)+(t2.System-t1.System))*100, deltaSys, numCPU),
		UserPct:    calculatePct((t2.User-t1.User)*100, deltaSys, numCPU),
		SystemPct:  calculatePct((t2.System-t1.System)*100, deltaSys, numCPU),
		NumThreads: fp.NumThreads,
		Cpus:       []*model.SingleCPUStat{},
		Nice:       fp.Nice,
		UserTime:   int64(t2.User),
		SystemTime: int64(t2.System),
	}
}

func calculatePct(deltaProc, deltaTime, numCPU float64) float32 {
	if deltaTime == 0 {
		return 0
	}

	// Calculates utilization split across all CPUs. A busy-loop process
	// on a 2-CPU-core system would be reported as 50% instead of 100%.
	overalPct := (deltaProc / deltaTime) * 100

	// In cases where we get values that don't make sense, clamp to (100% * number of CPUS)
	if overalPct > (numCPU * 100) {
		overalPct = numCPU * 100
	}
	return float32(overalPct)
}
