// +build linux_bpf

package config

import (
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// EnabledProbes returns a map of probes that are enabled per config settings.
// This map does not include the probes used exclusively in the offset guessing process.
func (c *Config) EnabledProbes(runtimeTracer bool) (map[probes.ProbeName]struct{}, error) {
	enabled := make(map[probes.ProbeName]struct{}, 0)

	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, err
	}
	pre410Kernel := kv < kernel.VersionCode(4, 1, 0)

	if c.CollectTCPConns {
		if !runtimeTracer && pre410Kernel {
			enabled[probes.TCPSendMsgPre410] = struct{}{}
		} else {
			enabled[probes.TCPSendMsg] = struct{}{}
		}
		enabled[probes.TCPCleanupRBuf] = struct{}{}
		enabled[probes.TCPClose] = struct{}{}
		enabled[probes.TCPCloseReturn] = struct{}{}
		enabled[probes.InetCskAcceptReturn] = struct{}{}
		enabled[probes.InetCskListenStop] = struct{}{}
		enabled[probes.TCPSetState] = struct{}{}

		if !runtimeTracer && kv < kernel.VersionCode(4, 7, 0) {
			enabled[probes.TCPRetransmitPre470] = struct{}{}
		} else {
			enabled[probes.TCPRetransmit] = struct{}{}
		}
	}

	if c.CollectUDPConns {
		enabled[probes.UDPRecvMsgReturn] = struct{}{}
		enabled[probes.UDPDestroySock] = struct{}{}
		enabled[probes.UDPDestroySockReturn] = struct{}{}
		enabled[probes.IPMakeSkb] = struct{}{}
		enabled[probes.InetBind] = struct{}{}
		enabled[probes.InetBindRet] = struct{}{}

		if c.CollectIPv6Conns {
			if !runtimeTracer && kv < kernel.VersionCode(4, 7, 0) {
				enabled[probes.IP6MakeSkbPre470] = struct{}{}
			} else {
				enabled[probes.IP6MakeSkb] = struct{}{}
			}

			enabled[probes.Inet6Bind] = struct{}{}
			enabled[probes.Inet6BindRet] = struct{}{}
		}

		if !runtimeTracer && pre410Kernel {
			enabled[probes.UDPRecvMsgPre410] = struct{}{}
		} else {
			enabled[probes.UDPRecvMsg] = struct{}{}
		}
	}

	return enabled, nil
}
