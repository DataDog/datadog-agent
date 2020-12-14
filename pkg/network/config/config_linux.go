// +build linux_bpf

package config

import (
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
)

// EnabledProbes returns a map of probes that are enabled per config settings.
// This map does not include the probes used exclusively in the offset guessing process.
func (c *Config) EnabledProbes(pre410Kernel bool) (map[probes.ProbeName]struct{}, error) {
	enabled := make(map[probes.ProbeName]struct{}, 0)

	if c.CollectTCPConns {
		if pre410Kernel {
			enabled[probes.TCPSendMsgPre410] = struct{}{}
		} else {
			enabled[probes.TCPSendMsg] = struct{}{}
		}
		enabled[probes.TCPCleanupRBuf] = struct{}{}
		enabled[probes.TCPClose] = struct{}{}
		enabled[probes.TCPCloseReturn] = struct{}{}
		enabled[probes.TCPRetransmit] = struct{}{}
		enabled[probes.InetCskAcceptReturn] = struct{}{}
		enabled[probes.TCPv4DestroySock] = struct{}{}
		enabled[probes.TCPSetState] = struct{}{}

		if c.BPFDebug || c.EnableHTTPMonitoring {
			enabled[probes.TCPSendMsgReturn] = struct{}{}
		}
	}

	if c.CollectUDPConns {
		enabled[probes.UDPRecvMsgReturn] = struct{}{}
		enabled[probes.UDPDestroySock] = struct{}{}
		enabled[probes.IPMakeSkb] = struct{}{}
		enabled[probes.IP6MakeSkb] = struct{}{}

		if pre410Kernel {
			enabled[probes.UDPRecvMsgPre410] = struct{}{}
		} else {
			enabled[probes.UDPRecvMsg] = struct{}{}
		}

		tp, err := c.ChooseSyscallProbe(string(probes.TraceSysBindEnter), string(probes.SysBindX64), string(probes.SysBind))
		if err != nil {
			return nil, err
		}
		enabled[probes.ProbeName(tp)] = struct{}{}

		tp, err = c.ChooseSyscallProbeExit(string(probes.TraceSysBindExit), string(probes.SysBindRet))
		if err != nil {
			return nil, err
		}
		enabled[probes.ProbeName(tp)] = struct{}{}

		tp, err = c.ChooseSyscallProbe(string(probes.TraceSysSocketEnter), string(probes.SysSocketX64), string(probes.SysSocket))
		if err != nil {
			return nil, err
		}
		enabled[probes.ProbeName(tp)] = struct{}{}

		tp, err = c.ChooseSyscallProbeExit(string(probes.TraceSysSocketExit), string(probes.SysSocketRet))
		if err != nil {
			return nil, err
		}
		enabled[probes.ProbeName(tp)] = struct{}{}
	}

	return enabled, nil
}
