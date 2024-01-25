// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package config

import (
	"fmt"
	"path/filepath"

	ebpfkernel "github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// defaultSystemProbeAddress is the default unix socket path to be used for connecting to the system probe
	defaultSystemProbeAddress = "/opt/datadog-agent/run/sysprobe.sock"

	defaultConfigDir = "/etc/datadog-agent"
)

// ValidateSocketAddress validates that the sysprobe socket config option is of the correct format.
func ValidateSocketAddress(sockPath string) error {
	if !filepath.IsAbs(sockPath) {
		return fmt.Errorf("socket path must be an absolute file path: `%s`", sockPath)
	}
	return nil
}

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
