// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

//nolint:revive // TODO(NET) Fix revive linter
package fentry

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	errtelemetry "github.com/DataDog/datadog-agent/pkg/network/telemetry"
	manager "github.com/DataDog/ebpf-manager"
)

func initManager(mgr *errtelemetry.Manager, closedHandler *ebpf.PerfHandler, cfg *config.Config) {
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
	pm := &manager.PerfMap{
		Map: manager.Map{Name: probes.ConnCloseEventMap},
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
