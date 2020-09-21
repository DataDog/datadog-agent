// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import "github.com/DataDog/datadog-agent/pkg/security/ebpf"

// mountTables is the list of eBPF tables used by mount's kProbes
var mountTables = []string{
	"mount_id_offset",
}

func (mr *MountResolver) setMountIDOffset() error {
	if mr.probe.kernelVersion != 0 && mr.probe.kernelVersion <= kernel4_13 {
		offsetItem := ebpf.Uint32TableItem(268)
		table := mr.probe.Table("mount_id_offset")
		if err := table.Set(ebpf.ZeroUint32TableItem, offsetItem); err != nil {
			return err
		}
	}

	return nil
}

func (mr *MountResolver) Start() error {
	return mr.setMountIDOffset()
}
