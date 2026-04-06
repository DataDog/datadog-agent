// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package tracer

import (
	"strconv"

	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/network/events"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const defaultBaseController = "memory"

func rescueEventWithProcfs(entry *events.Process) *events.Process {
	pid := strconv.FormatUint(uint64(entry.Pid), 10)
	procRoot := kernel.ProcFSRoot()

	// Try cgroup v2 first (controller ""), then fall back to v1 ("memory").
	cid, _ := cgroups.IdentiferFromCgroupReferences(procRoot, pid, "", cgroups.ContainerFilter)
	if cid == "" {
		cid, _ = cgroups.IdentiferFromCgroupReferences(procRoot, pid, defaultBaseController, cgroups.ContainerFilter)
	}
	if cid != "" {
		entry.ContainerID = intern.GetByString(cid)
		return entry
	}
	return nil
}
