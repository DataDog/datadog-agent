// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	manager "github.com/DataDog/ebpf-manager"
)

func newTestEBPFProgram(c *config.Config) (*manager.Manager, error) {
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
		RLimit: &unix.Rlimit{
			Cur: math.MaxUint64,
			Max: math.MaxUint64,
		},
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

	events.Configure(config.New(), "test", m, &options)
	err = m.InitWithOptions(bc, options)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func structToBytes(b *testing.B, data EbpfEvent, numEvents int) [4096]int8 {
	var result [4096]int8
	var buffer bytes.Buffer

	// Serialize the event multiple times
	for i := 0; i < numEvents; i++ {
		require.NoError(b, binary.Write(&buffer, binary.LittleEndian, data))
	}

	serializedData := buffer.Bytes()
	// Ensure the serialized data fits into the result array
	require.LessOrEqualf(b, len(serializedData), len(result), "serialized data exceeds 4096 bytes")

	// Copy serialized data to the result array
	for i, b := range serializedData {
		result[i] = int8(b)
	}

	return result
}

// BenchmarkConsumer benchmarks the consumer with a large number of events to measure the performance.
func BenchmarkFlow(b *testing.B) {
	// serialized data can't exceed 4096 bytes that why we can insert 14 events in a batch
	const numOfEventsInBatch = 14
	const TotalEventsCount = 140
	c := config.New()

	program, err := newTestEBPFProgram(c)
	require.NoError(b, err)
	httpReq := "GET /etc/os-release HTTP/1.1\nHost: localhost:8001\nUser-Agent: curl/7.81.0\nAccept: */*"

	httpEvent := EbpfEvent{
		Tuple: ConnTuple{},
		Http: EbpfTx{
			Request_method:       1,
			Response_status_code: 200,
			Request_fragment:     requestFragment([]byte(httpReq)),
		},
	}

	httpTelemetry := NewTelemetry("http")

	p := protocol{
		cfg:        c,
		telemetry:  httpTelemetry,
		statkeeper: NewStatkeeper(c, httpTelemetry, NewIncompleteBuffer(c, httpTelemetry)),
	}
	consumer, err := events.NewConsumer("test", program, p.processHTTP)
	require.NoError(b, err)

	mockBatch := events.Batch{
		Len:        uint16(numOfEventsInBatch),
		Cap:        uint16(numOfEventsInBatch),
		Event_size: uint16(unsafe.Sizeof(httpEvent)),
		Data:       structToBytes(b, httpEvent, numOfEventsInBatch),
	}

	b.ReportAllocs()
	runtime.GC()
	b.ResetTimer()

	b.N = TotalEventsCount / numOfEventsInBatch
	for i := 0; i < b.N; i++ {
		fmt.Printf("Iteration %d\n", i)

		mockBatch.Idx = uint64(i)
		var buf bytes.Buffer
		require.NoError(b, binary.Write(&buf, binary.LittleEndian, &mockBatch))

		events.RecordSample(c, consumer, buf.Bytes())

		consumer.Start()
		require.Eventually(b, func() bool {
			// Ensure all events were processed by hits 2XX counter for each iteration
			return (i+1)*numOfEventsInBatch == int(httpTelemetry.hits2XX.counterPlain.Get())
		}, 50*time.Second, 1000*time.Millisecond)
	}
	b.Cleanup(func() {
		program.Stop(manager.CleanAll)
	})
}
