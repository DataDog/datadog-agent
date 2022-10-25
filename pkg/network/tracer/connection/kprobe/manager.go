// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package kprobe

import (
	"os"
	"strings"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
)

const (
	// maxActive configures the maximum number of instances of the kretprobe-probed functions handled simultaneously.
	// This value should be enough for typical workloads (e.g. some amount of processes blocked on the `accept` syscall).
	maxActive = 128
)

var mainProbes = map[probes.ProbeName]string{
	probes.NetDevQueue:                    "tracepoint__net__net_dev_queue",
	probes.ProtocolClassifierSocketFilter: "socket__classifier",
	probes.TCPSendMsg:                     "kprobe__tcp_sendmsg",
	probes.TCPSendMsgReturn:               "kretprobe__tcp_sendmsg",
	probes.TCPRecvMsg:                     "kprobe__tcp_recvmsg",
	probes.TCPRecvMsgReturn:               "kretprobe__tcp_recvmsg",
	probes.TCPReadSock:                    "kprobe__tcp_read_sock",
	probes.TCPReadSockReturn:              "kretprobe__tcp_read_sock",
	probes.TCPClose:                       "kprobe__tcp_close",
	probes.TCPCloseReturn:                 "kretprobe__tcp_close",
	probes.TCPConnect:                     "kprobe__tcp_connect",
	probes.TCPFinishConnect:               "kprobe__tcp_finish_connect",
	probes.TCPSetState:                    "kprobe__tcp_set_state",
	probes.IPMakeSkb:                      "kprobe__ip_make_skb",
	probes.IPMakeSkbReturn:                "kretprobe__ip_make_skb",
	probes.IP6MakeSkb:                     "kprobe__ip6_make_skb",
	probes.IP6MakeSkbReturn:               "kretprobe__ip6_make_skb",
	probes.UDPRecvMsg:                     "kprobe__udp_recvmsg",
	probes.UDPRecvMsgReturn:               "kretprobe__udp_recvmsg",
	probes.UDPv6RecvMsg:                   "kprobe__udpv6_recvmsg",
	probes.UDPv6RecvMsgReturn:             "kretprobe__udpv6_recvmsg",
	probes.TCPRetransmit:                  "kprobe__tcp_retransmit_skb",
	probes.InetCskAcceptReturn:            "kretprobe__inet_csk_accept",
	probes.InetCskListenStop:              "kprobe__inet_csk_listen_stop",
	probes.UDPDestroySock:                 "kprobe__udp_destroy_sock",
	probes.UDPDestroySockReturn:           "kretprobe__udp_destroy_sock",
	probes.InetBind:                       "kprobe__inet_bind",
	probes.Inet6Bind:                      "kprobe__inet6_bind",
	probes.InetBindRet:                    "kretprobe__inet_bind",
	probes.Inet6BindRet:                   "kretprobe__inet6_bind",
	probes.SockFDLookup:                   "kprobe__sockfd_lookup_light",
	probes.SockFDLookupRet:                "kretprobe__sockfd_lookup_light",
	probes.DoSendfile:                     "kprobe__do_sendfile",
	probes.DoSendfileRet:                  "kretprobe__do_sendfile",
}

var altProbes = map[probes.ProbeName]string{
	probes.TCPRetransmitPre470:              "kprobe__tcp_retransmit_skb_pre_4_7_0",
	probes.IP6MakeSkbPre470:                 "kprobe__ip6_make_skb__pre_4_7_0",
	probes.UDPRecvMsgPre410:                 "kprobe__udp_recvmsg_pre_4_1_0",
	probes.UDPv6RecvMsgPre410:               "kprobe__udpv6_recvmsg_pre_4_1_0",
	probes.TCPSendMsgPre410:                 "kprobe__tcp_sendmsg__pre_4_1_0",
	probes.TCPRecvMsgPre410:                 "kprobe__tcp_recvmsg__pre_4_1_0",
	probes.SKBConsumeUDP:                    "kprobe__skb_consume_udp",
	probes.SKBFreeDatagramLocked:            "kprobe__skb_free_datagram_locked",
	probes.UnderscoredSKBFreeDatagramLocked: "kprobe____skb_free_datagram_locked",
}

