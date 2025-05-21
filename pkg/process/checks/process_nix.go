// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package checks

import (
	"fmt"
	"math"
	"os/user"
	"slices"
	"strconv"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/shirou/gopsutil/v4/cpu"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/fargate"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

// overridden in tests
var hostCPUCount = system.HostCPUCount

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
	// nolint: staticcheck
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
	overalPct := min((deltaProc/deltaTime)*100, 100)

	// In order to emulate top we multiply utilization by # of CPUs so a busy loop would be 100%.
	pct := overalPct * numCPU

	// Clamp to 0 below if we get a negative value
	// CPU time counters in /proc/ used to determine process execution time may potentially be decremented, leading to a negative deltaProc
	// Avoid reporting negative CPU percentages when this occurs
	pct = math.Max(pct, 0.0)
	return float32(pct)
}

// warnECSFargateMisconfig pidMode is currently not supported on fargate windows, see docs: https://docs.aws.amazon.com/AmazonECS/latest/developerguide/task_definition_parameters.html#task_definition_pidmode
func warnECSFargateMisconfig(containers []*model.Container) {
	log.Warnf("is fargate instance: %t", fargate.IsFargateInstance())
	if fargate.IsFargateInstance() && !isECSFargatePidModeSetToTask(containers) {
		log.Warn(`Process collection may be misconfigured. Please ensure your task definition has "pidMode":"task" set. See https://docs.datadoghq.com/integrations/ecs_fargate/#process-collection for more information.`)
	}
}

// isECSFargatePidModeSetToTask checks if pidMode is set to task in task definition to allow for process collection and assumes the method is called in a fargate environment
func isECSFargatePidModeSetToTask(containers []*model.Container) bool {
	// aws-fargate-pause container only exists when "pidMode" is set to "task" on ecs fargate
	ecsContainerNameTag := fmt.Sprintf("%s:%s", tags.EcsContainerName, "aws-fargate-pause")
	log.Warnf("containers len %d, array: %+v\n", len(containers), containers)
	for _, c := range containers {
		log.Warnf("container being checked %+v\n", c)
		log.Warnf("container tags %+v\n", c.Tags)
		// container fields are not yet populated with information from tags at this point, so we need to check the tags
		if slices.Contains(c.Tags, ecsContainerNameTag) {
			return true
		}
	}
	return false
}
