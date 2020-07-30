package probe

import "github.com/DataDog/datadog-agent/pkg/security/ebpf"

// ExecTables - eBPF tables used by open's kProbes
var ExecTables = []KTable{}

// ExecHookPoints - list of open's hooks
var ExecHookPoints = []*HookPoint{
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
