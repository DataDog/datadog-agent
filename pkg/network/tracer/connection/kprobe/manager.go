// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kprobe

import (
	"os"

	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
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
	probes.TCPCloseCleanProtocolsReturn,
	probes.TCPCloseFlushReturnRingbuffer,
	probes.TCPConnCloseEmitEventRingbuffer,
	probes.TCPCloseFlushReturnPerfbuffer,
	probes.TCPConnCloseEmitEventPerfbuffer,
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
	probes.UDPDestroySockReturnRingbuffer,
	probes.UDPDestroySockReturnPerfbuffer,
	probes.UDPv6DestroySock,
	probes.UDPv6DestroySockReturnRingbuffer,
	probes.UDPv6DestroySockReturnPerfbuffer,
	probes.InetBind,
	probes.Inet6Bind,
	probes.InetBindRet,
	probes.Inet6BindRet,
	probes.UDPSendPage,
	probes.UDPSendPageReturn,
}

func initManager(mgr *ebpftelemetry.Manager, connCloseEventHandler ebpf.EventHandler, runtimeTracer bool, cfg *config.Config) error {
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
		{Name: probes.ConnectionProtocolMap},
		{Name: probes.TcpSendMsgArgsMap},
		{Name: probes.TcpSendPageArgsMap},
		{Name: probes.UdpSendPageArgsMap},
		{Name: probes.IpMakeSkbArgsMap},
		{Name: probes.MapErrTelemetryMap},
		{Name: probes.HelperErrTelemetryMap},
		{Name: probes.TcpRecvMsgArgsMap},
		{Name: probes.ClassificationProgsMap},
		{Name: probes.TCPCloseProgsMap},
		{Name: probes.ConnCloseProgsIndvMap},
	}
	switch handler := connCloseEventHandler.(type) {
	case *ebpf.RingBufferHandler:
		options := manager.RingBufferOptions{
			RecordGetter:     handler.RecordGetter,
			RecordHandler:    handler.RecordHandler,
			TelemetryEnabled: cfg.InternalTelemetryEnabled,
			// RingBufferSize is not used yet by the manager, we use a map editor to set it in the tracer
			RingBufferSize: ComputeDefaultClosedConnRingBufferSize(),
		}
		rb := &manager.RingBuffer{
			Map:               manager.Map{Name: probes.ConnCloseEventMap},
			RingBufferOptions: options,
		}

		mgr.RingBuffers = []*manager.RingBuffer{rb}
		ebpftelemetry.ReportRingBufferTelemetry(rb)
	case *ebpf.PerfHandler:
		pm := &manager.PerfMap{
			Map: manager.Map{Name: probes.ConnCloseEventMap},
			PerfMapOptions: manager.PerfMapOptions{
				PerfRingBufferSize: ComputeDefaultClosedConnPerfBufferSize(),
				Watermark:          1,
				RecordHandler:      handler.RecordHandler,
				LostHandler:        handler.LostHandler,
				RecordGetter:       handler.RecordGetter,
				TelemetryEnabled:   cfg.InternalTelemetryEnabled,
			},
		}
		mgr.PerfMaps = []*manager.PerfMap{pm}
		ebpftelemetry.ReportPerfMapTelemetry(pm)
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

// ComputeDefaultClosedConnRingBufferSize is the default buffer size of the ring buffer for closed connection events.
// Must be a power of 2 and a multiple of the page size
func ComputeDefaultClosedConnRingBufferSize() int {
	numCPU, err := utils.NumCPU()
	pageSize := os.Getpagesize()
	if err != nil {
		numCPU = 1
	}

	if numCPU <= 16 {
		return 8 * 8 * pageSize
	}

	return 16 * 8 * pageSize
}

// ComputeDefaultClosedConnPerfBufferSize is the default buffer size of the perf buffer for closed connection events.
// Must be a multiple of the page size
func ComputeDefaultClosedConnPerfBufferSize() int {
	return 8 * os.Getpagesize()
}
