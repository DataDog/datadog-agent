// +build linux_bpf

package bytecode

import "fmt"

// ProbeName stores the name of the kernel probes setup for tracing
type ProbeName string

const (
	// TCPv4DestroySock traces the tcp_v4_destroy_sock system call (called for both ipv4 and ipv6)
	TCPv4DestroySock ProbeName = "kprobe/tcp_v4_destroy_sock"

	// TCPv6Connect traces the v6 connect() system call
	TCPv6Connect ProbeName = "kprobe/tcp_v6_connect"
	// TCPv6ConnectReturn traces the return value for the v6 connect() system call
	TCPv6ConnectReturn ProbeName = "kretprobe/tcp_v6_connect"

	// TCPSendMsg traces the tcp_sendmsg() system call
	TCPSendMsg ProbeName = "kprobe/tcp_sendmsg"

	// TCPSendMsgPre410 traces the tcp_sendmsg() system call on kernels prior to 4.1.0. This is created because
	// we need to load a different kprobe implementation
	TCPSendMsgPre410 ProbeName = "kprobe/tcp_sendmsg/pre_4_1_0"

	// TCPSendMsgReturn traces the return value for the tcp_sendmsg() system call
	// XXX: This is only used for telemetry for now to count the number of errors returned
	// by the tcp_sendmsg func (so we can have a # of tcp sent bytes we miscounted)
	TCPSendMsgReturn ProbeName = "kretprobe/tcp_sendmsg"

	// TCPGetInfo traces the tcp_get_info() system call
	// This probe is used for offset guessing only
	TCPGetInfo ProbeName = "kprobe/tcp_get_info"

	// TCPSetState traces the tcp_set_state() kernel function
	TCPSetState ProbeName = "kprobe/tcp_set_state"

	// TCPCleanupRBuf traces the tcp_cleanup_rbuf() system call
	TCPCleanupRBuf ProbeName = "kprobe/tcp_cleanup_rbuf"
	// TCPClose traces the tcp_close() system call
	TCPClose ProbeName = "kprobe/tcp_close"
	// TCPCloseReturn traces the return of tcp_close() system call
	TCPCloseReturn ProbeName = "kretprobe/tcp_close"

	// We use the following two probes for UDP sends
	IPMakeSkb  ProbeName = "kprobe/ip_make_skb"
	IP6MakeSkb ProbeName = "kprobe/ip6_make_skb"

	// UDPRecvMsg traces the udp_recvmsg() system call
	UDPRecvMsg ProbeName = "kprobe/udp_recvmsg"
	// UDPRecvMsgPre410 traces the udp_recvmsg() system call on kernels prior to 4.1.0
	UDPRecvMsgPre410 ProbeName = "kprobe/udp_recvmsg/pre_4_1_0"
	// UDPRecvMsgReturn traces the return value for the udp_recvmsg() system call
	UDPRecvMsgReturn ProbeName = "kretprobe/udp_recvmsg"

	// UDPDestroySock traces the udp_destroy_sock() function
	UDPDestroySock ProbeName = "kprobe/udp_destroy_sock"

	// TCPRetransmit traces the return value for the tcp_retransmit_skb() system call
	TCPRetransmit ProbeName = "kprobe/tcp_retransmit_skb"

	// InetCskAcceptReturn traces the return value for the inet_csk_accept syscall
	InetCskAcceptReturn ProbeName = "kretprobe/inet_csk_accept"

	// SysSocket traces calls to the socket kprobe
	SysSocket    ProbeName = "kprobe/sys_socket"
	SysSocketX64 ProbeName = "kprobe/sys_socket/x64"

	// SysSocketRet is the kretprobe for SysSocket
	SysSocketRet ProbeName = "kretprobe/sys_socket"

	// SysBind is the kprobe the bind() syscall.
	SysBind    ProbeName = "kprobe/sys_bind"
	SysBindX64 ProbeName = "kprobe/sys_bind/x64"

	// SysBindRet is the kretprobe for bind().
	SysBindRet ProbeName = "kretprobe/sys_bind"

	// SocketDnsFilter is the socket probe for dns
	SocketDnsFilter ProbeName = "socket/dns_filter"
)

const (
	TraceSysBindEnter ProbeName = "tracepoint/syscalls/sys_enter_bind"
	TraceSysBindExit  ProbeName = "tracepoint/syscalls/sys_exit_bind"

	TraceSysSocketEnter ProbeName = "tracepoint/syscalls/sys_enter_socket"
	TraceSysSocketExit  ProbeName = "tracepoint/syscalls/sys_exit_socket"
)

// BPFMapName stores the name of the BPF maps storing statistics and other info
type BPFMapName string

const (
	ConnMap            BPFMapName = "conn_stats"
	TcpStatsMap        BPFMapName = "tcp_stats"
	TcpCloseEventMap   BPFMapName = "tcp_close_event"
	TracerStatusMap    BPFMapName = "tracer_status"
	PortBindingsMap    BPFMapName = "port_bindings"
	UdpPortBindingsMap BPFMapName = "udp_port_bindings"
	TelemetryMap       BPFMapName = "telemetry"
	TcpCloseBatchMap   BPFMapName = "tcp_close_batch"
)

// SectionName returns the SectionName for the given BPF map
func (b BPFMapName) SectionName() string {
	return fmt.Sprintf("maps/%s", b)
}

var (
	// KProbeOverrides specifies a mapping between sections in our kprobe functions and
	// the actual eBPF function that it should bind to
	KProbeOverrides = map[ProbeName]ProbeName{
		TCPSendMsgPre410: TCPSendMsg,
		UDPRecvMsgPre410: UDPRecvMsg,
		SysBindX64:       SysBind,
		SysSocketX64:     SysSocket,
	}
)
