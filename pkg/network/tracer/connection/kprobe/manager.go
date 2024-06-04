// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kprobe

import (
	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/util"
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
	probes.TCPCloseFlushReturn,
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
	probes.UDPDestroySockReturn,
	probes.UDPv6DestroySock,
	probes.UDPv6DestroySockReturn,
	probes.InetBind,
	probes.Inet6Bind,
	probes.InetBindRet,
	probes.Inet6BindRet,
	probes.UDPSendPage,
	probes.UDPSendPageReturn,
}

func initManager(mgr *ddebpf.Manager, connCloseEventHandler ddebpf.EventHandler, failedConnsHandler ddebpf.EventHandler, runtimeTracer bool, cfg *config.Config) error {
	mgr.Maps = []*manager.Map{
		{Name: probes.ConnMap},
		{Name: probes.TCPStatsMap},
		{Name: probes.TCPConnectSockPidMap},
		{Name: probes.ClosedConnAlreadyFlushed},
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
		{Name: probes.MapErrTelemetryMap},
		{Name: probes.HelperErrTelemetryMap},
		{Name: probes.TCPRecvMsgArgsMap},
		{Name: probes.ClassificationProgsMap},
		{Name: probes.TCPCloseProgsMap},
	}
	util.SetupClosedConnHandler(connCloseEventHandler, mgr, cfg)
	if FailedConnectionsSupported(cfg) {
		util.SetupFailedConnHandler(failedConnsHandler, mgr, cfg)
	}

	for _, funcName := range mainProbes {
		p := &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: funcName,
				UID:          probeUID,
			},
		}
		mgr.Probes = append(mgr.Probes, p)
	}

	mgr.Probes = append(mgr.Probes,
		&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.SKBFreeDatagramLocked, UID: probeUID}},
		&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.UnderscoredSKBFreeDatagramLocked, UID: probeUID}},
		&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.SKBConsumeUDP, UID: probeUID}},
	)

	if !runtimeTracer {
		// the runtime compiled tracer has no need for separate probes targeting specific kernel versions, since it can
		// do that with #ifdefs inline. Thus, the following probes should only be declared as existing in the prebuilt
		// tracer.
		mgr.Probes = append(mgr.Probes,
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.TCPRetransmitPre470, UID: probeUID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.IPMakeSkbPre4180, UID: probeUID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.IP6MakeSkbPre470, UID: probeUID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.IP6MakeSkbPre5180, UID: probeUID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.UDPRecvMsgPre5190, UID: probeUID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.UDPv6RecvMsgPre5190, UID: probeUID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.UDPRecvMsgPre470, UID: probeUID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.UDPv6RecvMsgPre470, UID: probeUID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.UDPRecvMsgPre410, UID: probeUID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.UDPv6RecvMsgPre410, UID: probeUID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.UDPRecvMsgReturnPre470, UID: probeUID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.UDPv6RecvMsgReturnPre470, UID: probeUID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.TCPSendMsgPre410, UID: probeUID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.TCPRecvMsgPre410, UID: probeUID}},
			&manager.Probe{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: probes.TCPRecvMsgPre5190, UID: probeUID}},
		)
	}

	return nil
}
