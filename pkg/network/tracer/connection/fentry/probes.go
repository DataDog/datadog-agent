// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package fentry

import (
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

const (
	// inetCskListenStop traces the inet_csk_listen_stop system call (called for both ipv4 and ipv6)
	inetCskListenStop = "inet_csk_listen_stop_enter"

	// tcpConnect traces the connect() system call
	tcpConnect = "tcp_connect"
	// tcpFinishConnect traces tcp_finish_connect() kernel function. This is
	// used to know when a TCP connection switches to the ESTABLISHED state
	tcpFinishConnect = "tcp_finish_connect"

	// tcpSendMsgReturn traces the return value for the tcp_sendmsg() system call
	tcpSendMsgReturn  = "tcp_sendmsg_exit"
	tcpSendPageReturn = "tcp_sendpage_exit"
	udpSendPageReturn = "udp_sendpage_exit"

	// tcpSetState traces the tcp_set_state() kernel function
	tcpSetState = "tcp_set_state"

	// tcpRecvMsgReturn traces the return value for the tcp_recvmsg() system call
	tcpRecvMsgReturn = "tcp_recvmsg_exit"
	// tcpClose traces the tcp_close() system call
	tcpClose = "tcp_close"
	// tcpCloseReturn traces the return of tcp_close() system call
	tcpCloseReturn = "tcp_close_exit"

	// We use the following two probes for UDP
	udpRecvMsgReturn   = "udp_recvmsg_exit"
	udpSendMsgReturn   = "udp_sendmsg_exit"
	udpSendSkb         = "kprobe__udp_send_skb"
	udpv6RecvMsgReturn = "udpv6_recvmsg_exit"
	udpv6SendMsgReturn = "udpv6_sendmsg_exit"
	udpv6SendSkb       = "kprobe__udp_v6_send_skb"

	// udpDestroySock traces the udp_destroy_sock() function
	udpDestroySock = "udp_destroy_sock"
	// udpDestroySockReturn traces the return of the udp_destroy_sock() system call
	udpDestroySockReturn = "udp_destroy_sock_exit"

	// tcpRetransmit traces the the tcp_retransmit_skb() kernel function
	tcpRetransmit = "tcp_retransmit_skb"
	// tcpRetransmitRet traces the return of the tcp_retransmit_skb() system call
	tcpRetransmitRet = "tcp_retransmit_skb_exit"

	// inetCskAcceptReturn traces the return value for the inet_csk_accept syscall
	inetCskAcceptReturn = "inet_csk_accept_exit"

	// inetBindRet is the kretprobe of the bind() syscall for IPv4
	inetBindRet = "inet_bind_exit"
	// inet6BindRet is the kretprobe of the bind() syscall for IPv6
	inet6BindRet = "inet6_bind_exit"

	// sockFDLookupRet is the kretprobe used for mapping socket FDs to kernel sock structs
	sockFDLookupRet = "sockfd_lookup_light_exit"
)

var programs = map[string]struct{}{
	inet6BindRet:         {},
	inetBindRet:          {},
	inetCskAcceptReturn:  {},
	inetCskListenStop:    {},
	sockFDLookupRet:      {}, // TODO: not available on certain kernels, will have to one or more hooks to get equivalent functionality; affects HTTPS monitoring (OpenSSL/GnuTLS/GoTLS)
	tcpRecvMsgReturn:     {},
	tcpClose:             {},
	tcpCloseReturn:       {},
	tcpConnect:           {},
	tcpFinishConnect:     {},
	tcpRetransmit:        {},
	tcpRetransmitRet:     {},
	tcpSendMsgReturn:     {},
	tcpSendPageReturn:    {},
	tcpSetState:          {},
	udpDestroySock:       {},
	udpDestroySockReturn: {},
	udpRecvMsgReturn:     {},
	udpSendMsgReturn:     {},
	udpSendPageReturn:    {},
	udpSendSkb:           {},
	udpv6RecvMsgReturn:   {},
	udpv6SendMsgReturn:   {},
	udpv6SendSkb:         {},
}

func enableProgram(enabled map[string]struct{}, name string) {
	if _, ok := programs[name]; ok {
		enabled[name] = struct{}{}
	}
}

// enabledPrograms returns a map of probes that are enabled per config settings.
func enabledPrograms(c *config.Config) (map[string]struct{}, error) {
	enabled := make(map[string]struct{}, 0)
	if c.CollectTCPConns {
		enableProgram(enabled, tcpSendMsgReturn)
		enableProgram(enabled, tcpSendPageReturn)
		enableProgram(enabled, tcpRecvMsgReturn)
		enableProgram(enabled, tcpClose)
		enableProgram(enabled, tcpCloseReturn)
		enableProgram(enabled, tcpConnect)
		enableProgram(enabled, tcpFinishConnect)
		enableProgram(enabled, inetCskAcceptReturn)
		enableProgram(enabled, inetCskListenStop)
		enableProgram(enabled, tcpSetState)
		enableProgram(enabled, tcpRetransmit)
		enableProgram(enabled, tcpRetransmitRet)

		// TODO: see comments above on availability for these
		//       hooks
		// ksymPath := filepath.Join(c.ProcRoot, "kallsyms")
		// missing, err := ebpf.VerifyKernelFuncs(ksymPath, []string{"sockfd_lookup_light"})
		// if err == nil && len(missing) == 0 {
		// 	enableProgram(enabled, sockFDLookupRet)
		// }
	}

	if c.CollectUDPConns {
		enableProgram(enabled, inetBindRet)
		enableProgram(enabled, udpDestroySock)
		enableProgram(enabled, udpDestroySockReturn)
		enableProgram(enabled, udpRecvMsgReturn)
		enableProgram(enabled, udpSendMsgReturn)
		enableProgram(enabled, udpSendSkb)
		enableProgram(enabled, udpSendPageReturn)

		if c.CollectIPv6Conns {
			enableProgram(enabled, inet6BindRet)
			enableProgram(enabled, udpv6RecvMsgReturn)
			enableProgram(enabled, udpv6SendMsgReturn)
			enableProgram(enabled, udpv6SendSkb)
		}
	}

	return enabled, nil
}
