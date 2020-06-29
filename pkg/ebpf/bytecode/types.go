// +build linux_bpf

package bytecode

import "fmt"

// KProbeName stores the name of the kernel probes setup for tracing
type KProbeName string

const (
	// TCPv4DestroySock traces the tcp_v4_destroy_sock system call (called for both ipv4 and ipv6)
	TCPv4DestroySock KProbeName = "kprobe/tcp_v4_destroy_sock"

	// TCPv6Connect traces the v6 connect() system call
	TCPv6Connect KProbeName = "kprobe/tcp_v6_connect"
	// TCPv6ConnectReturn traces the return value for the v6 connect() system call
	TCPv6ConnectReturn KProbeName = "kretprobe/tcp_v6_connect"

	// TCPSendMsg traces the tcp_sendmsg() system call
	TCPSendMsg KProbeName = "kprobe/tcp_sendmsg"

	// TCPSendMsgPre410 traces the tcp_sendmsg() system call on kernels prior to 4.1.0. This is created because
	// we need to load a different kprobe implementation
	TCPSendMsgPre410 KProbeName = "kprobe/tcp_sendmsg/pre_4_1_0"

	// TCPSendMsgReturn traces the return value for the tcp_sendmsg() system call
	// XXX: This is only used for telemetry for now to count the number of errors returned
	// by the tcp_sendmsg func (so we can have a # of tcp sent bytes we miscounted)
	TCPSendMsgReturn KProbeName = "kretprobe/tcp_sendmsg"

	// TCPGetInfo traces the tcp_get_info() system call
	// This probe is used for offset guessing only
	TCPGetInfo KProbeName = "kprobe/tcp_get_info"

	// TCPCleanupRBuf traces the tcp_cleanup_rbuf() system call
	TCPCleanupRBuf KProbeName = "kprobe/tcp_cleanup_rbuf"
	// TCPClose traces the tcp_close() system call
	TCPClose KProbeName = "kprobe/tcp_close"
	// TCPCloseReturn traces the return of tcp_close() system call
	TCPCloseReturn KProbeName = "kretprobe/tcp_close"

	// UDPSendMsg traces the udp_sendmsg() system call
	UDPSendMsg KProbeName = "kprobe/udp_sendmsg"
	// UDPSendMsgPre410 traces the udp_sendmsg() system call on kernels prior to 4.1.0
	UDPSendMsgPre410 KProbeName = "kprobe/udp_sendmsg/pre_4_1_0"
	// UDPRecvMsg traces the udp_recvmsg() system call
	UDPRecvMsg KProbeName = "kprobe/udp_recvmsg"
	// UDPRecvMsgPre410 traces the udp_recvmsg() system call on kernels prior to 4.1.0
	UDPRecvMsgPre410 KProbeName = "kprobe/udp_recvmsg/pre_4_1_0"
	// UDPRecvMsgReturn traces the return value for the udp_recvmsg() system call
	UDPRecvMsgReturn KProbeName = "kretprobe/udp_recvmsg"

	// UDPDestroySock traces the udp_destroy_sock() function
	UDPDestroySock KProbeName = "kprobe/udp_destroy_sock"

	// TCPRetransmit traces the return value for the tcp_retransmit_skb() system call
	TCPRetransmit KProbeName = "kprobe/tcp_retransmit_skb"

	// InetCskAcceptReturn traces the return value for the inet_csk_accept syscall
	InetCskAcceptReturn KProbeName = "kretprobe/inet_csk_accept"

	// SysSocket traces calls to the socket kprobe
	SysSocket KProbeName = "kprobe/sys_socket"

	// SysSocketRet is the kretprobe for SysSocket
	SysSocketRet KProbeName = "kretprobe/sys_socket"

	// SysBind is the kprobe the bind() syscall.
	SysBind KProbeName = "kprobe/sys_bind"

	// SysBindRet is the kretprobe for bind().
	SysBindRet KProbeName = "kretprobe/sys_bind"

	// SocketDnsFilter is the socket probe for dns
	SocketDnsFilter KProbeName = "socket/dns_filter"
)

// BPFMapName stores the name of the BPF maps storing statistics and other info
type BPFMapName string

const (
	ConnMap            BPFMapName = "conn_stats"
	TcpStatsMap        BPFMapName = "tcp_stats"
	TcpCloseEventMap   BPFMapName = "tcp_close_event"
	LatestTimestampMap BPFMapName = "latest_ts"
	TracerStatusMap    BPFMapName = "tracer_status"
	PortBindingsMap    BPFMapName = "port_bindings"
	UdpPortBindingsMap BPFMapName = "udp_port_bindings"
	TelemetryMap       BPFMapName = "telemetry"
	ConfigMap          BPFMapName = "config"
	TcpCloseBatchMap   BPFMapName = "tcp_close_batch"
)

// SectionName returns the SectionName for the given BPF map
func (b BPFMapName) SectionName() string {
	return fmt.Sprintf("maps/%s", b)
}

var (
	// KProbeOverrides specifies a mapping between sections in our kprobe functions and
	// the actual eBPF function that it should bind to
	KProbeOverrides = map[KProbeName]KProbeName{
		TCPSendMsgPre410: TCPSendMsg,
		UDPSendMsgPre410: UDPSendMsg,
		UDPRecvMsgPre410: UDPRecvMsg,
	}
)
