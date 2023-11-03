// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import (
	manager "github.com/DataDog/ebpf-manager"
)

func getExecProbes(fentry bool) []*manager.Probe {
	var execProbes = []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_bprm_execve",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_security_bprm_check",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "sched_process_fork",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_do_exit",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_kernel_clone",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_kernel_thread",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_user_mode_thread",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_cgroup_procs_write",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_cgroup1_procs_write",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_exit_itimers",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_setup_new_exec_interp",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID + "_a",
				EBPFFuncName: "hook_setup_new_exec_args_envs",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_setup_arg_pages",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_mprotect_fixup",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_commit_creds",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_switch_task_namespaces",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_do_coredump",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_prepare_binprm",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_do_fork",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook__do_fork",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_cgroup_tasks_write",
			},
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_cgroup1_tasks_write",
			},
		},
	}

	execProbes = append(execProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "execve",
	}, fentry, EntryAndExit)...)
	execProbes = append(execProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "execveat",
	}, fentry, EntryAndExit)...)

	for _, name := range []string{
		"setuid",
		"setuid16",
		"setgid",
		"setgid16",
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
		flags := EntryAndExit

		execProbes = append(execProbes, ExpandSyscallProbes(&manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID: SecurityAgentUID,
			},
			SyscallFuncName: name,
		}, fentry, flags)...)
	}

	return execProbes
}

func getExecTailCallRoutes() []manager.TailCallRoute {
	return []manager.TailCallRoute{
		{
			ProgArrayName: "args_envs_progs",
			Key:           ExecGetEnvsOffsetKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_get_envs_offset",
			},
		},
		{
			ProgArrayName: "args_envs_progs",
			Key:           ExecParseArgsEnvsSplitKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_parse_args_envs_split",
			},
		},
		{
			ProgArrayName: "args_envs_progs",
			Key:           ExecParseArgsEnvsKey,
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: "tail_call_target_parse_args_envs",
			},
		},
	}
}
