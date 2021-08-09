// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build linux

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
		Section: "kprobe/do_fork",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/_do_fork",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/kernel_clone",
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
		Section: "kprobe/prepare_binprm",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/bprm_execve",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/security_bprm_check",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/security_bprm_committed_creds",
	},
	{
		UID:     SecurityAgentUID,
		Section: "kprobe/commit_creds",
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

	for _, name := range []string{
		"setuid",
		"setuid16",
		"setgid",
		"setgid16",
		"seteuid",
		"seteuid16",
		"setegid",
		"setegid16",
		"setfsuid",
		"setfsuid16",
		"setfsgid",
		"setfsgid16",
		"setreuid",
		"setreuid16",
		"setregid",
		"setregid16",
		"setresuid",
		"setresuid16",
		"setresgid",
		"setresgid16",
		"capset",
	} {
		execProbes = append(execProbes, ExpandSyscallProbes(&manager.Probe{
			UID:             SecurityAgentUID,
			SyscallFuncName: name,
		}, EntryAndExit)...)
	}

	return execProbes
}

func getExecTailCallRoutes() []manager.TailCallRoute {
	var routes []manager.TailCallRoute

	for i := uint32(0); i != 10; i++ {
		route := manager.TailCallRoute{
			ProgArrayName: "args_envs_progs",
			Key:           i,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				Section: "kprobe/parse_args_envs",
			},
		}
		routes = append(routes, route)
	}

	return routes
}
