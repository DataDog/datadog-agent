// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package redis

import (
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MinimumKernelVersion indicates the minimum kernel version required for Redis monitoring
var MinimumKernelVersion kernel.Version

func init() {
	MinimumKernelVersion = kernel.VersionCode(5, 4, 0)
}

// Supported returns true if Redis monitoring is supported on the current kernel version.
// We only support Redis with kernel >= 5.4.0, as the kernel implementation exceeds
// the complexity limits on kernels prior to that.
func Supported() bool {
	kversion, err := kernel.HostVersion()
	if err != nil {
		log.Warn("could not determine the current kernel version. Redis monitoring disabled.")
		return false
	}

	return kversion >= MinimumKernelVersion
}
