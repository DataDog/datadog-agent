// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
)

func getCheckHelperCallInputType(probe *Probe) uint64 {
	input := uint64(1)

	switch {
	case probe.kernelVersion.Code != 0 && probe.kernelVersion.Code >= kernel.Kernel5_13:
		input = uint64(2)
	}

	return input
}
