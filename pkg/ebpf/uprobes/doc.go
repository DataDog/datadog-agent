// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

/*
Package uprobes contains methods to help handling the attachment of uprobes to
userspace programs

The main type for this package is the UprobeAttacher type, created with
NewUprobeAttacher. The main configuration it requires is a list of rules that
define how to match the possible targets (shared libraries and/or executables)
and which probes to attach to them. Example usage:

	connectProbeID := manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_connect"}
	mainProbeID := manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__main"}

	mgr := manager.Manager{}

	attacherCfg := AttacherConfig{
		Rules: []*AttachRule{
			{
				LibraryNameRegex: regexp.MustCompile(`libssl.so`),
				Targets:          AttachToSharedLibraries,
				ProbesSelector: []manager.ProbesSelector{
					&manager.ProbeSelector{ProbeIdentificationPair: connectProbeID},
				},
			},
			{
				Targets: AttachToExecutable,
				ProbesSelector: []manager.ProbesSelector{
					&manager.ProbeSelector{ProbeIdentificationPair: mainProbeID},
				},
			},
		},
		ExcludeTargets: ExcludeInternal | ExcludeSelf,
		EbpfConfig:     ebpfCfg,
	}

	ua, err := NewUprobeAttacher("test", attacherCfg, &mgr, callback, &NativeBinaryInspector{}, processMonitor)
	ua.Start()

Once started, the attacher monitors new processes and `open` calls for new
shared libraries. For the first task it uses the monitor provided as argument,
and for the second it uses the shared-libraries program in
pkg/network/usm/sharedlibraries.

# Notes and things to take into account

  - When adding new probes, be sure to add the corresponding code to
    match the libraries in
    pkg/network/ebpf/c/shared-libraries/probes.h:do_sys_open_helper_exit, as an
    initial filtering is performed there.

  - If multiple rules match a binary file, and we fail to attach the required probes for one of them,
    the whole attach operation will be considered as failed, and the probes will be detached. If you want
    to control which probes are optional and which are mandatory, you can use the manager.AllOf/manager.BestEffort
    selectors in a single rule.

  - The recommended way to add a process monitor is using the event stream, creating a event consumer. There is also
    monitor.GetProcessMonitor, but that monitor is intended for use in USM only and is not recommended for use in other
    parts of the codebase, as it will be deprecated once netlink is not needed anymore.
*/
package uprobes
