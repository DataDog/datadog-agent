// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
)

func getCheckHelperCallInputType(kernelVersion *kernel.Version) uint64 {
	input := uint64(1)

	switch {
	case kernelVersion.Code != 0 && kernelVersion.Code >= kernel.Kernel5_13:
		input = uint64(2)
	}

	return input
}
