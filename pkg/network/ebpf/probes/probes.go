// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

//nolint:revive // TODO(NET) Fix revive linter
package probes

// ProbeFuncName stores the function name of the kernel probes setup for tracing
type ProbeFuncName = string

const (
	// InetCskListenStop traces the inet_csk_listen_stop system call (called for both ipv4 and ipv6)
	InetCskListenStop ProbeFuncName = "kprobe__inet_csk_listen_stop"

	// TCPConnect traces the connect() system call
	TCPConnect ProbeFuncName = "kprobe__tcp_connect"
	// TCPFinishConnect traces tcp_finish_connect() kernel function. This is
	// used to know when a TCP connection switches to the ESTABLISHED state
	TCPFinishConnect ProbeFuncName = "kprobe__tcp_finish_connect"
	// TCPv6Connect traces the v6 connect() system call
	TCPv6Connect ProbeFuncName = "kprobe__tcp_v6_connect"
	// TCPv6ConnectReturn traces the return value for the v6 connect() system call
	TCPv6ConnectReturn ProbeFuncName = "kretprobe__tcp_v6_connect"

	// ProtocolClassifierEntrySocketFilter runs a classifier algorithm as a socket filter
	ProtocolClassifierEntrySocketFilter ProbeFuncName = "socket__classifier_entry"
	//nolint:revive // TODO(NET) Fix revive linter
	ProtocolClassifierQueuesSocketFilter ProbeFuncName = "socket__classifier_queues"
	//nolint:revive // TODO(NET) Fix revive linter
	ProtocolClassifierDBsSocketFilter ProbeFuncName = "socket__classifier_dbs"
	//nolint:revive // TODO(NET) Fix revive linter
	ProtocolClassifierGRPCSocketFilter ProbeFuncName = "socket__classifier_grpc"

	// NetDevQueue runs a tracepoint that allows us to correlate __sk_buf (in a socket filter) with the `struct sock*`
	// belongs (but hidden) for it.
	NetDevQueue ProbeFuncName = "tracepoint__net__net_dev_queue"

	// TCPSendMsg traces the tcp_sendmsg() system call
	TCPSendMsg ProbeFuncName = "kprobe__tcp_sendmsg"
	// TCPSendPage traces the tcp_sendpage() kernel function
	TCPSendPage ProbeFuncName = "kprobe__tcp_sendpage"
	// UDPSendPage traces the udp_sendpage() kernel function
	UDPSendPage ProbeFuncName = "kprobe__udp_sendpage"

	// TCPSendMsgPre410 traces the tcp_sendmsg() system call on kernels prior to 4.1.0. This is created because
	// we need to load a different kprobe implementation
	TCPSendMsgPre410 ProbeFuncName = "kprobe__tcp_sendmsg__pre_4_1_0"

	// TCPSendMsgReturn traces the return value for the tcp_sendmsg() system call
	// XXX: This is only used for telemetry for now to count the number of errors returned
	// by the tcp_sendmsg func (so we can have a # of tcp sent bytes we miscounted)
	TCPSendMsgReturn ProbeFuncName = "kretprobe__tcp_sendmsg"
	// TCPSendPageReturn traces the return value of tcp_sendpage()
	TCPSendPageReturn ProbeFuncName = "kretprobe__tcp_sendpage"
	// UDPSendPageReturn traces the return value of udp_sendpage()
	UDPSendPageReturn ProbeFuncName = "kretprobe__udp_sendpage"

	// TCPGetSockOpt traces the tcp_getsockopt() kernel function
	// This probe is used for offset guessing only
	TCPGetSockOpt ProbeFuncName = "kprobe__tcp_getsockopt"

	// SockGetSockOpt traces the sock_common_getsockopt() kernel function
	// This probe is used for offset guessing only
	SockGetSockOpt ProbeFuncName = "kprobe__sock_common_getsockopt"

	// TCPRecvMsg traces the tcp_recvmsg() kernel function
	TCPRecvMsg ProbeFuncName = "kprobe__tcp_recvmsg"
	// TCPRecvMsgPre410 traces the tcp_recvmsg() system call on kernels prior to 4.1.0. This is created because
	// we need to load a different kprobe implementation
	TCPRecvMsgPre410 ProbeFuncName = "kprobe__tcp_recvmsg__pre_4_1_0"
	// TCPRecvMsgPre5190 traces the tcp_recvmsg() system call on kernels prior to 5.19.0
	TCPRecvMsgPre5190 ProbeFuncName = "kprobe__tcp_recvmsg__pre_5_19_0"
	// TCPRecvMsgReturn traces the return for the tcp_recvmsg() kernel function
	TCPRecvMsgReturn ProbeFuncName = "kretprobe__tcp_recvmsg"
	// TCPReadSock traces the tcp_read_sock() kernel function
	TCPReadSock ProbeFuncName = "kprobe__tcp_read_sock"
	// TCPReadSockReturn traces the return for the tcp_read_sock() kernel function
	TCPReadSockReturn ProbeFuncName = "kretprobe__tcp_read_sock"

	// TCPClose traces the tcp_close() system call
	TCPClose ProbeFuncName = "kprobe__tcp_close"
	// TCPCloseCleanProtocolsReturn traces the return of tcp_close() system call
	TCPCloseCleanProtocolsReturn ProbeFuncName = "kretprobe__tcp_close_clean_protocols"
	// TCPCloseFlushReturnRingbuffer traces the return of tcp_close() system call on kernels with ringbuffer support
	TCPCloseFlushReturnRingbuffer ProbeFuncName = "kretprobe__tcp_close_flush_batch_ringbuffer"
	// TCPCloseFlushReturnPerfbuffer traces the return of tcp_close() system call on kernels without ringbuffer support
	TCPCloseFlushReturnPerfbuffer ProbeFuncName = "kretprobe__tcp_close_flush_batch_perfbuffer"
	// TCPConnCloseEmitEventPerfbuffer is a tail call used to emit a single connection close event to the perf buffer
	TCPConnCloseEmitEventPerfbuffer ProbeFuncName = "tail_call_target_tcp_close_flush_individual_conn_perfbuffer"
	// TCPConnCloseEmitEventRingbuffer is a tail call used to emit a single connection close event to the ring or perf buffer
	TCPConnCloseEmitEventRingbuffer ProbeFuncName = "tail_call_target_tcp_close_flush_individual_conn_ringbuffer"

	// We use the following two probes for UDP sends

	// IPMakeSkb traces ip_make_skb
	IPMakeSkb ProbeFuncName = "kprobe__ip_make_skb"
	// IPMakeSkbReturn traces return of ip_make_skb
	IPMakeSkbReturn ProbeFuncName = "kretprobe__ip_make_skb"
	// IP6MakeSkb traces ip6_make_skb
	IP6MakeSkb ProbeFuncName = "kprobe__ip6_make_skb"
	// IP6MakeSkbReturn traces return of ip6_make_skb
	IP6MakeSkbReturn ProbeFuncName = "kretprobe__ip6_make_skb"
	// IP6MakeSkbPre470 traces ip6_make_skb on kernel versions < 4.7
	IP6MakeSkbPre470 ProbeFuncName = "kprobe__ip6_make_skb__pre_4_7_0"
	// IP6MakeSkbPre5180 traces ip6_make_skb on kernel versions < 5.18
	IP6MakeSkbPre5180 ProbeFuncName = "kprobe__ip6_make_skb__pre_5_18_0"

	// UDPRecvMsg traces the udp_recvmsg() system call
	UDPRecvMsg ProbeFuncName = "kprobe__udp_recvmsg"
	// UDPRecvMsgPre470 traces the udp_recvmsg() system call on kernels prior to 4.7.0
	UDPRecvMsgPre470 ProbeFuncName = "kprobe__udp_recvmsg_pre_4_7_0"
	// UDPRecvMsgPre410 traces the udp_recvmsg() system call on kernels prior to 4.1.0
	UDPRecvMsgPre410 ProbeFuncName = "kprobe__udp_recvmsg_pre_4_1_0"
	// UDPRecvMsgPre5190 traces the udp_recvmsg() system call on kernels prior to 5.19.0
	UDPRecvMsgPre5190 ProbeFuncName = "kprobe__udp_recvmsg_pre_5_19_0"
	// UDPRecvMsgReturn traces the return value for the udp_recvmsg() system call
	UDPRecvMsgReturn ProbeFuncName = "kretprobe__udp_recvmsg"
	// UDPRecvMsgReturnPre470 traces the return value for the udp_recvmsg() system call prior to 4.7.0
	UDPRecvMsgReturnPre470 ProbeFuncName = "kretprobe__udp_recvmsg_pre_4_7_0"

	// UDPv6RecvMsg traces the udpv6_recvmsg() system call
	UDPv6RecvMsg ProbeFuncName = "kprobe__udpv6_recvmsg"
	// UDPv6RecvMsgPre470 traces the udpv6_recvmsg() system call on kernels prior to 4.7.0
	UDPv6RecvMsgPre470 ProbeFuncName = "kprobe__udpv6_recvmsg_pre_4_7_0"
	// UDPv6RecvMsgPre410 traces the udpv6_recvmsg() system call on kernels prior to 4.1.0
	UDPv6RecvMsgPre410 ProbeFuncName = "kprobe__udpv6_recvmsg_pre_4_1_0"
	// UDPv6RecvMsgPre5190 traces the udpv6_recvmsg() system call on kernels prior to 5.19.0
	UDPv6RecvMsgPre5190 ProbeFuncName = "kprobe__udpv6_recvmsg_pre_5_19_0"
	// UDPv6RecvMsgReturn traces the return value for the udpv6_recvmsg() system call
	UDPv6RecvMsgReturn ProbeFuncName = "kretprobe__udpv6_recvmsg"
	// UDPv6RecvMsgReturnPre470 traces the return value for the udpv6_recvmsg() system call prior to 4.7.0
	UDPv6RecvMsgReturnPre470 ProbeFuncName = "kretprobe__udpv6_recvmsg_pre_4_7_0"

	// SKBConsumeUDP traces skb_consume_udp()
	SKBConsumeUDP ProbeFuncName = "kprobe__skb_consume_udp"
	// SKBFreeDatagramLocked traces skb_free_datagram_locked()
	SKBFreeDatagramLocked ProbeFuncName = "kprobe__skb_free_datagram_locked"
	// UnderscoredSKBFreeDatagramLocked traces __skb_free_datagram_locked()
	UnderscoredSKBFreeDatagramLocked ProbeFuncName = "kprobe____skb_free_datagram_locked"

	// UDPDestroySock traces the udp_destroy_sock() function
	UDPDestroySock ProbeFuncName = "kprobe__udp_destroy_sock"
	// UDPDestroySockReturnRingbuffer traces the return of the udp_destroy_sock() system call using a ringbuffer
	UDPDestroySockReturnRingbuffer ProbeFuncName = "kretprobe__udp_destroy_sock_ringbuffer"
	// UDPDestroySockReturnPerfbuffer traces the return of the udp_destroy_sock() system call using a perf buffer
	UDPDestroySockReturnPerfbuffer ProbeFuncName = "kretprobe__udp_destroy_sock_perfbuffer"

	// UDPv6DestroySock traces the udpv6_destroy_sock() function
	UDPv6DestroySock ProbeFuncName = "kprobe__udpv6_destroy_sock"
	// UDPv6DestroySockReturnRingbuffer traces the return of the udpv6_destroy_sock() system call using a ringbuffer
	UDPv6DestroySockReturnRingbuffer ProbeFuncName = "kretprobe__udpv6_destroy_sock_ringbuffer"
	// UDPv6DestroySockReturnPerfbuffer traces the return of the udpv6_destroy_sock() system call using a perf buffer
	UDPv6DestroySockReturnPerfbuffer ProbeFuncName = "kretprobe__udpv6_destroy_sock_perfbuffer"

	// TCPRetransmit traces the params for the tcp_retransmit_skb() system call
	TCPRetransmit ProbeFuncName = "kprobe__tcp_retransmit_skb"
	// TCPRetransmitPre470 traces the params for the tcp_retransmit_skb() system call on kernel version < 4.7
	TCPRetransmitPre470 ProbeFuncName = "kprobe__tcp_retransmit_skb_pre_4_7_0"
	// TCPRetransmitRet traces the return value for the tcp_retransmit_skb() system call
	TCPRetransmitRet ProbeFuncName = "kretprobe__tcp_retransmit_skb"

	// InetCskAcceptReturn traces the return value for the inet_csk_accept syscall
	InetCskAcceptReturn ProbeFuncName = "kretprobe__inet_csk_accept"

	// InetBind is the kprobe of the bind() syscall for IPv4
	InetBind ProbeFuncName = "kprobe__inet_bind"
	// Inet6Bind is the kprobe of the bind() syscall for IPv6
	Inet6Bind ProbeFuncName = "kprobe__inet6_bind"

	// InetBindRet is the kretprobe of the bind() syscall for IPv4
	InetBindRet ProbeFuncName = "kretprobe__inet_bind"
	// Inet6BindRet is the kretprobe of the bind() syscall for IPv6
	Inet6BindRet ProbeFuncName = "kretprobe__inet6_bind"

	// SocketDNSFilter is the socket probe for dns
	SocketDNSFilter ProbeFuncName = "socket__dns_filter"

	// ConntrackHashInsert is the probe for new conntrack entries
	ConntrackHashInsert ProbeFuncName = "kprobe___nf_conntrack_hash_insert"

	// ConntrackFillInfo is the probe for dumping existing conntrack entries
	ConntrackFillInfo ProbeFuncName = "kprobe_ctnetlink_fill_info"
)

