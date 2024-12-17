// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"bytes"
	"encoding/binary"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/require"
)

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

// setupBenchmark sets up the benchmark environment by creating a consumer, protocol, and configuration.
func setupBenchmark(b *testing.B, c *config.Config, totalEventsCount int) (*events.Consumer[EbpfEvent], *protocol, *manager.Manager) {
	const numOfEventsInBatch = 14

	program, err := events.NewEBPFProgram(c)
	require.NoError(b, err)

	httpTelemetry := NewTelemetry("http")

	p := protocol{
		cfg:        c,
		telemetry:  httpTelemetry,
		statkeeper: NewStatkeeper(c, httpTelemetry, NewIncompleteBuffer(c, httpTelemetry)),
	}
	consumer, err := events.NewConsumer("test", program, p.processHTTP)
	require.NoError(b, err)

	go generateMockEvents(b, c, consumer, numOfEventsInBatch, totalEventsCount)

	return consumer, &p, program
}

// generateMockEvents generates mock events to be used in the benchmark.
func generateMockEvents(b *testing.B, c *config.Config, consumer *events.Consumer[EbpfEvent], numOfEventsInBatch, totalEvents int) {
	httpEvents := createHTTPEvents()
	require.NotEmpty(b, httpEvents, "httpEvents slice is empty")

	mockBatch := events.Batch{
		Len:        uint16(numOfEventsInBatch),
		Cap:        uint16(numOfEventsInBatch),
		Event_size: uint16(unsafe.Sizeof(httpEvents[0])),
		Data:       structToBytes(b, httpEvents, numOfEventsInBatch),
	}

	for i := 0; i < totalEvents/numOfEventsInBatch; i++ {
		mockBatch.Idx = uint64(i)
		var buf bytes.Buffer
		require.NoError(b, binary.Write(&buf, binary.LittleEndian, &mockBatch))
		events.RecordSample(c, consumer, buf.Bytes())
		buf.Reset()
	}
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

// BenchmarkHTTPEventConsumer benchmarks the consumer with a large number of events to measure the performance.
func BenchmarkHTTPEventConsumer(b *testing.B) {
	const TotalEventsCount = 42000
	// Set MemProfileRate to 1 in order to collect every allocation
	runtime.MemProfileRate = 1

	consumer, p, program := setupBenchmark(b, config.New(), TotalEventsCount)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		consumer.Start()
		require.Eventually(b, func() bool {
			return TotalEventsCount == int(p.telemetry.hits2XX.counterPlain.Get())
		}, 5*time.Second, 100*time.Millisecond)
	}

	b.Cleanup(func() {
		b.Logf("USM summary: %s", p.telemetry.metricGroup.Summary())
		p.telemetry.hits2XX.counterPlain.Reset()
		program.Stop(manager.CleanAll)
	})
}
