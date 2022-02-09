//+build linux_bpf

package kprobe

import (
	"fmt"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// enabledProbes returns a map of probes that are enabled per config settings.
// This map does not include the probes used exclusively in the offset guessing process.
func enabledProbes(c *config.Config, runtimeTracer bool) (map[probes.ProbeName]struct{}, error) {
	enabled := make(map[probes.ProbeName]struct{}, 0)
	ksymPath := filepath.Join(c.ProcRoot, "kallsyms")

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

		missing, err := ebpf.VerifyKernelFuncs(ksymPath, []string{"sockfd_lookup_light"})
		if err == nil && len(missing) == 0 {
			enabled[probes.SockFDLookup] = struct{}{}
			enabled[probes.SockFDLookupRet] = struct{}{}
			enabled[probes.DoSendfile] = struct{}{}
			enabled[probes.DoSendfileRet] = struct{}{}
		}
	}

	if c.CollectUDPConns {
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

		if runtimeTracer {
			missing, err := ebpf.VerifyKernelFuncs(ksymPath, []string{"skb_consume_udp", "__skb_free_datagram_locked", "skb_free_datagram_locked"})
			if err != nil {
				return nil, fmt.Errorf("error verifying kernel function presence: %s", err)
			}

			enabled[probes.UDPRecvMsg] = struct{}{}
			enabled[probes.UDPRecvMsgReturn] = struct{}{}
			if c.CollectIPv6Conns {
				enabled[probes.UDPv6RecvMsg] = struct{}{}
				enabled[probes.UDPv6RecvMsgReturn] = struct{}{}
			}

			if _, miss := missing["skb_consume_udp"]; !miss {
				enabled[probes.SKBConsumeUDP] = struct{}{}
			} else if _, miss := missing["__skb_free_datagram_locked"]; !miss {
				enabled[probes.SKB__FreeDatagramLocked] = struct{}{}
			} else if _, miss := missing["skb_free_datagram_locked"]; !miss {
				enabled[probes.SKBFreeDatagramLocked] = struct{}{}
			} else {
				return nil, fmt.Errorf("missing desired UDP receive kernel functions")
			}
		} else {
			if pre410Kernel {
				enabled[probes.UDPRecvMsgPre410] = struct{}{}
			} else {
				enabled[probes.UDPRecvMsg] = struct{}{}
			}
			enabled[probes.UDPRecvMsgReturn] = struct{}{}
		}
	}

	return enabled, nil
}
