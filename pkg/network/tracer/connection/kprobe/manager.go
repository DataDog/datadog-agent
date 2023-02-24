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

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	manager "github.com/DataDog/ebpf-manager"
)

const (
	// maxActive configures the maximum number of instances of the kretprobe-probed functions handled simultaneously.
	// This value should be enough for typical workloads (e.g. some amount of processes blocked on the `accept` syscall).
	maxActive = 128
)

var mainProbes = []probes.ProbeFuncName{
	probes.NetDevQueue,
	probes.ProtocolClassifierEntrySocketFilter,
	probes.ProtocolClassifierQueuesSocketFilter,
	probes.ProtocolClassifierDBsSocketFilter,
	probes.TCPSendMsg,
	probes.TCPSendMsgReturn,
	probes.TCPRecvMsg,
	probes.TCPRecvMsgReturn,
	probes.TCPReadSock,
	probes.TCPReadSockReturn,
	probes.TCPClose,
	probes.TCPCloseReturn,
	probes.TCPConnect,
	probes.TCPFinishConnect,
	probes.TCPSetState,
	probes.IPMakeSkb,
	probes.IPMakeSkbReturn,
	probes.IP6MakeSkb,
	probes.IP6MakeSkbReturn,
	probes.UDPRecvMsg,
	probes.UDPRecvMsgReturn,
	probes.UDPv6RecvMsg,
	probes.UDPv6RecvMsgReturn,
	probes.TCPRetransmit,
	probes.InetCskAcceptReturn,
	probes.InetCskListenStop,
	probes.UDPDestroySock,
	probes.UDPDestroySockReturn,
	probes.InetBind,
	probes.Inet6Bind,
	probes.InetBindRet,
	probes.Inet6BindRet,
	probes.SockFDLookup,
	probes.SockFDLookupRet,
	probes.DoSendfile,
	probes.DoSendfileRet,
}

func initManager(mgr *manager.Manager, config *config.Config, closedHandler *ebpf.PerfHandler, runtimeTracer bool) {
	mgr.Maps = []*manager.Map{
		{Name: probes.ConnMap},
		{Name: probes.TCPStatsMap},
		{Name: probes.TCPConnectSockPidMap},
		{Name: probes.ConnCloseBatchMap},
		{Name: "udp_recv_sock"},
		{Name: "udpv6_recv_sock"},
		{Name: probes.PortBindingsMap},
		{Name: probes.UDPPortBindingsMap},
		{Name: "pending_bind"},
		{Name: probes.TelemetryMap},
		{Name: probes.SockByPidFDMap},
		{Name: probes.PidFDBySockMap},
		{Name: probes.SockFDLookupArgsMap},
		{Name: probes.DoSendfileArgsMap},
		{Name: probes.TcpSendMsgArgsMap},
		{Name: probes.IpMakeSkbArgsMap},
		{Name: probes.MapErrTelemetryMap},
		{Name: probes.HelperErrTelemetryMap},
		{Name: probes.TcpRecvMsgArgsMap},
		{Name: probes.ClassificationProgsMap},
	}
	mgr.PerfMaps = []*manager.PerfMap{
		{
			Map: manager.Map{Name: probes.ConnCloseEventMap},
			PerfMapOptions: manager.PerfMapOptions{
				PerfRingBufferSize: 8 * os.Getpagesize(),
				Watermark:          1,
				RecordHandler:      closedHandler.RecordHandler,
				LostHandler:        closedHandler.LostHandler,
				RecordGetter:       closedHandler.RecordGetter,
			},
		},
	}
	for _, funcName := range mainProbes {
		p := &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: funcName,
				UID:          probes.UID,
			},
		}
		if strings.HasPrefix(funcName, "kretprobe") {
			p.KProbeMaxActive = maxActive
		}
		mgr.Probes = append(mgr.Probes, p)
	}

	if runtimeTracer {
		mgr.Probes = append(mgr.Probes,
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.SKBFreeDatagramLocked, UID: probes.UID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.UnderscoredSKBFreeDatagramLocked, UID: probes.UID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.SKBConsumeUDP, UID: probes.UID}},
		)
	} else {
		// the runtime compiled tracer has no need for separate probes targeting specific kernel versions, since it can
		// do that with #ifdefs inline. Thus, the following probes should only be declared as existing in the prebuilt
		// tracer.
		mgr.Probes = append(mgr.Probes,
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.TCPRetransmitPre470, UID: probes.UID}, MatchFuncName: "^tcp_retransmit_skb$"},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.IP6MakeSkbPre470, UID: probes.UID}, MatchFuncName: "^ip6_make_skb$"},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.UDPRecvMsgPre410, UID: probes.UID}, MatchFuncName: "^udp_recvmsg$"},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.UDPv6RecvMsgPre410, UID: probes.UID}, MatchFuncName: "^udpv6_recvmsg$"},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.TCPSendMsgPre410, UID: probes.UID}, MatchFuncName: "^tcp_sendmsg$"},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.TCPRecvMsgPre410, UID: probes.UID}, MatchFuncName: "^tcp_recvmsg$"},
		)
	}
}
