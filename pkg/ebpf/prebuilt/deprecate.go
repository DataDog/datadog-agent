// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

// Package prebuilt implements prebuilt specific eBPF functionality
package prebuilt

import (
	ebpfutil "github.com/DataDog/datadog-agent/pkg/util/ebpf"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// DeprecatedKernelVersionRhel is the kernel version
	// where prebuilt eBPF is deprecated on RHEL based kernels
	DeprecatedKernelVersionRhel = ebpfutil.VersionCode(5, 14, 0)
	// DeprecatedKernelVersion is the kernel version
	// where prebuilt eBPF is deprecated on non-RHEL based kernels
	DeprecatedKernelVersion = ebpfutil.VersionCode(6, 0, 0)
)

// IsDeprecated returns true if prebuilt ebpf is deprecated
// on this host
func IsDeprecated() bool {
	// has to be kernel 6+ or RHEL 9+ (kernel 5.14+)
	family, err := ebpfutil.Family()
	if err != nil {
		log.Warnf("could not determine OS family: %s", err)
		return false
	}

	// check kernel version
	kv, err := ebpfutil.HostVersion()
	if err != nil {
		log.Warnf("could not determine kernel version: %s", err)
		return false
	}

	if family == "rhel" {
		return kv >= DeprecatedKernelVersionRhel
	}

	return kv >= DeprecatedKernelVersion
}
