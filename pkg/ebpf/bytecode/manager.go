// +build linux_bpf

package bytecode

import (
	"github.com/DataDog/ebpf/manager"
	"os"
)

const (
	// maxActive configures the maximum number of instances of the kretprobe-probed functions handled simultaneously.
	// This value should be enough for typical workloads (e.g. some amount of processes blocked on the accept syscall).
	maxActive     = 128
	syscallPrefix = "sys_"
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
			{Section: string(TCPv6ConnectReturn), KProbeMaxActive: maxActive},
		},
	}
}

func NewManager(perf *ClosedConnPerfHandler) *manager.Manager {
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
			{Name: string(ConfigMap)},
			{Name: string(LatestTimestampMap)},
		},
		PerfMaps: []*manager.PerfMap{
			{
				Map: manager.Map{Name: string(TcpCloseEventMap)},
				PerfMapOptions: manager.PerfMapOptions{
					PerfRingBufferSize: 8 * os.Getpagesize(),
					Watermark:          1,
					DataHandler:        perf.dataHandler,
					LostHandler:        perf.lostHandler,
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
			{Section: string(UDPSendMsg)},
			{Section: string(UDPSendMsgPre410)},
			{Section: string(UDPRecvMsg)},
			{Section: string(UDPRecvMsgPre410)},
			{Section: string(UDPRecvMsgReturn), KProbeMaxActive: maxActive},
			{Section: string(TCPRetransmit)},
			{Section: string(InetCskAcceptReturn), KProbeMaxActive: maxActive},
			{Section: string(TCPv4DestroySock)},
			{Section: string(UDPDestroySock)},
			{Section: string(SysBind), SyscallFuncName: syscallPrefix + "bind"},
			{Section: string(SysBindRet), SyscallFuncName: syscallPrefix + "bind", KProbeMaxActive: maxActive},
			{Section: string(SysSocket), SyscallFuncName: syscallPrefix + "socket"},
			{Section: string(SysSocketRet), SyscallFuncName: syscallPrefix + "socket", KProbeMaxActive: maxActive},
			{Section: string(SocketDnsFilter)},
		},
	}
}

func ConfigureMapMaxEntries(m *manager.Manager, sizes map[BPFMapName]uint32) {
	for _, mp := range m.Maps {
		if maxSize, ok := sizes[BPFMapName(mp.Name)]; ok {
			mp.MaxEntries = maxSize
		}
	}
}
