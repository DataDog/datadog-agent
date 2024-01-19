// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

//nolint:revive // TODO(NET) Fix revive linter
package fentry

import (
	"os"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf/features"

	ebpfCore "github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
)

func initManager(mgr *ebpftelemetry.Manager, closedHandler *ebpf.PerfHandler, ringHandlerTCP *ebpf.RingHandler, cfg *config.Config) {
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
		{Name: probes.MapErrTelemetryMap},
		{Name: probes.HelperErrTelemetryMap},
	}
	if features.HaveMapType(ebpfCore.RingBuf) == nil {
		rb := &manager.RingBuffer{
			Map: manager.Map{Name: probes.ConnCloseEventMapRing},
			RingBufferOptions: manager.RingBufferOptions{
				RingBufferSize: 16 * 256 * os.Getpagesize(),
				RecordGetter:   ringHandlerTCP.RecordGetter,
			},
		}
		mgr.RingBuffers = []*manager.RingBuffer{rb}
		ebpf.ReportRingBufferTelemetry(rb)
	} else {
		pm := &manager.PerfMap{
			Map: manager.Map{Name: probes.ConnCloseEventMapPerf},
			PerfMapOptions: manager.PerfMapOptions{
				PerfRingBufferSize: 8 * os.Getpagesize(),
				Watermark:          1,
				RecordHandler:      closedHandler.RecordHandler,
				LostHandler:        closedHandler.LostHandler,
				RecordGetter:       closedHandler.RecordGetter,
				TelemetryEnabled:   cfg.InternalTelemetryEnabled,
			},
		}
		mgr.PerfMaps = []*manager.PerfMap{pm}
		ebpf.ReportPerfMapTelemetry(pm)
	}

	for funcName := range programs {
		p := &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFFuncName: funcName,
				UID:          probeUID,
			},
		}
		mgr.Probes = append(mgr.Probes, p)
	}
}
