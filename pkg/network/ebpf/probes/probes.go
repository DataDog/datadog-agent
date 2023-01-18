// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package probes

import "fmt"

// ProbeName stores the name of the kernel probes setup for tracing
type ProbeName string

const (
	// InetCskListenStop traces the inet_csk_listen_stop system call (called for both ipv4 and ipv6)
	InetCskListenStop ProbeName = "kprobe/inet_csk_listen_stop"

	// TCPConnect traces the connect() system call
	TCPConnect ProbeName = "kprobe/tcp_connect"
	// TCPFinishConnect traces tcp_finish_connect() kernel function. This is
	// used to know when a TCP connection switches to the ESTABLISHED state
	TCPFinishConnect ProbeName = "kprobe/tcp_finish_connect"
	// TCPv6Connect traces the v6 connect() system call
	TCPv6Connect ProbeName = "kprobe/tcp_v6_connect"
	// TCPv6ConnectReturn traces the return value for the v6 connect() system call
	TCPv6ConnectReturn ProbeName = "kretprobe/tcp_v6_connect"

	// ProtocolClassifierSocketFilter runs a classifier algorithm as a socket filer
	ProtocolClassifierSocketFilter ProbeName = "socket/classifier"

	// NetDevQueue runs a tracepoint that allows us to correlate __sk_buf (in a socket filter) with the `struct sock*`
	// belongs (but hidden) for it.
	NetDevQueue ProbeName = "tracepoint/net/net_dev_queue"

	// TCPSendMsg traces the tcp_sendmsg() system call
	TCPSendMsg ProbeName = "kprobe/tcp_sendmsg"

	// TCPSendMsgPre410 traces the tcp_sendmsg() system call on kernels prior to 4.1.0. This is created because
	// we need to load a different kprobe implementation
	TCPSendMsgPre410 ProbeName = "kprobe/tcp_sendmsg/pre_4_1_0"

	// TCPSendMsgReturn traces the return value for the tcp_sendmsg() system call
	// XXX: This is only used for telemetry for now to count the number of errors returned
	// by the tcp_sendmsg func (so we can have a # of tcp sent bytes we miscounted)
	TCPSendMsgReturn ProbeName = "kretprobe/tcp_sendmsg"

	// TCPGetSockOpt traces the tcp_getsockopt() kernel function
	// This probe is used for offset guessing only
	TCPGetSockOpt ProbeName = "kprobe/tcp_getsockopt"

	// SockGetSockOpt traces the sock_common_getsockopt() kernel function
	// This probe is used for offset guessing only
	SockGetSockOpt ProbeName = "kprobe/sock_common_getsockopt"

	// TCPSetState traces the tcp_set_state() kernel function
	TCPSetState ProbeName = "kprobe/tcp_set_state"

	// TCPRecvMsg traces the tcp_recvmsg() kernel function
	TCPRecvMsg ProbeName = "kprobe/tcp_recvmsg"
	// TCPRecvMsgPre410 traces the tcp_recvmsg() system call on kernels prior to 4.1.0. This is created because
	// we need to load a different kprobe implementation
	TCPRecvMsgPre410 ProbeName = "kprobe/tcp_recvmsg/pre_4_1_0"
	// TCPRecvMsgreturn traces the return for the tcp_recvmsg() kernel function
	TCPRecvMsgReturn ProbeName = "kretprobe/tcp_recvmsg"
	// TCPReadSock traces the tcp_read_sock() kernel function
	TCPReadSock ProbeName = "kprobe/tcp_read_sock"
	// TCPReadSockReturn traces the return for the tcp_read_sock() kernel function
	TCPReadSockReturn ProbeName = "kretprobe/tcp_read_sock"

	// TCPClose traces the tcp_close() system call
	TCPClose ProbeName = "kprobe/tcp_close"
	// TCPCloseReturn traces the return of tcp_close() system call
	TCPCloseReturn ProbeName = "kretprobe/tcp_close"

	// We use the following two probes for UDP sends

	// IPMakeSkb traces ip_make_skb
	IPMakeSkb ProbeName = "kprobe/ip_make_skb"
	// IPMakeSkbReturn traces return of ip_make_skb
	IPMakeSkbReturn ProbeName = "kretprobe/ip_make_skb"
	// IP6MakeSkb traces ip6_make_skb
	IP6MakeSkb ProbeName = "kprobe/ip6_make_skb"
	// IP6MakeSkbReturn traces return of ip6_make_skb
	IP6MakeSkbReturn ProbeName = "kretprobe/ip6_make_skb"
	// IP6MakeSkbPre470 traces ip6_make_skb on kernel versions < 4.7
	IP6MakeSkbPre470 ProbeName = "kprobe/ip6_make_skb/pre_4_7_0"

	// UDPRecvMsg traces the udp_recvmsg() system call
	UDPRecvMsg ProbeName = "kprobe/udp_recvmsg"
	// UDPRecvMsgPre410 traces the udp_recvmsg() system call on kernels prior to 4.1.0
	UDPRecvMsgPre410 ProbeName = "kprobe/udp_recvmsg/pre_4_1_0"
	// UDPRecvMsgReturn traces the return value for the udp_recvmsg() system call
	UDPRecvMsgReturn ProbeName = "kretprobe/udp_recvmsg"

	// UDPv6RecvMsg traces the udpv6_recvmsg() system call
	UDPv6RecvMsg ProbeName = "kprobe/udpv6_recvmsg"
	// UDPv6RecvMsgPre410 traces the udpv6_recvmsg() system call on kernels prior to 4.1.0
	UDPv6RecvMsgPre410 ProbeName = "kprobe/udpv6_recvmsg/pre_4_1_0"
	// UDPv6RecvMsgReturn traces the return value for the udpv6_recvmsg() system call
	UDPv6RecvMsgReturn ProbeName = "kretprobe/udpv6_recvmsg"

	// SKBConsumeUDP traces skb_consume_udp()
	SKBConsumeUDP ProbeName = "kprobe/skb_consume_udp"
	// SKBFreeDatagramLocked traces skb_free_datagram_locked()
	SKBFreeDatagramLocked ProbeName = "kprobe/skb_free_datagram_locked"
	// UnderscoredSKBFreeDatagramLocked traces __skb_free_datagram_locked()
	UnderscoredSKBFreeDatagramLocked ProbeName = "kprobe/__skb_free_datagram_locked"

	// UDPDestroySock traces the udp_destroy_sock() function
	UDPDestroySock ProbeName = "kprobe/udp_destroy_sock"
	// UDPDestroySockReturn traces the return of the udp_destroy_sock() system call
	UDPDestroySockReturn ProbeName = "kretprobe/udp_destroy_sock"

	// TCPRetransmit traces the params for the tcp_retransmit_skb() system call
	TCPRetransmit ProbeName = "kprobe/tcp_retransmit_skb"
	// TCPRetransmitPre470 traces the params for the tcp_retransmit_skb() system call on kernel version < 4.7
	TCPRetransmitPre470 ProbeName = "kprobe/tcp_retransmit_skb/pre_4_7_0"
	// TCPRetransmitRet traces the return value for the tcp_retransmit_skb() system call
	TCPRetransmitRet ProbeName = "kretprobe/tcp_retransmit_skb"

	// InetCskAcceptReturn traces the return value for the inet_csk_accept syscall
	InetCskAcceptReturn ProbeName = "kretprobe/inet_csk_accept"

	// InetBind is the kprobe of the bind() syscall for IPv4
	InetBind ProbeName = "kprobe/inet_bind"
	// Inet6Bind is the kprobe of the bind() syscall for IPv6
	Inet6Bind ProbeName = "kprobe/inet6_bind"

	// InetBindRet is the kretprobe of the bind() syscall for IPv4
	InetBindRet ProbeName = "kretprobe/inet_bind"
	// Inet6BindRet is the kretprobe of the bind() syscall for IPv6
	Inet6BindRet ProbeName = "kretprobe/inet6_bind"

	// SocketDNSFilter is the socket probe for dns
	SocketDNSFilter ProbeName = "socket/dns_filter"

	// ConntrackHashInsert is the probe for new conntrack entries
	ConntrackHashInsert ProbeName = "kprobe/__nf_conntrack_hash_insert"

	// ConntrackFillInfo is the probe for for dumping existing conntrack entries
	ConntrackFillInfo ProbeName = "kprobe/ctnetlink_fill_info"

	// SockFDLookup is the kprobe used for mapping socket FDs to kernel sock structs
	SockFDLookup ProbeName = "kprobe/sockfd_lookup_light"

	// SockFDLookupRet is the kretprobe used for mapping socket FDs to kernel sock structs
	SockFDLookupRet ProbeName = "kretprobe/sockfd_lookup_light"

	// DoSendfile is the kprobe used to trace traffic via SENDFILE(2) syscall
	DoSendfile ProbeName = "kprobe/do_sendfile"

	// DoSendfileRet is the kretprobe used to trace traffic via SENDFILE(2) syscall
	DoSendfileRet ProbeName = "kretprobe/do_sendfile"
)

