// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && test

package events

import (
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/ringbuf"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	ebpftelemetry "github.com/DataDog/datadog-agent/pkg/ebpf/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/config"
)

// NewEBPFProgram creates a new test eBPF program.
func NewEBPFProgram(c *config.Config) (*ddebpf.Manager, error) {
	bc, err := bytecode.GetReader(c.BPFDir, "usm_events_test-debug.o")
	if err != nil {
		return nil, err
	}
	defer bc.Close()

	m := &manager.Manager{
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "tracepoint__syscalls__sys_enter_write",
				},
			},
		},
	}
	options := manager.Options{
		RemoveRlimit: true,
		ActivatedProbes: []manager.ProbesSelector{
			&manager.ProbeSelector{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					EBPFFuncName: "tracepoint__syscalls__sys_enter_write",
				},
			},
		},
		ConstantEditors: []manager.ConstantEditor{
			{
				Name:  "test_monitoring_enabled",
				Value: uint64(1),
			},
		},
	}
	ddEbpfManager := ddebpf.NewManager(m, "usm", &ebpftelemetry.ErrorsTelemetryModifier{})

	Configure(config.New(), "test", ddEbpfManager.Manager, &options)
	err = ddEbpfManager.InitWithOptions(bc, &options)
	if err != nil {
		return nil, err
	}

	return ddEbpfManager, nil
}

// RecordSample records a sample using the consumer Handler.
func RecordSample[V any](c *config.Config, consumer *Consumer[V], sampleData []byte) {
	// Ring buffers require kernel version 5.8.0 or higher, therefore, the Handler is chosen based on the kernel version.
	if c.EnableUSMRingBuffers && features.HaveMapType(ebpf.RingBuf) == nil {
		handler := consumer.handler.(*ddebpf.RingBufferHandler)
		handler.RecordHandler(&ringbuf.Record{
			RawSample: sampleData,
		}, nil, nil)
	} else {
		handler := consumer.handler.(*ddebpf.PerfHandler)
		handler.RecordHandler(&perf.Record{
			RawSample: sampleData,
		}, nil, nil)
	}
}
