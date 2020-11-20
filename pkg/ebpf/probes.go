// +build linux_bpf

package ebpf

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf/manager"
)

const x64SyscallPrefix = "__x64_"

// ChooseSyscallProbeExit chooses between a tracepoint or kretprobe based on configuration and
// host-supported features.
func (c *Config) ChooseSyscallProbeExit(tracepoint string, fallback string) (string, error) {
	// return value doesn't require the x64 indirection
	return c.ChooseSyscallProbe(tracepoint, "", fallback)
}

// ChooseSyscallProbe chooses between a tracepoint, arch-specific kprobe, or arch-agnostic kprobe based on
// configuration and host-supported features.
func (c *Config) ChooseSyscallProbe(tracepoint string, x64probe string, fallback string) (string, error) {
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
	syscall := fparts[1]

	if x64probe != "" {
		xparts := strings.Split(x64probe, "/")
		if len(xparts) < 2 {
			return "", fmt.Errorf("invalid x64 probe name")
		}
		if xparts[1] != syscall {
			return "", fmt.Errorf("x64 and fallback probe syscalls do not match")
		}
	}

	if id, err := manager.GetTracepointID(category, tpName); c.EnableTracepoints && err == nil && id != -1 {
		log.Info("Using a tracepoint to probe bind syscall")
		return tracepoint, nil
	}

	if x64probe != "" {
		// In linux kernel version 4.17(?) they added architecture specific calling conventions to syscalls within the kernel.
		// When attaching a kprobe to the `__x64_sys_` prefixed syscall, all the arguments are behind an additional layer of
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
		if sysName, err := manager.GetSyscallFnName(syscall); err == nil && strings.HasPrefix(sysName, x64SyscallPrefix) {
			return x64probe, nil
		}
	}
	return fallback, nil
}
