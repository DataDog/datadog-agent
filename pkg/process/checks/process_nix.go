// +build !windows

package checks

import (
	"os/user"
	"runtime"
	"strconv"

	"github.com/DataDog/gopsutil/cpu"
	"github.com/DataDog/gopsutil/process"

	model "github.com/DataDog/agent-payload/process"
)

func formatUser(fp *process.FilledProcess) *model.ProcessUser {
	var username string
	var uid, gid int32
	if len(fp.Uids) > 0 {
		u, err := user.LookupId(strconv.Itoa(int(fp.Uids[0])))
		if err == nil {
			username = u.Username
		}
		uid = fp.Uids[0]
	}
	if len(fp.Gids) > 0 {
		gid = fp.Gids[0]
	}

	return &model.ProcessUser{
		Name: username,
		Uid:  uid,
		Gid:  gid,
	}
}

func formatCPU(fp *process.FilledProcess, t2, t1, syst2, syst1 cpu.TimesStat) *model.CPUStat {
	numCPU := float64(runtime.NumCPU())
	deltaSys := syst2.Total() - syst1.Total()
	return &model.CPUStat{
		LastCpu:    t2.CPU,
		TotalPct:   calculatePct((t2.User-t1.User)+(t2.System-t1.System), deltaSys, numCPU),
		UserPct:    calculatePct(t2.User-t1.User, deltaSys, numCPU),
		SystemPct:  calculatePct(t2.System-t1.System, deltaSys, numCPU),
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

	// Sometimes we get values that don't make sense, so we clamp to 100%
	if overalPct > 100 {
		overalPct = 100
	}

	// In order to emulate top we multiply utilization by # of CPUs so a busy loop would be 100%.
	return float32(overalPct * numCPU)
}
