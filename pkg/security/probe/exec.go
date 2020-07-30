// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import "github.com/DataDog/datadog-agent/pkg/security/ebpf"

// execHookPoints holds the list of hookpoints to track processes execution
var execHookPoints = []*HookPoint{
	{
		Name: "sys_execve",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/" + getSyscallFnName("execve"),
		}},
		EventTypes: map[string]Capabilities{
			"*": {},
		},
	},
	{
		Name: "sys_execveat",
		KProbes: []*ebpf.KProbe{{
			EntryFunc: "kprobe/" + getSyscallFnName("execveat"),
		}},
		EventTypes: map[string]Capabilities{
			"*": {},
		},
		Optional: true,
	},
	{
		Name: "do_fork",
		KProbes: []*ebpf.KProbe{{
			ExitFunc: "kretprobe/_do_fork",
		}, {
			ExitFunc: "kretprobe/do_fork",
		}},
		EventTypes: map[string]Capabilities{
			"*": {},
		},
	},
	{
		Name: "do_exit",
		KProbes: []*ebpf.KProbe{{
			ExitFunc: "kprobe/do_exit",
		}},
		EventTypes: map[string]Capabilities{
			"*": {},
		},
	},
}
