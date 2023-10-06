// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package checks

import (
	"math"
	"os/user"
	"strconv"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/gopsutil/cpu"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

var (
	// overridden in tests
	hostCPUCount = system.HostCPUCount
)

func formatUser(fp *procutil.Process, uidProbe *LookupIdProbe) *model.ProcessUser {
	var username string
	var uid, gid int32
	if len(fp.Uids) > 0 {
		var (
			u   *user.User
			err error
		)
		if uidProbe == nil {
			// If the probe is nil, skip it and just call `LookupId` directly
			u, err = user.LookupId(strconv.Itoa(int(fp.Uids[0])))
		} else {
			u, err = uidProbe.LookupId(strconv.Itoa(int(fp.Uids[0])))
		}
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

func formatCPUTimes(fp *procutil.Stats, t2, t1 *procutil.CPUTimesStat, syst2, syst1 cpu.TimesStat) *model.CPUStat {
	numCPU := float64(hostCPUCount())
	deltaSys := syst2.Total() - syst1.Total()
	return &model.CPUStat{
		LastCpu:    "cpu",
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
	pct := overalPct * numCPU

	// Clamp to 0 below if we get a negative value
	// CPU time counters in /proc/ used to determine process execution time may potentially be decremented, leading to a negative deltaProc
	// Avoid reporting negative CPU percentages when this occurs
	pct = math.Max(pct, 0.0)
	return float32(pct)
}
