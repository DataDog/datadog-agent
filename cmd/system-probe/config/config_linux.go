// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package config

import (
	ebpfkernel "github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ProcessEventDataStreamSupported returns true if process event data stream is supported
func ProcessEventDataStreamSupported() bool {
	kernelVersion, err := ebpfkernel.NewKernelVersion()
	if err != nil {
		log.Errorf("unable to detect the kernel version: %s", err)
		return false
	}
	// This is different from the check VerifyOSVersion in probe_ebpf.go
	// We ran tests on 4.14 and realize we are able to support it for that kernel version
	// Since we have a large customer base with 4.14, we decided to enable for them
	if !kernelVersion.IsRH7Kernel() && kernelVersion.Code < ebpfkernel.Kernel4_14 {
		return false
	}

	return true
}
