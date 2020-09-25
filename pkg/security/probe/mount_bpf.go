// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"github.com/cobaugh/osrelease"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (mr *MountResolver) setMountIDOffset() error {
	var suseKernel bool
	osrelease, err := osrelease.Read()
	if err == nil {
		suseKernel = (osrelease["ID"] == "sles") || (osrelease["ID"] == "opensuse-leap")
	}

	var offsetItem ebpf.Uint32MapItem
	if suseKernel {
		offsetItem = 292
	} else if mr.probe.kernelVersion != 0 && mr.probe.kernelVersion <= kernel4_13 {
		offsetItem = 268
	}

	if offsetItem != 0 {
		log.Debugf("Setting mount_id offset to %d", offsetItem)

		table := mr.probe.Map("mount_id_offset")
		if table == nil {
			return errors.New("map mount_id_offset not found")
		}
		return table.Put(ebpf.ZeroUint32MapItem, offsetItem)
	}

	return nil
}

func (mr *MountResolver) Start() error {
	return mr.setMountIDOffset()
}
