// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

//nolint:revive // TODO(NET) Fix revive linter
package fentry

import (
	manager "github.com/DataDog/ebpf-manager"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/network/tracer/connection/kprobe"
)

func initManager(mgr *ebpftelemetry.Manager, connCloseEventHandler ebpf.EventHandler, cfg *config.Config) {
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
		{Name: probes.MapErrTelemetryMap},
		{Name: probes.HelperErrTelemetryMap},
	}
	switch handler := connCloseEventHandler.(type) {
	case *ebpf.RingBufferHandler:
		options := manager.RingBufferOptions{
			RecordGetter:     handler.RecordGetter,
			RecordHandler:    handler.RecordHandler,
			TelemetryEnabled: cfg.InternalTelemetryEnabled,
			RingBufferSize:   kprobe.ComputeDefaultClosedConnRingBufferSize(),
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
				PerfRingBufferSize: kprobe.ComputeDefaultClosedConnPerfBufferSize(),
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
