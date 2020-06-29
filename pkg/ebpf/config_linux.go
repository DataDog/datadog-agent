// +build linux_bpf

package ebpf

import "github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"

// EnabledKProbes returns a map of kprobes that are enabled per config settings.
// This map does not include the probes used exclusively in the offset guessing process.
func (c *Config) EnabledKProbes(pre410Kernel bool) map[bytecode.KProbeName]struct{} {
	enabled := make(map[bytecode.KProbeName]struct{}, 0)

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
		enabled[bytecode.SysSocket] = struct{}{}
		enabled[bytecode.SysSocketRet] = struct{}{}
		enabled[bytecode.SysBind] = struct{}{}
		enabled[bytecode.SysBindRet] = struct{}{}
		enabled[bytecode.UDPDestroySock] = struct{}{}

		if pre410Kernel {
			enabled[bytecode.UDPSendMsgPre410] = struct{}{}
			enabled[bytecode.UDPRecvMsgPre410] = struct{}{}
		} else {
			enabled[bytecode.UDPRecvMsg] = struct{}{}
			enabled[bytecode.UDPSendMsg] = struct{}{}
		}

	}

	return enabled
}
