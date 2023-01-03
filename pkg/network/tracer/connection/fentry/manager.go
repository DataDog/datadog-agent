// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package fentry

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	manager "github.com/DataDog/ebpf-manager"
)

func initManager(mgr *manager.Manager, config *config.Config, closedHandler *ebpf.PerfHandler) {
	mgr.Maps = []*manager.Map{
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
		{Name: string(probes.MapErrTelemetryMap)},
		{Name: string(probes.HelperErrTelemetryMap)},
	}
	mgr.PerfMaps = []*manager.PerfMap{
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
	}

	for sec, funcName := range programs {
		p := &manager.Probe{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				EBPFSection:  sec,
				EBPFFuncName: funcName,
				UID:          probeUID,
			},
		}
		mgr.Probes = append(mgr.Probes, p)
	}
}
