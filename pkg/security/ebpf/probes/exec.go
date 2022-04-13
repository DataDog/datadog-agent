// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probes

import manager "github.com/DataDog/ebpf-manager"

// execProbes holds the list of probes used to track processes execution
var execProbes = []*manager.Probe{
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "tracepoint/sched/sched_process_fork",
			EBPFFuncName: "sched_process_fork",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/do_exit",
			EBPFFuncName: "kprobe_do_exit",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/do_fork",
			EBPFFuncName: "kprobe_do_fork",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/_do_fork",
			EBPFFuncName: "kprobe__do_fork",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/kernel_clone",
			EBPFFuncName: "kprobe_kernel_clone",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/cgroup_procs_write",
			EBPFFuncName: "kprobe_cgroup_procs_write",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/cgroup1_procs_write",
			EBPFFuncName: "kprobe_cgroup1_procs_write",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/cgroup_tasks_write",
			EBPFFuncName: "kprobe_cgroup_tasks_write",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/cgroup1_tasks_write",
			EBPFFuncName: "kprobe_cgroup1_tasks_write",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/exit_itimers",
			EBPFFuncName: "kprobe_exit_itimers",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/prepare_binprm",
			EBPFFuncName: "kprobe_prepare_binprm",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/bprm_execve",
			EBPFFuncName: "kprobe_bprm_execve",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/security_bprm_check",
			EBPFFuncName: "kprobe_security_bprm_check",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/security_bprm_committed_creds",
			EBPFFuncName: "kprobe_security_bprm_committed_creds",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/commit_creds",
			EBPFFuncName: "kprobe_commit_creds",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kretprobe/__task_pid_nr_ns",
			EBPFFuncName: "kretprobe__task_pid_nr_ns",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kretprobe/alloc_pid",
			EBPFFuncName: "kretprobe_alloc_pid",
		},
	},
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          SecurityAgentUID,
			EBPFSection:  "kprobe/switch_task_namespaces",
			EBPFFuncName: "kprobe_switch_task_namespaces",
		},
	},
}

func getExecProbes() []*manager.Probe {
	execProbes = append(execProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "execve",
	}, Entry)...)
	execProbes = append(execProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
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
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID: SecurityAgentUID,
			},
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
				EBPFSection:  "kprobe/parse_args_envs",
				EBPFFuncName: "kprobe_parse_args_envs",
			},
		}
		routes = append(routes, route)
	}

	return routes
}