// BPFMapName stores the name of the BPF maps storing statistics and other info
type BPFMapName = string

// constants for the map names
const (
	//nolint:revive // TODO(NET) Fix revive linter
	ConnMap BPFMapName = "conn_stats"
	//nolint:revive // TODO(NET) Fix revive linter
	TCPStatsMap BPFMapName = "tcp_stats"
	//nolint:revive // TODO(NET) Fix revive linter
	TCPRetransmitsMap BPFMapName = "tcp_retransmits"
	//nolint:revive // TODO(NET) Fix revive linter
	TCPConnectSockPidMap BPFMapName = "tcp_ongoing_connect_pid"
	//nolint:revive // TODO(NET) Fix revive linter
	ConnCloseEventMap BPFMapName = "conn_close_event"
	//nolint:revive // TODO(NET) Fix revive linter
	TracerStatusMap BPFMapName = "tracer_status"
	//nolint:revive // TODO(NET) Fix revive linter
	ConntrackStatusMap BPFMapName = "conntrack_status"
	//nolint:revive // TODO(NET) Fix revive linter
	PortBindingsMap BPFMapName = "port_bindings"
	//nolint:revive // TODO(NET) Fix revive linter
	UDPPortBindingsMap BPFMapName = "udp_port_bindings"
	//nolint:revive // TODO(NET) Fix revive linter
	TelemetryMap BPFMapName = "telemetry"
	//nolint:revive // TODO(NET) Fix revive linter
	ConnCloseBatchMap BPFMapName = "conn_close_batch"
	//nolint:revive // TODO(NET) Fix revive linter
	ConntrackMap BPFMapName = "conntrack"
	//nolint:revive // TODO(NET) Fix revive linter
	ConntrackTelemetryMap BPFMapName = "conntrack_telemetry"
	//nolint:revive // TODO(NET) Fix revive linter
	TcpSendMsgArgsMap BPFMapName = "tcp_sendmsg_args"
	//nolint:revive // TODO(NET) Fix revive linter
	TcpSendPageArgsMap BPFMapName = "tcp_sendpage_args"
	//nolint:revive // TODO(NET) Fix revive linter
	UdpSendPageArgsMap BPFMapName = "udp_sendpage_args"
	//nolint:revive // TODO(NET) Fix revive linter
	IpMakeSkbArgsMap BPFMapName = "ip_make_skb_args"
	//nolint:revive // TODO(NET) Fix revive linter
	MapErrTelemetryMap BPFMapName = "map_err_telemetry_map"
	//nolint:revive // TODO(NET) Fix revive linter
	HelperErrTelemetryMap BPFMapName = "helper_err_telemetry_map"
	//nolint:revive // TODO(NET) Fix revive linter
	TcpRecvMsgArgsMap BPFMapName = "tcp_recvmsg_args"
	//nolint:revive // TODO(NET) Fix revive linter
	ProtocolClassificationBufMap BPFMapName = "classification_buf"
	//nolint:revive // TODO(NET) Fix revive linter
	KafkaClientIDBufMap BPFMapName = "kafka_client_id"
	//nolint:revive // TODO(NET) Fix revive linter
	KafkaTopicNameBufMap BPFMapName = "kafka_topic_name"
	//nolint:revive // TODO(NET) Fix revive linter
	ConnectionProtocolMap BPFMapName = "connection_protocol"
	//nolint:revive // TODO(NET) Fix revive linter
	ConnectionTupleToSocketSKBConnMap BPFMapName = "conn_tuple_to_socket_skb_conn_tuple"
	//nolint:revive // TODO(NET) Fix revive linter
	ClassificationProgsMap BPFMapName = "classification_progs"
	//nolint:revive // TODO(NET) Fix revive linter
	TCPCloseProgsMap BPFMapName = "tcp_close_progs"
	// ConnCloseProgsIndvMap is the map that stores the tail call programs to be called when a connection is closed
	// and it is not sent to userspace in a batch
	ConnCloseProgsIndvMap BPFMapName = "conn_close_individual_progs"
)
