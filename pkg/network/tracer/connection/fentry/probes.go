// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package fentry

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
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

	// tcpCleanupRBuf traces the tcp_cleanup_rbuf() system call
	tcpCleanupRBuf = "fentry/tcp_cleanup_rbuf"
	// tcpClose traces the tcp_close() system call
	tcpClose = "fentry/tcp_close"
	// tcpCloseReturn traces the return of tcp_close() system call
	tcpCloseReturn = "fexit/tcp_close"

	// We use the following two probes for UDP sends

	udpSendSkb = "fentry/udp_send_skb"
	// udpSendSkbReturn traces the return of udp_send_skb
	udpSendSkbReturn = "fexit/udp_send_skb"
	udpV6SendSkb     = "fentry/udp_v6_send_skb"
	// udpV6SendSkbReturn traces return of udp_v6_send_skb
	udpV6SendSkbReturn = "fexit/udp_v6_send_skb"

	// udpRecvMsgReturn traces the udp_recvmsg() system call
	udpRecvMsgReturn = "fexit/udp_recvmsg"

	// udpv6RecvMsgReturn traces the return value for the udpv6_recvmsg() system call
	udpv6RecvMsgReturn = "fexit/udpv6_recvmsg"

	// udpDestroySock traces the udp_destroy_sock() function
	udpDestroySock = "fentry/udp_destroy_sock"
	// udpDestroySockReturn traces the return of the udp_destroy_sock() system call
	udpDestroySockReturn = "fexit/udp_destroy_sock"

	// tcpRetransmit traces the return value for the tcp_retransmit_skb() system call
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
	doSendfileRet:        "do_sendfile_exit",
	inet6BindRet:         "inet6_bind_exit",
	inetBindRet:          "inet_bind_exit",
	inetCskAcceptReturn:  "inet_csk_accept_exit",
	inetCskListenStop:    "inet_csk_listen_stop_enter",
	sockFDLookupRet:      "sockfd_lookup_light_exit",
	tcpCleanupRBuf:       "tcp_cleanup_rbuf",
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
	udpSendSkb:           "udp_send_skb",
	udpSendSkbReturn:     "udp_send_skb_exit",
	udpV6SendSkb:         "udp_v6_send_skb",
	udpV6SendSkbReturn:   "udp_v6_send_skb_exit",
	udpv6RecvMsgReturn:   "udpv6_recvmsg_exit",
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
		enableProgram(enabled, tcpSendMsgReturn)
		enableProgram(enabled, tcpCleanupRBuf)
		enableProgram(enabled, tcpClose)
		enableProgram(enabled, tcpCloseReturn)
		enableProgram(enabled, tcpConnect)
		enableProgram(enabled, tcpFinishConnect)
		enableProgram(enabled, inetCskAcceptReturn)
		enableProgram(enabled, inetCskListenStop)
		enableProgram(enabled, tcpSetState)
		enableProgram(enabled, tcpRetransmit)

		ksymPath := filepath.Join(c.ProcRoot, "kallsyms")
		missing, err := ebpf.VerifyKernelFuncs(ksymPath, []string{"sockfd_lookup_light"})
		if err == nil && len(missing) == 0 {
			enableProgram(enabled, sockFDLookupRet)
			enableProgram(enabled, doSendfileRet)
		}
	}

	if c.CollectUDPConns {
		enableProgram(enabled, udpDestroySock)
		enableProgram(enabled, udpDestroySockReturn)
		enableProgram(enabled, udpSendSkb)
		enableProgram(enabled, udpSendSkbReturn)
		enableProgram(enabled, inetBindRet)

		if c.CollectIPv6Conns {
			enableProgram(enabled, udpV6SendSkb)
			enableProgram(enabled, udpV6SendSkbReturn)
			enableProgram(enabled, inet6BindRet)
		}

		enableProgram(enabled, udpRecvMsgReturn)
		if c.CollectIPv6Conns {
			enableProgram(enabled, udpv6RecvMsgReturn)
		}
	}

	return enabled, nil
}
