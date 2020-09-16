// +build linux_bpf

package ebpf

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/ebpf/manager"
)

const x64SyscallPrefix = "__x64_"

// EnabledProbes returns a map of probes that are enabled per config settings.
// This map does not include the probes used exclusively in the offset guessing process.
func (c *Config) EnabledProbes(pre410Kernel bool) (map[bytecode.ProbeName]struct{}, error) {
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
		enabled[bytecode.TCPSetState] = struct{}{}

		if c.BPFDebug {
			enabled[bytecode.TCPSendMsgReturn] = struct{}{}
		}
	}

	if c.CollectUDPConns {
		enabled[bytecode.UDPRecvMsgReturn] = struct{}{}
		enabled[bytecode.UDPDestroySock] = struct{}{}
		enabled[bytecode.IPMakeSkb] = struct{}{}
		enabled[bytecode.IP6MakeSkb] = struct{}{}

		if pre410Kernel {
			enabled[bytecode.UDPRecvMsgPre410] = struct{}{}
		} else {
			enabled[bytecode.UDPRecvMsg] = struct{}{}
		}

		tp, err := c.chooseSyscallProbe(bytecode.TraceSysBindEnter, bytecode.SysBindX64, bytecode.SysBind)
		if err != nil {
			return nil, err
		}
		enabled[tp] = struct{}{}

		tp, err = c.chooseSyscallProbeExit(bytecode.TraceSysBindExit, bytecode.SysBindRet)
		if err != nil {
			return nil, err
		}
		enabled[tp] = struct{}{}

		tp, err = c.chooseSyscallProbe(bytecode.TraceSysSocketEnter, bytecode.SysSocketX64, bytecode.SysSocket)
		if err != nil {
			return nil, err
		}
		enabled[tp] = struct{}{}

		tp, err = c.chooseSyscallProbeExit(bytecode.TraceSysSocketExit, bytecode.SysSocketRet)
		if err != nil {
			return nil, err
		}
		enabled[tp] = struct{}{}
	}

	return enabled, nil
}

func (c *Config) chooseSyscallProbeExit(tracepoint bytecode.ProbeName, fallback bytecode.ProbeName) (bytecode.ProbeName, error) {
	// return value doesn't require the x64 indirection
	return c.chooseSyscallProbe(tracepoint, "", fallback)
}

func (c *Config) chooseSyscallProbe(tracepoint bytecode.ProbeName, x64probe bytecode.ProbeName, fallback bytecode.ProbeName) (bytecode.ProbeName, error) {
	tparts := strings.Split(string(tracepoint), "/")
	if len(tparts) != 3 || tparts[0] != "tracepoint" || tparts[1] != "syscalls" {
		return "", fmt.Errorf("invalid tracepoint name")
	}
	category := tparts[1]
	tpName := tparts[2]

	fparts := strings.Split(string(fallback), "/")
	if len(fparts) != 2 {
		return "", fmt.Errorf("invalid fallback probe name")
	}
	syscall := fparts[1]

	if x64probe != "" {
		xparts := strings.Split(string(x64probe), "/")
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
