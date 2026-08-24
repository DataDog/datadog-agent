// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import manager "github.com/DataDog/ebpf-manager"

// nsInstallHooks are the proc_ns_operations install callbacks. Hooking them is how the namespace
// type actually joined is recovered, since a setns nstype of 0 leaves the kernel to resolve the
// type from the file descriptor. They are best effort: timens_install requires CONFIG_TIME_NS and
// cgroupns_install requires CONFIG_CGROUPS, so neither is guaranteed to be present.
var nsInstallHooks = []string{
	"hook_mntns_install",
	"hook_netns_install",
	"hook_pidns_install",
	"hook_userns_install",
	"hook_utsns_install",
	"hook_ipcns_install",
	"hook_cgroupns_install",
	"hook_timens_install",
}

// nsInstallHookSelectors returns the install hooks as probe selectors, for the setns event type.
func nsInstallHookSelectors() []manager.ProbesSelector {
	selectors := make([]manager.ProbesSelector, 0, len(nsInstallHooks))
	for _, hook := range nsInstallHooks {
		selectors = append(selectors, hookFunc(hook))
	}
	return selectors
}

func getSetNSProbes(fentry bool) []*manager.Probe {
	var probes []*manager.Probe

	probes = append(probes, ExpandSyscallProbes(&manager.Probe{
		ProbeIdentificationPair: manager.ProbeIdentificationPair{
			UID: SecurityAgentUID,
		},
		SyscallFuncName: "setns",
	}, fentry, EntryAndExit)...)

	for _, hook := range nsInstallHooks {
		probes = append(probes, &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: hook,
			},
		})
	}

	return probes
}
