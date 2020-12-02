// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probes

import (
	"github.com/DataDog/ebpf/manager"
)

// execProbes holds the list of probes used to track processes execution
var execProbes = []*manager.Probe{
	{
		UID:     SecurityAgentUID,
		Section: "tracepoint/sched/sched_process_fork",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/do_exit",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/cgroup_procs_write",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/cgroup1_procs_write",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/cgroup_tasks_write",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/cgroup1_tasks_write",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/exit_itimers",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/security_bprm_committed_creds",
	},
}

func getExecProbes() []*manager.Probe {
	execProbes = append(execProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "execve",
	}, Entry)...)
	execProbes = append(execProbes, ExpandSyscallProbes(&manager.Probe{
		UID:             SecurityAgentUID,
		SyscallFuncName: "execveat",
	}, Entry)...)

	return execProbes
}
