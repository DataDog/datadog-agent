// +build linux_bpf

package bytecode

import (
	"os"

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
			{Name: string(TracerStatusMap)},
		},
		PerfMaps: []*manager.PerfMap{},
		Probes: []*manager.Probe{
			{Section: string(TCPGetInfo)},
			{Section: string(TCPv6Connect)},
			{Section: string(IPMakeSkb)},
			{Section: string(TCPv6ConnectReturn), KProbeMaxActive: maxActive},
		},
	}
}

func NewManager(closedHandler *PerfHandler) *manager.Manager {
	return &manager.Manager{
		Maps: []*manager.Map{
			{Name: string(ConnMap)},
			{Name: string(TcpStatsMap)},
			{Name: string(TcpCloseBatchMap)},
			{Name: "udp_recv_sock"},
			{Name: string(PortBindingsMap)},
			{Name: string(UdpPortBindingsMap)},
			{Name: "pending_sockets"},
			{Name: "pending_bind"},
			{Name: "unbound_sockets"},
			{Name: string(TelemetryMap)},
		},
		PerfMaps: []*manager.PerfMap{
			{
				Map: manager.Map{Name: string(TcpCloseEventMap)},
				PerfMapOptions: manager.PerfMapOptions{
					PerfRingBufferSize: 8 * os.Getpagesize(),
					Watermark:          1,
					DataHandler:        closedHandler.dataHandler,
					LostHandler:        closedHandler.lostHandler,
				},
			},
		},
		Probes: []*manager.Probe{
			{Section: string(TCPSendMsg)},
			{Section: string(TCPSendMsgPre410)},
			{Section: string(TCPSendMsgReturn), KProbeMaxActive: maxActive},
			{Section: string(TCPCleanupRBuf)},
			{Section: string(TCPClose)},
			{Section: string(TCPCloseReturn), KProbeMaxActive: maxActive},
			{Section: string(TCPSetState)},
			{Section: string(IPMakeSkb)},
			{Section: string(IP6MakeSkb)},
			{Section: string(UDPRecvMsg)},
			{Section: string(UDPRecvMsgPre410)},
			{Section: string(UDPRecvMsgReturn), KProbeMaxActive: maxActive},
			{Section: string(TCPRetransmit)},
			{Section: string(InetCskAcceptReturn), KProbeMaxActive: maxActive},
			{Section: string(TCPv4DestroySock)},
			{Section: string(UDPDestroySock)},
			{Section: string(SysBind), SyscallFuncName: "bind"},
			{Section: string(SysBindX64), SyscallFuncName: "bind"},
			{Section: string(TraceSysBindEnter)},
			{Section: string(SysBindRet), SyscallFuncName: "bind", KProbeMaxActive: maxActive},
			{Section: string(TraceSysBindExit)},
			{Section: string(SysSocket), SyscallFuncName: "socket"},
			{Section: string(SysSocketX64), SyscallFuncName: "socket"},
			{Section: string(TraceSysSocketEnter)},
			{Section: string(SysSocketRet), SyscallFuncName: "socket", KProbeMaxActive: maxActive},
			{Section: string(TraceSysSocketExit)},
			{Section: string(SocketDnsFilter)},
		},
	}
}
