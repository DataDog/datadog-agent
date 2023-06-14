// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package checks

import (
	"math"
	"runtime"

	"github.com/DataDog/gopsutil/cpu"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

var (
	// overridden in tests
	numCPU = runtime.NumCPU
)

func formatUser(fp *procutil.Process, _ *LookupIdProbe) *model.ProcessUser {
	return &model.ProcessUser{
		Name: fp.Username,
	}
}

func formatCPUTimes(fp *procutil.Stats, t2, t1 *procutil.CPUTimesStat, _, _ cpu.TimesStat) *model.CPUStat {
	numCPU := float64(numCPU())
	deltaSys := float64(t2.Timestamp - t1.Timestamp)
	// under windows, utime & stime are number of 100-ns increments.  The elapsed time
	// is in nanoseconds.
	return &model.CPUStat{
		LastCpu:    "cpu",
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

	// Clamp to 0 below if we get a negative value
	// deltaTime is approximated using the system time on Windows, and can turn negative when NTP clock synchronization occurs or the system time is manually reset
	// Avoid reporting negative CPU percentages when this occurs
	overalPct = math.Max(overalPct, 0.0)
	return float32(overalPct)
}
