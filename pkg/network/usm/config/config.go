// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package config provides helpers for USM configuration
package config

import (
	"errors"
	"fmt"
	"runtime"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MinimumKernelVersion indicates the minimum kernel version required for HTTP monitoring
var MinimumKernelVersion kernel.Version
var ErrNotSupported = errors.New("Universal Service Monitoring (USM) is not supported")

func init() {
	MinimumKernelVersion = kernel.VersionCode(4, 14, 0)
}

func runningOnARM() bool {
	return strings.HasPrefix(runtime.GOARCH, "arm")
}

// TLSSupported returns true if HTTPs monitoring is supported on the current OS.
// We only support ARM with kernel >= 5.5.0 and with runtime compilation enabled
func TLSSupported(c *config.Config) bool {
	if c.EnableEbpfless {
		return false
	}

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

// IsUSMSupported returns `true` if USM is supported on this
// platform.
func IsUSMSupported(cfg *config.Config) error {
	// TODO: remove this once USM is supported on ebpf-less
	if cfg.EnableEbpfless {
		return fmt.Errorf("%w: eBPF-less is not supported", ErrNotSupported)
	}

	kversion, err := kernel.HostVersion()
	if err != nil {
		return fmt.Errorf("could not determine the current kernel version: %w", err)
	}

	if kversion < MinimumKernelVersion {
		return fmt.Errorf("%w: a Linux kernel version of %s or higher is required; we detected %s", ErrNotSupported, MinimumKernelVersion, kversion)
	}

	return nil
}

// IsUSMSupportedAndEnabled returns true if USM is supported and enabled
func IsUSMSupportedAndEnabled(config *config.Config) bool {
	// http.Supported is misleading, it should be named usm.Supported.
	return config.ServiceMonitoringEnabled && IsUSMSupported(config) == nil
}

// NeedProcessMonitor returns true if the process monitor is needed for the given configuration
func NeedProcessMonitor(config *config.Config) bool {
	return config.EnableNativeTLSMonitoring || config.EnableGoTLSSupport || config.EnableJavaTLSSupport || config.EnableIstioMonitoring || config.EnableNodeJSMonitoring
}
