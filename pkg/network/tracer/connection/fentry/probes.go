// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package fentry

import (
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
)

const (
	// inetCskListenStop traces the inet_csk_listen_stop system call (called for both ipv4 and ipv6)
	inetCskListenStop = "fentry/inet_csk_listen_stop"

	// tcpConnect traces the connect() system call
	tcpConnect = "fentry/tcp_connect"
	// tcpFinishConnect traces tcp_finish_connect() kernel function. This is
	// used to know when a TCP connection switches to the ESTABLISHED state
	tcpFinishConnect = "fentry/tcp_finish_connect"

	// tcpSendMsgReturn traces the return value for the tcp_sendmsg() system call
	tcpSendMsgReturn = "fexit/tcp_sendmsg"

	// tcpSetState traces the tcp_set_state() kernel function
	tcpSetState = "fentry/tcp_set_state"

	// tcpRecvMsgReturn traces the return value for the tcp_recvmsg() system call
	tcpRecvMsgReturn = "fexit/tcp_recvmsg"
	// tcpClose traces the tcp_close() system call
	tcpClose = "fentry/tcp_close"
	// tcpCloseReturn traces the return of tcp_close() system call
	tcpCloseReturn = "fexit/tcp_close"

	// We use the following two probes for UDP
	udpRecvMsgReturn   = "fexit/udp_recvmsg"
	udpSendMsgReturn   = "fexit/udp_sendmsg"
	udpSendSkb         = "kprobe/udp_send_skb"
	udpv6RecvMsgReturn = "fexit/udpv6_recvmsg"
	udpv6SendMsgReturn = "fexit/udpv6_sendmsg"
	udpv6SendSkb       = "kprobe/udp_v6_send_skb"

	// udpDestroySock traces the udp_destroy_sock() function
	udpDestroySock = "fentry/udp_destroy_sock"
	// udpDestroySockReturn traces the return of the udp_destroy_sock() system call
	udpDestroySockReturn = "fexit/udp_destroy_sock"

	// tcpRetransmit traces the the tcp_retransmit_skb() kernel function
	tcpRetransmit = "fentry/tcp_retransmit_skb"

	// inetCskAcceptReturn traces the return value for the inet_csk_accept syscall
	inetCskAcceptReturn = "fexit/inet_csk_accept"

	// inetBindRet is the kretprobe of the bind() syscall for IPv4
	inetBindRet = "fexit/inet_bind"
	// inet6BindRet is the kretprobe of the bind() syscall for IPv6
	inet6BindRet = "fexit/inet6_bind"

	// sockFDLookupRet is the kretprobe used for mapping socket FDs to kernel sock structs
	sockFDLookupRet = "fexit/sockfd_lookup_light"

	// doSendfileRet is the kretprobe used to trace traffic via SENDFILE(2) syscall
	doSendfileRet = "fexit/do_sendfile"
)

var programs = map[string]string{
	string(probes.NetDevQueue):                    "tracepoint__net__net_dev_queue",
	string(probes.ProtocolClassifierSocketFilter): "socket__classifier",
	doSendfileRet:        "do_sendfile_exit", // TODO: available but sockfd_lookup_light not available on some kernels
	inet6BindRet:         "inet6_bind_exit",
	inetBindRet:          "inet_bind_exit",
	inetCskAcceptReturn:  "inet_csk_accept_exit",
	inetCskListenStop:    "inet_csk_listen_stop_enter",
	sockFDLookupRet:      "sockfd_lookup_light_exit", // TODO: not available on certain kernels, will have to one or more hooks to get equivalent functionality; affects do_sendfile and HTTPS monitoring (OpenSSL/GnuTLS/GoTLS)
	tcpRecvMsgReturn:     "tcp_recvmsg_exit",
	tcpClose:             "tcp_close",
	tcpCloseReturn:       "tcp_close_exit",
	tcpConnect:           "tcp_connect",
	tcpFinishConnect:     "tcp_finish_connect",
	tcpRetransmit:        "tcp_retransmit_skb",
	tcpSendMsgReturn:     "tcp_sendmsg_exit",
	tcpSetState:          "tcp_set_state",
	udpDestroySock:       "udp_destroy_sock",
	udpDestroySockReturn: "udp_destroy_sock_exit",
	udpRecvMsgReturn:     "udp_recvmsg_exit",
	udpSendMsgReturn:     "udp_sendmsg_exit",
	udpSendSkb:           "kprobe__udp_send_skb",
	udpv6RecvMsgReturn:   "udpv6_recvmsg_exit",
	udpv6SendMsgReturn:   "udpv6_sendmsg_exit",
	udpv6SendSkb:         "kprobe__udp_v6_send_skb",
}

func enableProgram(enabled map[string]string, name string) {
	if fn, ok := programs[name]; ok {
		enabled[name] = fn
	}
}

// enabledPrograms returns a map of probes that are enabled per config settings.
func enabledPrograms(c *config.Config) (map[string]string, error) {
	enabled := make(map[string]string, 0)
	if c.CollectTCPConns {
		if c.ClassificationSupported() {
			enableProgram(enabled, string(probes.ProtocolClassifierSocketFilter))
			enableProgram(enabled, string(probes.NetDevQueue))
		}

		enableProgram(enabled, tcpSendMsgReturn)
		enableProgram(enabled, tcpRecvMsgReturn)
		enableProgram(enabled, tcpClose)
		enableProgram(enabled, tcpCloseReturn)
		enableProgram(enabled, tcpConnect)
		enableProgram(enabled, tcpFinishConnect)
		enableProgram(enabled, inetCskAcceptReturn)
		enableProgram(enabled, inetCskListenStop)
		enableProgram(enabled, tcpSetState)
		enableProgram(enabled, tcpRetransmit)

		// TODO: see comments above on availability for these
		//       hooks
		// ksymPath := filepath.Join(c.ProcRoot, "kallsyms")
		// missing, err := ebpf.VerifyKernelFuncs(ksymPath, []string{"sockfd_lookup_light"})
		// if err == nil && len(missing) == 0 {
		// 	enableProgram(enabled, sockFDLookupRet)
		// 	enableProgram(enabled, doSendfileRet)
		// }
	}

	if c.CollectUDPConns {
		enableProgram(enabled, inetBindRet)
		enableProgram(enabled, udpDestroySock)
		enableProgram(enabled, udpDestroySockReturn)
		enableProgram(enabled, udpRecvMsgReturn)
		enableProgram(enabled, udpSendMsgReturn)
		enableProgram(enabled, udpSendSkb)

		if c.CollectIPv6Conns {
			enableProgram(enabled, inet6BindRet)
			enableProgram(enabled, udpv6RecvMsgReturn)
			enableProgram(enabled, udpv6SendMsgReturn)
			enableProgram(enabled, udpv6SendSkb)
		}
	}

	return enabled, nil
}
