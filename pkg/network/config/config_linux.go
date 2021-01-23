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
		enabled[probes.InetBind] = struct{}{}
		enabled[probes.InetBindRet] = struct{}{}

		if c.CollectIPv6Conns {
			enabled[probes.IP6MakeSkb] = struct{}{}
			enabled[probes.Inet6Bind] = struct{}{}
			enabled[probes.Inet6BindRet] = struct{}{}
		}

		if pre410Kernel {
			enabled[probes.UDPRecvMsgPre410] = struct{}{}
		} else {
			enabled[probes.UDPRecvMsg] = struct{}{}
		}
	}

	return enabled, nil
}
