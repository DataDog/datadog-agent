// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package fentry

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/util"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
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

	// tcpRecvMsgReturn traces the return value for the tcp_recvmsg() system call
	tcpRecvMsgReturn        = "tcp_recvmsg_exit"
	tcpRecvMsgPre5190Return = "tcp_recvmsg_exit_pre_5_19_0"
	// tcpClose traces the tcp_close() system call
	tcpClose = "tcp_close"
	// tcpCloseReturn traces the return of tcp_close() system call
	tcpCloseReturn = "tcp_close_exit"

	// We use the following two probes for UDP
	udpRecvMsg              = "udp_recvmsg"
	udpRecvMsgReturn        = "udp_recvmsg_exit"
	udpRecvMsgPre5190Return = "udp_recvmsg_exit_pre_5_19_0"
	udpSendMsgReturn        = "udp_sendmsg_exit"
	udpSendSkb              = "kprobe__udp_send_skb"

	skbFreeDatagramLocked   = "skb_free_datagram_locked"
	__skbFreeDatagramLocked = "__skb_free_datagram_locked" // nolint:revive
	skbConsumeUDP           = "skb_consume_udp"

	udpv6RecvMsg              = "udpv6_recvmsg"
	udpv6RecvMsgReturn        = "udpv6_recvmsg_exit"
	udpv6RecvMsgPre5190Return = "udpv6_recvmsg_exit_pre_5_19_0"
	udpv6SendMsgReturn        = "udpv6_sendmsg_exit"
	udpv6SendSkb              = "kprobe__udp_v6_send_skb"

	// udpDestroySock traces the udp_destroy_sock() function
	udpDestroySock = "udp_destroy_sock"
	// udpDestroySockReturn traces the return of the udp_destroy_sock() system call
	udpDestroySockReturn = "udp_destroy_sock_exit"

	udpv6DestroySock       = "udpv6_destroy_sock"
	udpv6DestroySockReturn = "udpv6_destroy_sock_exit"

	// tcpRetransmit traces the tcp_retransmit_skb() kernel function
	tcpRetransmit = "tcp_retransmit_skb"
	// tcpRetransmitRet traces the return of the tcp_retransmit_skb() system call
	tcpRetransmitRet = "tcp_retransmit_skb_exit"

	// tcpEnterLoss traces tcp_enter_loss() to count RTO loss events per connection.
	tcpEnterLoss = "kprobe__tcp_enter_loss"
	// tcpEnterRecovery traces tcp_enter_recovery() to count fast-recovery events per connection.
	tcpEnterRecovery = "kprobe__tcp_enter_recovery"
	// tcpSendProbe0 traces tcp_send_probe0() to count zero-window probe events per connection.
	tcpSendProbe0 = "kprobe__tcp_send_probe0"

	// inetCskAcceptReturn traces the return value for the inet_csk_accept syscall
	inetCskAcceptReturn = "inet_csk_accept_exit"

	// inetBind traces the bind() syscall for IPv4
	inetBind = "inet_bind_enter"
	// inet6Bind traces the bind() syscall for IPv6
	inet6Bind = "inet6_bind_enter"
	// inetBindRet traces the bind() syscall for IPv4
	inetBindRet = "inet_bind_exit"
	// inet6BindRet traces the bind() syscall for IPv6
	inet6BindRet = "inet6_bind_exit"
)

var programs = map[string]struct{}{
	inetBind:                  {},
	inet6Bind:                 {},
	inet6BindRet:              {},
	inetBindRet:               {},
	inetCskAcceptReturn:       {},
	inetCskListenStop:         {},
	tcpRecvMsgReturn:          {},
	tcpClose:                  {},
	tcpCloseReturn:            {},
	tcpConnect:                {},
	tcpFinishConnect:          {},
	tcpRetransmit:             {},
	tcpRetransmitRet:          {},
	tcpEnterLoss:              {},
	tcpEnterRecovery:          {},
	tcpSendProbe0:             {},
	tcpSendMsgReturn:          {},
	tcpSendPageReturn:         {},
	udpDestroySock:            {},
	udpDestroySockReturn:      {},
	udpRecvMsg:                {},
	udpRecvMsgReturn:          {},
	udpSendMsgReturn:          {},
	udpSendPageReturn:         {},
	udpSendSkb:                {},
	udpv6RecvMsg:              {},
	udpv6RecvMsgReturn:        {},
	udpv6SendMsgReturn:        {},
	udpv6SendSkb:              {},
	udpv6DestroySock:          {},
	udpv6DestroySockReturn:    {},
	skbFreeDatagramLocked:     {},
	__skbFreeDatagramLocked:   {},
	skbConsumeUDP:             {},
	tcpRecvMsgPre5190Return:   {},
	udpRecvMsgPre5190Return:   {},
	udpv6RecvMsgPre5190Return: {},
}

