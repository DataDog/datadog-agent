package probes

import (
	"github.com/DataDog/datadog-agent/pkg/security/probe/uprobe"
	manager "github.com/DataDog/ebpf-manager"
)

var uprobeProbes = []*manager.Probe{
	{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID:          uprobe.UprobeUID,
			EBPFSection:  uprobe.UprobeSection,
			EBPFFuncName: uprobe.UprobeFuncName,
		},
		KeepProgramSpec: true,
	},
}

func getUprobeProbes() []*manager.Probe {
	uprobeProbes = append(uprobeProbes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "unshare",
	}, EntryAndExit)...)

	return uprobeProbes
}
