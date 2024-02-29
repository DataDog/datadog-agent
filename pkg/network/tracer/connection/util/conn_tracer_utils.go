// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package util contains common helpers used in the creation of the closed connection event handler
package util

import (
	"os"

	manager "github.com/DataDog/ebpf-manager"
	cebpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/features"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ComputeDefaultClosedConnRingBufferSize is the default buffer size of the ring buffer for closed connection events.
// Must be a power of 2 and a multiple of the page size
func ComputeDefaultClosedConnRingBufferSize() int {
	numCPUs, err := utils.NumCPU()
	if err != nil {
		numCPUs = 1
	}
	pageSize := os.Getpagesize()
	if numCPUs <= 16 {
		return 8 * 8 * pageSize
	}
	return 8 * 16 * pageSize
}

// computeDefaultClosedConnPerfBufferSize is the default buffer size of the perf buffer for closed connection events.
// Must be a multiple of the page size
func computeDefaultClosedConnPerfBufferSize() int {
	return 8 * os.Getpagesize()
}

// SetupClosedConnHandler sets up the closed connection event handler
func SetupClosedConnHandler(connCloseEventHandler ebpf.EventHandler, mgr *ebpftelemetry.Manager, cfg *config.Config) {
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
				PerfRingBufferSize: computeDefaultClosedConnPerfBufferSize(),
				Watermark:          1,
				RecordHandler:      handler.RecordHandler,
				LostHandler:        handler.LostHandler,
				RecordGetter:       handler.RecordGetter,
				TelemetryEnabled:   cfg.InternalTelemetryEnabled,
			},
		}
		mgr.PerfMaps = []*manager.PerfMap{pm}
		ebpftelemetry.ReportPerfMapTelemetry(pm)
		helperCallRemover := ebpf.NewHelperCallRemover(asm.FnRingbufOutput)
		err := helperCallRemover.BeforeInit(mgr.Manager, nil)
		if err != nil {
			log.Error("Failed to remove helper calls from eBPF programs: ", err)
		}
	}
}

// RingBufferSupported returns true if ring buffer is supported on the kernel and enabled in the config
func RingBufferSupported(c *config.Config) bool {
	return (features.HaveMapType(cebpf.RingBuf) == nil) && c.RingbuffersEnabled
}

// AddBoolConst modifies the options to include a constant editor for a boolean value
func AddBoolConst(options *manager.Options, flag bool, name string) {
	val := uint64(1)
	if !flag {
		val = uint64(0)
	}

	options.ConstantEditors = append(options.ConstantEditors,
		manager.ConstantEditor{
			Name:  name,
			Value: val,
		},
	)
}
