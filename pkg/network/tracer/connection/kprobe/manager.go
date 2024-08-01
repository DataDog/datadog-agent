// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kprobe

import (
	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/util/slices"
)

var mainProbes = []probes.ProbeFuncName{
	probes.NetDevQueue,
	probes.ProtocolClassifierEntrySocketFilter,
	probes.ProtocolClassifierQueuesSocketFilter,
	probes.ProtocolClassifierDBsSocketFilter,
	probes.ProtocolClassifierGRPCSocketFilter,
	probes.TCPSendMsg,
	probes.TCPSendMsgReturn,
	probes.TCPSendPage,
	probes.TCPSendPageReturn,
	probes.TCPRecvMsg,
	probes.TCPRecvMsgReturn,
	probes.TCPReadSock,
	probes.TCPReadSockReturn,
	probes.TCPClose,
	probes.TCPDone,
	probes.TCPCloseCleanProtocolsReturn,
	probes.TCPConnect,
	probes.TCPFinishConnect,
	probes.IPMakeSkb,
	probes.IPMakeSkbReturn,
	probes.IP6MakeSkb,
	probes.IP6MakeSkbReturn,
	probes.UDPRecvMsg,
	probes.UDPRecvMsgReturn,
	probes.UDPv6RecvMsg,
	probes.UDPv6RecvMsgReturn,
	probes.TCPRetransmit,
	probes.TCPRetransmitRet,
	probes.InetCskAcceptReturn,
	probes.InetCskListenStop,
	probes.UDPDestroySock,
	probes.UDPv6DestroySock,
	probes.InetBind,
	probes.Inet6Bind,
	probes.InetBindRet,
	probes.Inet6BindRet,
	probes.UDPSendPage,
	probes.UDPSendPageReturn,
}

var batchProbes = []probes.ProbeFuncName{
	probes.TCPDoneFlushReturn,
	probes.TCPCloseFlushReturn,
	probes.UDPDestroySockReturn,
	probes.UDPv6DestroySockReturn,
}

func initManager(mgr *ddebpf.Manager, runtimeTracer bool) error {
	mgr.Maps = []*manager.Map{
		{Name: probes.ConnMap},
		{Name: probes.TCPStatsMap},
		{Name: probes.TCPConnectSockPidMap},
		{Name: probes.ConnCloseFlushed},
		{Name: probes.ConnCloseBatchMap},
		{Name: "udp_recv_sock"},
		{Name: "udpv6_recv_sock"},
		{Name: probes.PortBindingsMap},
		{Name: probes.UDPPortBindingsMap},
		{Name: "pending_bind"},
		{Name: probes.TelemetryMap},
		{Name: probes.ConnectionProtocolMap},
		{Name: probes.TCPSendMsgArgsMap},
		{Name: probes.TCPSendPageArgsMap},
		{Name: probes.UDPSendPageArgsMap},
		{Name: probes.IPMakeSkbArgsMap},
		{Name: probes.TCPRecvMsgArgsMap},
		{Name: probes.ClassificationProgsMap},
		{Name: probes.TCPCloseProgsMap},
	}

	var funcNameToProbe = func(funcName probes.ProbeFuncName) *manager.Probe {
		return &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: funcName,
				UID:          probeUID,
			},
		}
	}

	mgr.Probes = append(mgr.Probes, slices.Map(mainProbes, funcNameToProbe)...)
	mgr.Probes = append(mgr.Probes, slices.Map(batchProbes, funcNameToProbe)...)
	mgr.Probes = append(mgr.Probes, slices.Map([]probes.ProbeFuncName{
		probes.SKBFreeDatagramLocked,
		probes.UnderscoredSKBFreeDatagramLocked,
		probes.SKBConsumeUDP,
	}, funcNameToProbe)...)

	if !runtimeTracer {
		// the runtime compiled tracer has no need for separate probes targeting specific kernel versions, since it can
		// do that with #ifdefs inline. Thus, the following probes should only be declared as existing in the prebuilt
		// tracer.
		mgr.Probes = append(mgr.Probes, slices.Map([]probes.ProbeFuncName{
			probes.TCPRetransmitPre470,
			probes.IPMakeSkbPre4180,
			probes.IP6MakeSkbPre470,
			probes.IP6MakeSkbPre5180,
			probes.UDPRecvMsgPre5190,
			probes.UDPv6RecvMsgPre5190,
			probes.UDPRecvMsgPre470,
			probes.UDPv6RecvMsgPre470,
			probes.UDPRecvMsgPre410,
			probes.UDPv6RecvMsgPre410,
			probes.UDPRecvMsgReturnPre470,
			probes.UDPv6RecvMsgReturnPre470,
			probes.TCPSendMsgPre410,
			probes.TCPRecvMsgPre410,
			probes.TCPRecvMsgPre5190,
		}, funcNameToProbe)...)
	}

	return nil
}