func enableProgram(enabled map[string]struct{}, name string) {
	if _, ok := programs[name]; ok {
		enabled[name] = struct{}{}
	}
}

// enabledPrograms returns a map of probes that are enabled per config settings.
func enabledPrograms(c *config.Config) (map[string]struct{}, error) {
	enabled := make(map[string]struct{}, 0)
	kv5190 := kernel.VersionCode(5, 19, 0)
	kv, err := kernel.HostVersion()
	if err != nil {
		return nil, err
	}

	hasSendPage := util.HasTCPSendPage(kv)

	if c.CollectTCPv4Conns || c.CollectTCPv6Conns {
		enableProgram(enabled, tcpSendMsgReturn)
		enableProgram(enabled, selectVersionBasedProbe(kv, tcpRecvMsgReturn, tcpRecvMsgPre5190Return, kv5190))
		enableProgram(enabled, tcpClose)
		enableProgram(enabled, tcpConnect)
		enableProgram(enabled, tcpFinishConnect)
		enableProgram(enabled, inetCskAcceptReturn)
		enableProgram(enabled, inetCskListenStop)
		enableProgram(enabled, tcpRetransmit)
		enableProgram(enabled, tcpRetransmitRet)
		enableProgram(enabled, tcpEnterLoss)
		enableProgram(enabled, tcpEnterRecovery)
		enableProgram(enabled, tcpSendProbe0)

		// TODO: see comments above on availability for these
		//       hooks
		// ksymPath := filepath.Join(c.ProcRoot, "kallsyms")
		// missing, err := ebpf.VerifyKernelFuncs(ksymPath, []string{"sockfd_lookup_light"})
		// if err == nil && len(missing) == 0 {
		// 	enableProgram(enabled, sockFDLookupRet)
		// }

		if c.CustomBatchingEnabled {
			enableProgram(enabled, tcpCloseReturn)
		}
		if hasSendPage {
			enableProgram(enabled, tcpSendPageReturn)
		}
	}

	if c.CollectUDPv4Conns {
		enableProgram(enabled, udpDestroySock)
		enableProgram(enabled, inetBind)
		enableProgram(enabled, inetBindRet)
		enableProgram(enabled, udpRecvMsg)
		enableProgram(enabled, selectVersionBasedProbe(kv, udpRecvMsgReturn, udpRecvMsgPre5190Return, kv5190))
		enableProgram(enabled, udpSendMsgReturn)
		enableProgram(enabled, udpSendSkb)

		if c.CustomBatchingEnabled {
			enableProgram(enabled, udpDestroySockReturn)
		}
	}

	if c.CollectUDPv6Conns {
		enableProgram(enabled, udpv6DestroySock)
		enableProgram(enabled, inet6Bind)
		enableProgram(enabled, inet6BindRet)
		enableProgram(enabled, udpv6RecvMsg)
		enableProgram(enabled, selectVersionBasedProbe(kv, udpv6RecvMsgReturn, udpv6RecvMsgPre5190Return, kv5190))
		enableProgram(enabled, udpv6SendMsgReturn)
		enableProgram(enabled, udpv6SendSkb)

		if c.CustomBatchingEnabled {
			enableProgram(enabled, udpv6DestroySockReturn)
		}
	}

	if c.CollectUDPv4Conns || c.CollectUDPv6Conns {
		if hasSendPage {
			enableProgram(enabled, udpSendPageReturn)
		}
		if err := enableAdvancedUDP(enabled); err != nil {
			return nil, err
		}
	}

	return enabled, nil
}

func enableAdvancedUDP(enabled map[string]struct{}) error {
	missing, err := ebpf.VerifyKernelFuncs("skb_consume_udp", "__skb_free_datagram_locked", "skb_free_datagram_locked")
	if err != nil {
		return fmt.Errorf("error verifying kernel function presence: %s", err)
	}
	if _, miss := missing["skb_consume_udp"]; !miss {
		enableProgram(enabled, skbConsumeUDP)
	} else if _, miss := missing["__skb_free_datagram_locked"]; !miss {
		enableProgram(enabled, __skbFreeDatagramLocked)
	} else if _, miss := missing["skb_free_datagram_locked"]; !miss {
		enableProgram(enabled, skbFreeDatagramLocked)
	} else {
		return errors.New("missing desired UDP receive kernel functions")
	}
	return nil
}

func selectVersionBasedProbe(kv kernel.Version, dfault string, versioned string, reqVer kernel.Version) string {
	if kv < reqVer {
		return versioned
	}
	return dfault
}
