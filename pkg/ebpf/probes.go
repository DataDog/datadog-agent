// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpf

import (
	"fmt"
	"runtime"
	"strings"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var indirectSyscallPrefixes = map[string]string{
	"amd64": "__x64_",
	"arm64": "__arm64_",
}

// ChooseSyscallProbeExit chooses between a tracepoint or kretprobe based on configuration and
// host-supported features.
func (c *Config) ChooseSyscallProbeExit(tracepoint string, fallback string) (string, error) {
	// return value doesn't require the indirection
	return c.ChooseSyscallProbe(tracepoint, "", fallback)
}

// ChooseSyscallProbe chooses between a tracepoint, arch-specific kprobe, or arch-agnostic kprobe based on
// configuration and host-supported features.
func (c *Config) ChooseSyscallProbe(tracepoint string, indirectProbe string, fallback string) (string, error) {
	tparts := strings.Split(tracepoint, "/")
	if len(tparts) != 3 || tparts[0] != "tracepoint" || tparts[1] != "syscalls" {
		return "", fmt.Errorf("invalid tracepoint name")
	}
	category := tparts[1]
	tpName := tparts[2]

	fparts := strings.Split(fallback, "/")
	if len(fparts) != 2 {
		return "", fmt.Errorf("invalid fallback probe name")
	}
	syscall := strings.TrimPrefix(fparts[1], "sys_")

	if indirectProbe != "" {
		xparts := strings.Split(indirectProbe, "/")
		if len(xparts) < 2 {
			return "", fmt.Errorf("invalid indirect probe name")
		}
		if strings.TrimPrefix(xparts[1], "sys_") != syscall {
			return "", fmt.Errorf("indirect and fallback probe syscalls do not match")
		}
	}

	if id, err := manager.GetTracepointID(category, tpName); c.EnableTracepoints && err == nil && id != -1 {
		log.Infof("Using a tracepoint to probe %s syscall", syscall)
		return tracepoint, nil
	}

	if indirectProbe != "" {
		// In linux kernel version 4.17(?) they added architecture specific calling conventions to syscalls within the kernel.
		// When attaching a kprobe to the `__x64_sys_` or `__arm64_sys_` prefixed syscall, all the arguments are behind an additional layer of
		// indirection. We are detecting this at runtime, and setting the constant `use_indirect_syscall` so the kprobe code
		// accesses the arguments correctly.
		//
		// For example:
		// int domain;
		// struct pt_regs *_ctx = (struct pt_regs*)PT_REGS_PARM1(ctx);
		// bpf_probe_read(&domain, sizeof(domain), &(PT_REGS_PARM1(_ctx)));
		//
		// Instead of:
		// int domain = PT_REGS_PARM1(ctx);
		//
		if sysName, err := manager.GetSyscallFnName(syscall); err == nil {
			if prefix, ok := indirectSyscallPrefixes[runtime.GOARCH]; ok && strings.HasPrefix(sysName, prefix) {
				return indirectProbe, nil
			}
		}
	}
	return fallback, nil
}
