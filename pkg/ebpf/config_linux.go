// +build linux_bpf

package ebpf

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/ebpf/manager"
	"strings"
)

// EnabledProbes returns a map of probes that are enabled per config settings.
// This map does not include the probes used exclusively in the offset guessing process.
func (c *Config) EnabledProbes(pre410Kernel bool) map[bytecode.ProbeName]struct{} {
	enabled := make(map[bytecode.ProbeName]struct{}, 0)

	if c.CollectTCPConns {
		if pre410Kernel {
			enabled[bytecode.TCPSendMsgPre410] = struct{}{}
		} else {
			enabled[bytecode.TCPSendMsg] = struct{}{}
		}
		enabled[bytecode.TCPCleanupRBuf] = struct{}{}
		enabled[bytecode.TCPClose] = struct{}{}
		enabled[bytecode.TCPCloseReturn] = struct{}{}
		enabled[bytecode.TCPRetransmit] = struct{}{}
		enabled[bytecode.InetCskAcceptReturn] = struct{}{}
		enabled[bytecode.TCPv4DestroySock] = struct{}{}

		if c.BPFDebug {
			enabled[bytecode.TCPSendMsgReturn] = struct{}{}
		}
	}

	if c.CollectUDPConns {
		enabled[bytecode.UDPRecvMsgReturn] = struct{}{}
		enabled[bytecode.UDPDestroySock] = struct{}{}

		if pre410Kernel {
			enabled[bytecode.UDPSendMsgPre410] = struct{}{}
			enabled[bytecode.UDPRecvMsgPre410] = struct{}{}
		} else {
			enabled[bytecode.UDPRecvMsg] = struct{}{}
			enabled[bytecode.UDPSendMsg] = struct{}{}
		}

		// try to use tracepoints if available
		if id, err := manager.GetTracepointID("syscalls", "sys_enter_bind"); err == nil && id != -1 {
			enabled[bytecode.TraceSysBindEnter] = struct{}{}
			enabled[bytecode.TraceSysBindExit] = struct{}{}
		} else {
			enabled[bytecode.SysBindRet] = struct{}{}
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
			if sysName, err := manager.GetSyscallFnName("sys_bind"); err == nil && strings.HasPrefix(sysName, "__x64_") {
				enabled[bytecode.SysBindX64] = struct{}{}
			} else {
				enabled[bytecode.SysBind] = struct{}{}
			}
		}
		if id, err := manager.GetTracepointID("syscalls", "sys_enter_socket"); err == nil && id != -1 {
			enabled[bytecode.TraceSysSocketEnter] = struct{}{}
			enabled[bytecode.TraceSysSocketExit] = struct{}{}
		} else {
			enabled[bytecode.SysSocketRet] = struct{}{}
			// see if using x64 syscall functions
			if sysName, err := manager.GetSyscallFnName("sys_socket"); err == nil && strings.HasPrefix(sysName, "__x64_") {
				enabled[bytecode.SysSocketX64] = struct{}{}
			} else {
				enabled[bytecode.SysSocket] = struct{}{}
			}
		}
	}

	return enabled
}
