package probe

import (
	eprobe "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
)

// ExecTables - eBPF tables used by open's kProbes
var ExecTables = []KTable{}

// ExecKProbes - list of open's hooks
var ExecKProbes = []*KProbe{
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_execve",
			EntryFunc: "kprobe/" + getSyscallFnName("execve"),
		},
		EventTypes: map[string]Capabilities{
			"*": Capabilities{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:      "sys_execveat",
			EntryFunc: "kprobe/" + getSyscallFnName("execveat"),
		},
		EventTypes: map[string]Capabilities{
			"*": Capabilities{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:     "sys_fork",
			ExitFunc: "kretprobe/_do_fork",
		},
		EventTypes: map[string]Capabilities{
			"*": Capabilities{},
		},
	},
	{
		KProbe: &eprobe.KProbe{
			Name:     "sys_exit",
			ExitFunc: "kprobe/do_exit",
		},
		EventTypes: map[string]Capabilities{
			"*": Capabilities{},
		},
	},
}
