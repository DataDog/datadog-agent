// +build linux_bpf

package ebpf

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/ebpf/manager"
)

const (
	// maxActive configures the maximum number of instances of the kretprobe-probed functions handled simultaneously.
	// This value should be enough for typical workloads (e.g. some amount of processes blocked on the accept syscall).
	maxActive = 128
)

func NewOffsetManager() *manager.Manager {
	return &manager.Manager{
		Maps: []*manager.Map{
			{Name: "connectsock_ipv6"},
			{Name: string(probes.TracerStatusMap)},
		},
		PerfMaps: []*manager.PerfMap{},
		Probes: []*manager.Probe{
			{Section: string(probes.TCPGetSockOpt)},
			{Section: string(probes.TCPv6Connect)},
			{Section: string(probes.IPMakeSkb)},
			{Section: string(probes.TCPv6ConnectReturn), KProbeMaxActive: maxActive},
		},
	}
}

func NewManager(closedHandler, httpHandler *ebpf.PerfHandler) *manager.Manager {
	return &manager.Manager{
		Maps: []*manager.Map{
			{Name: string(probes.ConnMap)},
			{Name: string(probes.TcpStatsMap)},
			{Name: string(probes.TcpCloseBatchMap)},
			{Name: "udp_recv_sock"},
			{Name: string(probes.PortBindingsMap)},
			{Name: string(probes.UdpPortBindingsMap)},
			{Name: "pending_bind"},
			{Name: string(probes.TelemetryMap)},
			{Name: string(probes.HttpInFlightMap)},
			{Name: string(probes.HttpBatchesMap)},
			{Name: string(probes.HttpBatchStateMap)},
		},
		PerfMaps: []*manager.PerfMap{
			{
				Map: manager.Map{Name: string(probes.TcpCloseEventMap)},
				PerfMapOptions: manager.PerfMapOptions{
					PerfRingBufferSize: 8 * os.Getpagesize(),
					Watermark:          1,
					DataHandler:        closedHandler.DataHandler,
					LostHandler:        closedHandler.LostHandler,
				},
			},
			{
				Map: manager.Map{Name: string(probes.HttpNotificationsMap)},
				PerfMapOptions: manager.PerfMapOptions{
					PerfRingBufferSize: 8 * os.Getpagesize(),
					Watermark:          1,
					DataHandler:        httpHandler.DataHandler,
					LostHandler:        httpHandler.LostHandler,
				},
			},
		},
		Probes: []*manager.Probe{
			{Section: string(probes.TCPSendMsg)},
			{Section: string(probes.TCPSendMsgPre410), MatchFuncName: "^tcp_sendmsg$"},
			{Section: string(probes.TCPSendMsgReturn), KProbeMaxActive: maxActive},
			{Section: string(probes.TCPCleanupRBuf)},
			{Section: string(probes.TCPClose)},
			{Section: string(probes.TCPCloseReturn), KProbeMaxActive: maxActive},
			{Section: string(probes.TCPSetState)},
			{Section: string(probes.IPMakeSkb)},
			{Section: string(probes.IP6MakeSkb)},
			{Section: string(probes.UDPRecvMsg)},
			{Section: string(probes.UDPRecvMsgPre410), MatchFuncName: "^udp_recvmsg$"},
			{Section: string(probes.UDPRecvMsgReturn), KProbeMaxActive: maxActive},
			{Section: string(probes.TCPRetransmit)},
			{Section: string(probes.InetCskAcceptReturn), KProbeMaxActive: maxActive},
			{Section: string(probes.TCPv4DestroySock)},
			{Section: string(probes.UDPDestroySock)},
			{Section: string(probes.InetBind)},
			{Section: string(probes.Inet6Bind)},
			{Section: string(probes.InetBindRet), KProbeMaxActive: maxActive},
			{Section: string(probes.Inet6BindRet), KProbeMaxActive: maxActive},
			{Section: string(probes.SocketDnsFilter)},
			{Section: string(probes.SocketHTTPFilter)},
		},
	}
}
