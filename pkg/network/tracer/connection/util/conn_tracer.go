// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package util contains common helpers used in the creation of the closed connection event handler
package util

import (
	"math"
	"os"

	manager "github.com/DataDog/ebpf-manager"
	cebpf "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"

	"github.com/DataDog/datadog-agent/pkg/ebpf"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// toPowerOf2 converts a number to its nearest power of 2
func toPowerOf2(x int) int {
	log2 := math.Log2(float64(x))
	return int(math.Pow(2, math.Round(log2)))
}

// computeDefaultClosedConnRingBufferSize is the default buffer size of the ring buffer for closed connection events.
// Must be a power of 2 and a multiple of the page size
func computeDefaultClosedConnRingBufferSize() int {
	numCPUs, err := cebpf.PossibleCPU()
	if err != nil {
		numCPUs = 1
	}
	return 8 * toPowerOf2(numCPUs) * os.Getpagesize()
}

// computeDefaultClosedConnPerfBufferSize is the default buffer size of the perf buffer for closed connection events.
// Must be a multiple of the page size
func computeDefaultClosedConnPerfBufferSize() int {
	return 8 * os.Getpagesize()
}

// EnableRingbuffersViaMapEditor sets up the ring buffer for closed connection events via a map editor
func EnableRingbuffersViaMapEditor(mgrOpts *manager.Options) {
	mgrOpts.MapSpecEditors[probes.ConnCloseEventMap] = manager.MapSpecEditor{
		Type:       cebpf.RingBuf,
		MaxEntries: uint32(computeDefaultClosedConnRingBufferSize()),
		KeySize:    0,
		ValueSize:  0,
		EditorFlag: manager.EditType | manager.EditMaxEntries | manager.EditKeyValue,
	}
}

// SetupClosedConnHandler sets up the closed connection event handler
func SetupClosedConnHandler(connCloseEventHandler ebpf.EventHandler, mgr *ebpf.Manager, cfg *config.Config) {
	switch handler := connCloseEventHandler.(type) {
	case *ebpf.RingBufferHandler:
		options := manager.RingBufferOptions{
			RecordGetter:     handler.RecordGetter,
			RecordHandler:    handler.RecordHandler,
			TelemetryEnabled: cfg.InternalTelemetryEnabled,
			// RingBufferSize is not used yet by the manager, we use a map editor to set it in the tracer
			RingBufferSize: computeDefaultClosedConnRingBufferSize(),
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

// AddBoolConst modifies the options to include a constant editor for a boolean value
func AddBoolConst(options *manager.Options, name string, flag bool) {
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