func newManager(config *config.Config, closedHandler *ebpf.PerfHandler, runtimeTracer bool) *manager.Manager {
	mgr := &manager.Manager{
		Maps: []*manager.Map{
			{Name: string(probes.ConnMap)},
			{Name: string(probes.TCPStatsMap)},
			{Name: string(probes.TCPConnectSockPidMap)},
			{Name: string(probes.ConnCloseBatchMap)},
			{Name: "udp_recv_sock"},
			{Name: "udpv6_recv_sock"},
			{Name: string(probes.PortBindingsMap)},
			{Name: string(probes.UDPPortBindingsMap)},
			{Name: "pending_bind"},
			{Name: string(probes.TelemetryMap)},
			{Name: string(probes.SockByPidFDMap)},
			{Name: string(probes.PidFDBySockMap)},
			{Name: string(probes.SockFDLookupArgsMap)},
			{Name: string(probes.DoSendfileArgsMap)},
			{Name: string(probes.TcpSendMsgArgsMap)},
			{Name: string(probes.IpMakeSkbArgsMap)},
			{Name: string(probes.MapErrTelemetryMap)},
			{Name: string(probes.HelperErrTelemetryMap)},
			{Name: string(probes.TcpRecvMsgArgsMap)},
		},
		PerfMaps: []*manager.PerfMap{
			{
				Map: manager.Map{Name: string(probes.ConnCloseEventMap)},
				PerfMapOptions: manager.PerfMapOptions{
					PerfRingBufferSize: 8 * os.Getpagesize(),
					Watermark:          1,
					RecordHandler:      closedHandler.RecordHandler,
					LostHandler:        closedHandler.LostHandler,
					RecordGetter:       closedHandler.RecordGetter,
				},
			},
		},
	}

	for probeName, funcName := range mainProbes {
		p := &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  string(probeName),
				EBPFFuncName: funcName,
				UID:          probeUID,
			},
		}
		if strings.HasPrefix(funcName, "kretprobe") {
			p.KProbeMaxActive = maxActive
		}
		mgr.Probes = append(mgr.Probes, p)
	}

	if runtimeTracer {
		mgr.Probes = append(mgr.Probes,
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.SKBFreeDatagramLocked), EBPFFuncName: altProbes[probes.SKBFreeDatagramLocked], UID: probeUID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.UnderscoredSKBFreeDatagramLocked), EBPFFuncName: altProbes[probes.UnderscoredSKBFreeDatagramLocked], UID: probeUID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.SKBConsumeUDP), EBPFFuncName: altProbes[probes.SKBConsumeUDP], UID: probeUID}},
		)
	} else {
		// the runtime compiled tracer has no need for separate probes targeting specific kernel versions, since it can
		// do that with #ifdefs inline. Thus, the following probes should only be declared as existing in the prebuilt
		// tracer.
		mgr.Probes = append(mgr.Probes,
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.TCPRetransmitPre470), EBPFFuncName: altProbes[probes.TCPRetransmitPre470], UID: probeUID}, MatchFuncName: "^tcp_retransmit_skb$"},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.IP6MakeSkbPre470), EBPFFuncName: altProbes[probes.IP6MakeSkbPre470], UID: probeUID}, MatchFuncName: "^ip6_make_skb$"},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.UDPRecvMsgPre410), EBPFFuncName: altProbes[probes.UDPRecvMsgPre410], UID: probeUID}, MatchFuncName: "^udp_recvmsg$"},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.UDPv6RecvMsgPre410), EBPFFuncName: altProbes[probes.UDPv6RecvMsgPre410], UID: probeUID}, MatchFuncName: "^udpv6_recvmsg$"},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.TCPSendMsgPre410), EBPFFuncName: altProbes[probes.TCPSendMsgPre410], UID: probeUID}, MatchFuncName: "^tcp_sendmsg$"},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFSection: string(probes.TCPRecvMsgPre410), EBPFFuncName: altProbes[probes.TCPRecvMsgPre410], UID: probeUID}, MatchFuncName: "^tcp_recvmsg$"},
		)
	}

	return mgr
}
