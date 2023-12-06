// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MinimumKernelVersion indicates the minimum kernel version required for HTTP monitoring
var MinimumKernelVersion kernel.Version

func init() {
	MinimumKernelVersion = kernel.VersionCode(4, 14, 0)
}

func runningOnARM() bool {
	return strings.HasPrefix(runtime.GOARCH, "arm")
}

// TLSSupported returns true if HTTPs monitoring is supported on the current OS.
// We only support ARM with kernel >= 5.5.0 and with runtime compilation enabled
func TLSSupported(c *config.Config) bool {
	kversion, err := kernel.HostVersion()
	if err != nil {
		log.Warn("could not determine the current kernel version. https monitoring disabled.")
		return false
	}

	if runningOnARM() {
		return kversion >= kernel.VersionCode(5, 5, 0) && (c.EnableRuntimeCompiler || c.EnableCORE)
	}

	return kversion >= MinimumKernelVersion
}

// Supported We only support http with kernel >= 4.14.0.
func Supported() bool {
	kversion, err := kernel.HostVersion()
	if err != nil {
		log.Warn("could not determine the current kernel version. http2 monitoring disabled.")
		return false
	}

	return kversion >= MinimumKernelVersion
}
