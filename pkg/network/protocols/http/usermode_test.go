// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"runtime/pprof"
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

// structToBytes serializes the provided events into a byte array.
func structToBytes(b *testing.B, events []EbpfEvent, numOfEventsInBatch int) [4096]int8 {
	var result [4096]int8
	var buffer bytes.Buffer

	// Serialize the events in the slice
	for i := 0; i < numOfEventsInBatch; i++ {
		// Use modulo to cycle through the provided events
		event := events[i%len(events)]
		require.NoError(b, binary.Write(&buffer, binary.LittleEndian, event))
	}

	serializedData := buffer.Bytes()
	// Ensure the serialized data fits into the result array
	require.LessOrEqualf(b, len(serializedData), len(result), "serialized data exceeds 4096 bytes")

	for i, b := range serializedData {
		result[i] = int8(b)
	}

	return result
}

// createHTTPEvents creates a slice of HTTP events to be used in the benchmark.
func createHTTPEvents() []EbpfEvent {
	httpReq1 := "GET /etc/os-release HTTP/1.1\nHost: localhost:8001\nUser-Agent: curl/7.81.0\nAccept: */*"
	httpReq2 := "POST /submit HTTP/1.1\nHost: localhost:8002\nUser-Agent: curl/7.81.0\nContent-Type: application/json\n\n{\"key\":\"value\"}"

	// To perform the benchmark with incomplete data, we can remove the request_started field from the EbpfTx struct.
	return []EbpfEvent{
		{
			Tuple: ConnTuple{},
			Http: EbpfTx{
				Request_started:      1,
				Response_last_seen:   2,
				Request_method:       1,
				Response_status_code: 200,
				Request_fragment:     requestFragment([]byte(httpReq1)),
			},
		},
		{
			Tuple: ConnTuple{},
			Http: EbpfTx{
				Request_started:      1,
				Response_last_seen:   2,
				Request_method:       2,
				Response_status_code: 200,
				Request_fragment:     requestFragment([]byte(httpReq2)),
			},
		},
	}
}

// BenchmarkConsumer benchmarks the consumer with a large number of events to measure the performance.
func BenchmarkFlow(b *testing.B) {
	// Serialized data can't exceed 4096 bytes that why we can insert 14 events in a batch.
	const numOfEventsInBatch = 14
	// The capacity of the data chanel is 100 batches, so we can process 1400 events in total.
	const TotalEventsCount = 1400
	c := config.New()

	program, err := newTestEBPFProgram(c)
	require.NoError(b, err)

	httpEvents := createHTTPEvents()
	require.NotEmpty(b, httpEvents, "httpEvents slice is empty")

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
		Event_size: uint16(unsafe.Sizeof(httpEvents[0])),
		Data:       structToBytes(b, httpEvents, numOfEventsInBatch),
	}

	for i := 0; i < TotalEventsCount/numOfEventsInBatch; i++ {
		mockBatch.Idx = uint64(i)
		var buf bytes.Buffer
		require.NoError(b, binary.Write(&buf, binary.LittleEndian, &mockBatch))
		events.RecordSample(c, consumer, buf.Bytes())
		buf.Reset()
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		consumer.Start()

		f, err := os.Create("/tmp/consumer_mem.prof")
		if err != nil {
			b.Logf("failed to create memory profile: %v", err)
		} else {
			pprof.Lookup("goroutine").WriteTo(f, 0)
			f.Close()
		}

		require.Eventually(b, func() bool {
			// Ensure all events were processed by hits 2XX counter for each iteration
			return TotalEventsCount == int(httpTelemetry.hits2XX.counterPlain.Get())
		}, 5*time.Second, 1000*time.Millisecond)
	}
	b.Cleanup(func() {
		program.Stop(manager.CleanAll)
	})
}
