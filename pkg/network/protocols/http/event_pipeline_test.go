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
	"net/http"
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/events"
	"github.com/stretchr/testify/require"
)

const (
	batchDataSize = 4096
)

type HTTPEventData struct {
	Method          uint8
	StatusCode      uint16
	RequestFragment []byte
}

// eBPFEventToBytes serializes the provided events into a byte array.
func eBPFEventToBytes(b *testing.B, events []EbpfEvent, numOfEventsInBatch int) [batchDataSize]int8 {
	var result [batchDataSize]int8
	var buffer bytes.Buffer

	// Serialize the events in the slice
	for i := 0; i < numOfEventsInBatch; i++ {
		// Use the two events alternately. Each iteration will use a different one.
		event := events[i%len(events)]
		require.NoError(b, binary.Write(&buffer, binary.LittleEndian, event))
	}

	serializedData := buffer.Bytes()
	// Ensure the serialized data fits into the result array
	require.LessOrEqualf(b, len(serializedData), len(result), "serialized data exceeds batchDataSize bytes")

	for i, b := range serializedData {
		result[i] = int8(b)
	}

	return result
}

// setupBenchmark sets up the benchmark environment by creating a consumer, protocol, and configuration.
func setupBenchmark(b *testing.B, c *config.Config, i, totalEventsCount, numOfEventsInBatch int, httpEvents []EbpfEvent, wg *sync.WaitGroup) (*events.Consumer[EbpfEvent], *protocol) {
	require.NotEmpty(b, httpEvents, "httpEvents slice is empty")

	program, err := events.NewEBPFProgram(c)
	require.NoError(b, err)

	httpTelemetry := NewTelemetry(fmt.Sprintf("http_%s_%d_%d", b.Name(), b.N, i))

	p := protocol{
		cfg:        c,
		telemetry:  httpTelemetry,
		statkeeper: NewStatkeeper(c, httpTelemetry, NewIncompleteBuffer(c, httpTelemetry)),
	}
	consumer, err := events.NewConsumer("test", program.Manager, p.processHTTP)
	require.NoError(b, err)

	// Using a wait group to ensure the goroutine finishes before the benchmark ends.
	wg.Add(1)
	go func() {
		defer wg.Done()
		generateMockEvents(b, c, consumer, httpEvents, numOfEventsInBatch, totalEventsCount)
	}()

	return consumer, &p
}

// generateMockEvents generates mock events to be used in the benchmark.
func generateMockEvents(b *testing.B, c *config.Config, consumer *events.Consumer[EbpfEvent], httpEvents []EbpfEvent, numOfEventsInBatch, totalEvents int) {
	// TODO: Determine if testing the CPU flow is necessary.
	mockBatch := events.Batch{
		Len:        uint16(numOfEventsInBatch),
		Cap:        uint16(numOfEventsInBatch),
		Event_size: uint16(unsafe.Sizeof(httpEvents[0])),
		Data:       eBPFEventToBytes(b, httpEvents, numOfEventsInBatch),
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
func createHTTPEvents(eventsData []HTTPEventData) []EbpfEvent {
	events := make([]EbpfEvent, len(eventsData))
	for i, data := range eventsData {
		events[i] = EbpfEvent{
			Tuple: ConnTuple{},
			Http: EbpfTx{
				Request_started:      1,
				Response_last_seen:   2,
				Request_method:       data.Method,
				Response_status_code: data.StatusCode,
				Request_fragment:     requestFragment(data.RequestFragment),
			},
		}
	}
	return events
}

// BenchmarkHTTPEventConsumer benchmarks the consumer with a large number of events to measure the performance.
func BenchmarkHTTPEventConsumer(b *testing.B) {
	// Set MemProfileRate to 1 in order to collect every allocation
	runtime.MemProfileRate = 1
	var wg sync.WaitGroup

	b.ReportAllocs()
	b.ResetTimer()

	testCases := []struct {
		name             string
		totalEventsCount int
		// Serialized data can't exceed batchDataSize bytes that why we can insert 14 events in a batch.
		numOfEventsInBatch int
		httpEvents         []EbpfEvent
	}{
		{"SmallBatch", 1000, 8, createHTTPEvents([]HTTPEventData{
			{Method: uint8(MethodGet), StatusCode: http.StatusOK, RequestFragment: []byte("GET / HTTP/1.1")},
			{Method: uint8(MethodPost), StatusCode: http.StatusCreated, RequestFragment: []byte("POST /create HTTP/1.1")},
		})},
		{"MediumBatch", 38000, 10, createHTTPEvents([]HTTPEventData{
			{Method: uint8(MethodGet), StatusCode: http.StatusOK, RequestFragment: []byte("GET / HTTP/1.1")},
			{Method: uint8(MethodPost), StatusCode: http.StatusCreated, RequestFragment: []byte("POST /create HTTP/1.1")},
		})},
		{"LargeBatch", 42000, 14, createHTTPEvents([]HTTPEventData{
			{Method: uint8(MethodGet), StatusCode: http.StatusOK, RequestFragment: []byte("GET / HTTP/1.1")},
			{Method: uint8(MethodPost), StatusCode: http.StatusCreated, RequestFragment: []byte("POST /create HTTP/1.1")},
		})},
		{"MaxEventsCount", 3150000, 14, createHTTPEvents([]HTTPEventData{
			{Method: uint8(MethodGet), StatusCode: http.StatusOK, RequestFragment: []byte("GET / HTTP/1.1")},
			{Method: uint8(MethodDelete), StatusCode: http.StatusAccepted, RequestFragment: []byte("DELETE /delete HTTP/1.1")},
		})},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				consumer, p := setupBenchmark(b, config.New(), i, tc.totalEventsCount, tc.numOfEventsInBatch, tc.httpEvents, &wg)

				consumer.Start()
				wg.Wait()

				require.Eventually(b, func() bool {
					if tc.totalEventsCount == int(p.telemetry.hits2XX.counterPlain.Get()) {
						b.Logf("USM summary: %s", p.telemetry.metricGroup.Summary())
						return true
					}
					return false
				}, 5*time.Second, 100*time.Millisecond)
			}
		})
	}
}
