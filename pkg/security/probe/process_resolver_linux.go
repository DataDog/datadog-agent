// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	manager "github.com/DataDog/ebpf-manager"
)

// getCGroupWriteConstants returns the value of the constant used to determine how cgroups should be captured in kernel
// space
func getCGroupWriteConstants() manager.ConstantEditor {
	cgroupWriteConst := uint64(1)
	kv, err := kernel.NewKernelVersion()
	if err == nil {
		if kv.IsRH7Kernel() {
			cgroupWriteConst = 2
		}
	}

	return manager.ConstantEditor{
		Name:  "cgroup_write_type",
		Value: cgroupWriteConst,
	}
}