// BPFMapName stores the name of the BPF maps storing statistics and other info
type BPFMapName string

// constants for the map names
const (
	ConnMap                      BPFMapName = "conn_stats"
	TCPStatsMap                  BPFMapName = "tcp_stats"
	TCPConnectSockPidMap         BPFMapName = "tcp_ongoing_connect_pid"
	ConnCloseEventMap            BPFMapName = "conn_close_event"
	TracerStatusMap              BPFMapName = "tracer_status"
	PortBindingsMap              BPFMapName = "port_bindings"
	UDPPortBindingsMap           BPFMapName = "udp_port_bindings"
	TelemetryMap                 BPFMapName = "telemetry"
	ConnCloseBatchMap            BPFMapName = "conn_close_batch"
	ConntrackMap                 BPFMapName = "conntrack"
	ConntrackTelemetryMap        BPFMapName = "conntrack_telemetry"
	SockFDLookupArgsMap          BPFMapName = "sockfd_lookup_args"
	DoSendfileArgsMap            BPFMapName = "do_sendfile_args"
	SockByPidFDMap               BPFMapName = "sock_by_pid_fd"
	PidFDBySockMap               BPFMapName = "pid_fd_by_sock"
	TagsMap                      BPFMapName = "conn_tags"
	TcpSendMsgArgsMap            BPFMapName = "tcp_sendmsg_args"
	IpMakeSkbArgsMap             BPFMapName = "ip_make_skb_args"
	MapErrTelemetryMap           BPFMapName = "map_err_telemetry_map"
	HelperErrTelemetryMap        BPFMapName = "helper_err_telemetry_map"
	TcpRecvMsgArgsMap            BPFMapName = "tcp_recvmsg_args"
	ProtocolClassificationBufMap BPFMapName = "classification_buf"
	StaticTableMap               BPFMapName = "http2_static_table"
)

// SectionName returns the SectionName for the given BPF map
func (b BPFMapName) SectionName() string {
	return fmt.Sprintf("maps/%s", b)
}
