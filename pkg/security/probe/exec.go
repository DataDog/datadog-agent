// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

// execHookPoints holds the list of hookpoints to track processes execution
var execHookPoints = []*HookPoint{
	{
		Name: "sys_execve",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/" + getSyscallFnName("execve"),
		}},
		EventTypes: []eval.EventType{"*"},
	},
	{
		Name: "sys_execveat",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/" + getSyscallFnName("execveat"),
		}},
		EventTypes: []eval.EventType{"*"},
		Optional:   true,
	},
	{
		Name:       "sched_process_fork",
		Tracepoint: "tracepoint/sched/sched_process_fork",
		EventTypes: []eval.EventType{"*"},
	},
	{
		Name: "do_exit",
		KProbes: []*ebpf.KProbe{{
			ExitFunc: "kprobe/do_exit",
		}},
		EventTypes: []eval.EventType{"*"},
	},
	{
		Name: "cgroup_procs_write",
		KProbes: []*ebpf.KProbe{{
			ExitFunc: "kprobe/cgroup_procs_write",
		}},
		EventTypes: []eval.EventType{"*"},
		Optional:   true,
	},
	{
		Name: "cgroup1_procs_write",
		KProbes: []*ebpf.KProbe{{
			ExitFunc: "kprobe/cgroup1_procs_write",
		}},
		EventTypes: []eval.EventType{"*"},
		Optional:   true,
	},
	{
		Name: "cgroup_tasks_write",
		KProbes: []*ebpf.KProbe{{
			ExitFunc: "kprobe/cgroup_tasks_write",
		}},
		EventTypes: []eval.EventType{"*"},
		Optional:   true,
	},
	{
		Name: "cgroup1_tasks_write",
		KProbes: []*ebpf.KProbe{{
			ExitFunc: "kprobe/cgroup1_tasks_write",
		}},
		EventTypes: []eval.EventType{"*"},
		Optional:   true,
	},
}
