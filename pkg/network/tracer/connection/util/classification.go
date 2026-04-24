// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package util

import (
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// The kernel has to be newer than 4.11.0 since we are using bpf_skb_load_bytes (4.5.0+), which
	// was added to socket filters in 4.11.0:
	// - 2492d3b867043f6880708d095a7a5d65debcfc32
	classificationMinimumKernel = kernel.VersionCode(4, 11, 0)

	rhel9KernelVersion = kernel.VersionCode(5, 14, 0)
)

// ClassificationSupported returns true if the current kernel version supports the classification feature.
// The kernel has to be newer than 4.11.0 since we are using bpf_skb_load_bytes (4.5.0+) method which was added to
// socket filters in 4.11.0, and a tracepoint (4.7.0+)
func ClassificationSupported(config *config.Config) bool {
	if !config.ProtocolClassificationEnabled {
		return false
	}
	if !config.CollectTCPv4Conns && !config.CollectTCPv6Conns {
		return false
	}
	currentKernelVersion, err := kernel.HostVersion()
	if err != nil {
		log.Warn("could not determine the current kernel version. classification monitoring disabled.")
		return false
	}

	if currentKernelVersion < classificationMinimumKernel {
		return false
	}

	// TODO: fix protocol classification is not supported on RHEL 9+
	family, err := kernel.Family()
	if err != nil {
		log.Warnf("could not determine OS family: %s", err)
		return false
	}

	if family == "rhel" && currentKernelVersion >= rhel9KernelVersion {
		log.Warn("protocol classification is currently not supported on RHEL 9+")
		return false
	}

	return true
}
