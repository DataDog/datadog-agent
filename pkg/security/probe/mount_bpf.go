// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cobaugh/osrelease"
)

// mountTables is the list of eBPF tables used by mount's kProbes
var mountTables = []string{
	"mount_id_offset",
}

func (mr *MountResolver) setMountIDOffset() error {
	var suseKernel bool
	osrelease, err := osrelease.Read()
	if err == nil {
		suseKernel = (osrelease["ID"] == "sles") || (osrelease["ID"] == "opensuse-leap")
	}

	var offsetItem ebpf.Uint32TableItem
	if suseKernel {
		offsetItem = 292
	} else if mr.probe.kernelVersion != 0 && mr.probe.kernelVersion <= kernel4_13 {
		offsetItem = 268
	}

	if offsetItem != 0 {
		log.Debugf("Setting mount_id offset to %d", offsetItem)
		table := mr.probe.Table("mount_id_offset")
		return table.Set(ebpf.ZeroUint32TableItem, offsetItem)
	}

	return nil
}

func (mr *MountResolver) Start() error {
	return mr.setMountIDOffset()
}
