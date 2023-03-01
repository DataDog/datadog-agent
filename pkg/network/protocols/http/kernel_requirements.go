// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package http

import (
	"path/filepath"
	"runtime"
	"strings"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MinimumKernelVersion indicates the minimum kernel version required for HTTP monitoring
var MinimumKernelVersion kernel.Version

func init() {
	MinimumKernelVersion = kernel.VersionCode(4, 14, 0)
}

// ErrNotSupported indicates that the current host doesn't fullfil the
// requirements for HTTP monitoring
type ErrNotSupported struct {
	error
}

func (e *ErrNotSupported) Unwrap() error {
	return e.error
}

func runningOnARM() bool {
	return strings.HasPrefix(runtime.GOARCH, "arm")
}

// We only support ARM with kernel >= 5.5.0 and with runtime compilation enabled
func HTTPSSupported(c *config.Config) bool {
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

func sysOpenAt2Supported(c *config.Config) bool {
	ksymPath := filepath.Join(c.ProcRoot, "kallsyms")
	missing, err := ddebpf.VerifyKernelFuncs(ksymPath, []string{doSysOpenAt2.section})
	if err == nil && len(missing) == 0 {
		return true
	}
	kversion, err := kernel.HostVersion()
	if err != nil {
		log.Error("could not determine the current kernel version. fallback to do_sys_open")
		return false
	}

	return kversion >= kernel.VersionCode(5, 6, 0)
}
