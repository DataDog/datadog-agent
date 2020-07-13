package probe

import (
	eprobe "github.com/DataDog/datadog-agent/pkg/ebpf/probe"
)

// ExecTables - eBPF tables used by open's kProbes
var ExecTables = []KTable{}

// ExecHookPoints - list of open's hooks
var ExecHookPoints = []*HookPoint{
	{
		KProbes: []*eprobe.KProbe{{
			Name:      "sys_execve",
			EntryFunc: "kprobe/" + getSyscallFnName("execve"),
		}},
		EventTypes: map[string]Capabilities{
			"*": Capabilities{},
		},
	},
	{
		KProbes: []*eprobe.KProbe{{
			Name:      "sys_execveat",
			EntryFunc: "kprobe/" + getSyscallFnName("execveat"),
		}},
		EventTypes: map[string]Capabilities{
			"*": Capabilities{},
		},
		Optional: true,
	},
	{
		KProbes: []*eprobe.KProbe{{
			Name:     "_do_fork",
			ExitFunc: "kretprobe/_do_fork",
		}, {
			Name:     "do_fork",
			ExitFunc: "kretprobe/do_fork",
		}},
		EventTypes: map[string]Capabilities{
			"*": Capabilities{},
		},
	},
	{
		KProbes: []*eprobe.KProbe{{
			Name:     "do_exit",
			ExitFunc: "kprobe/do_exit",
		}},
		EventTypes: map[string]Capabilities{
			"*": Capabilities{},
		},
	},
}
