// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"testing"

	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

func createBenchBatch(maxBatchSize, maxContentSize int, compressionKind string) *batch {
	return makeBatch(
		compressionfx.NewMockCompressor().NewCompressor(compressionKind, 1),
		maxBatchSize,
		maxContentSize,
		"bench",
		NewMockServerlessMeta(false),
		metrics.NewNoopPipelineMonitor(""),
		metrics.NewNoopPipelineMonitor("").MakeUtilizationMonitor("bench", "bench"),
		"bench",
	)
}

// BenchmarkBatchAddMessage measures the cost of adding a single message to the batch
func BenchmarkBatchAddMessage(b *testing.B) {
	batch := createBenchBatch(1000, 1024*1024, compression.NoneKind)
	msg := message.NewMessage([]byte("test message content for benchmarking"), nil, "", 0)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch.addMessage(msg)
		// Reset periodically to avoid buffer overflow
		if i%100 == 99 {
			batch.resetBatch()
		}
	}
}

// BenchmarkBatchFlush measures the cost of flushing a batch
func BenchmarkBatchFlush(b *testing.B) {
	output := make(chan *message.Payload, b.N)
	batch := createBenchBatch(100, 1024*1024, compression.NoneKind)
	msg := message.NewMessage([]byte("test message content for benchmarking"), nil, "", 0)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Add some messages
		for j := 0; j < 10; j++ {
			batch.addMessage(msg)
		}
		// Flush the batch
		batch.flushBuffer(output)
	}
}

// BenchmarkBatchFlushWithCompression measures the cost of flushing with zstd compression
func BenchmarkBatchFlushWithCompression(b *testing.B) {
	output := make(chan *message.Payload, b.N)
	batch := createBenchBatch(100, 1024*1024, compression.ZstdKind)
	msg := message.NewMessage([]byte("test message content for benchmarking with zstd compression enabled"), nil, "", 0)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Add some messages
		for j := 0; j < 10; j++ {
			batch.addMessage(msg)
		}
		// Flush the batch
		batch.flushBuffer(output)
	}
}

// BenchmarkBatchProcessMessage measures the cost of processing messages through the full pipeline
func BenchmarkBatchProcessMessage(b *testing.B) {
	output := make(chan *message.Payload, b.N)
	batch := createBenchBatch(10, 1024*1024, compression.NoneKind)
	msg := message.NewMessage([]byte("test message content for benchmarking"), nil, "", 0)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch.processMessage(msg, output)
	}
}

// BenchmarkBatchResetBatch measures the cost of resetting the batch
func BenchmarkBatchResetBatch(b *testing.B) {
	batch := createBenchBatch(100, 1024*1024, compression.NoneKind)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch.resetBatch()
	}
}

// BenchmarkBatchResetBatchWithCompression measures the cost of resetting with zstd compression
func BenchmarkBatchResetBatchWithCompression(b *testing.B) {
	batch := createBenchBatch(100, 1024*1024, compression.ZstdKind)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch.resetBatch()
	}
}

// BenchmarkBatchThroughput measures messages per second with realistic batch sizes
func BenchmarkBatchThroughput(b *testing.B) {
	output := make(chan *message.Payload, 1000)
	batch := createBenchBatch(100, 256*1024, compression.ZstdKind)
	// Realistic log message size ~200 bytes
	msg := message.NewMessage([]byte(`{"timestamp":"2024-01-15T10:30:00Z","level":"info","service":"myapp","message":"Processing request from client 192.168.1.100 with params {\"user_id\":12345,\"action\":\"view\"}"}`), nil, "", 0)

	// Drain the output channel in background
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-output:
			case <-done:
				return
			}
		}
	}()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch.processMessage(msg, output)
	}
	b.StopTimer()
	close(done)
}

// BenchmarkBatchFlushCycle measures a complete fill-flush cycle
func BenchmarkBatchFlushCycle(b *testing.B) {
	output := make(chan *message.Payload, b.N)
	batch := createBenchBatch(50, 64*1024, compression.ZstdKind)
	msg := message.NewMessage([]byte("test message content for benchmarking the complete flush cycle"), nil, "", 0)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Fill the batch
		for j := 0; j < 50; j++ {
			batch.addMessage(msg)
		}
		// Flush
		batch.flushBuffer(output)
	}
}
