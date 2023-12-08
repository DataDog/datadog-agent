// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http2

import (
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MinimumKernelVersion indicates the minimum kernel version required for HTTP2 monitoring
var MinimumKernelVersion kernel.Version

func init() {
	MinimumKernelVersion = kernel.VersionCode(5, 4, 0)
}

// Supported We only support http2 with kernel >= 5.4.0. The kernel implementation exceeds the instruction limit
// on kernels prior to 5.2.0. In 5.2.0 the instruction limit was bumped to 1M instead of 4K. Furthermore, we're sending
// paths from the kernel to the userspace using bpf_perf_event_output directly from a socket filter. This was added in
// 5.4.0.
func Supported() bool {
	kversion, err := kernel.HostVersion()
	if err != nil {
		log.Warn("could not determine the current kernel version. http2 monitoring disabled.")
		return false
	}

	return kversion >= MinimumKernelVersion
}
