// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package processor

import (
	"fmt"
	"testing"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/hook"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// BenchmarkProcessorHook measures the per-message cost of the logs pipeline
// hook tap point, comparing four hook modes:
//
//   - noop_hook: NoopHook (zero overhead baseline)
//   - 0sub: real Hook with zero subscribers (atomic fast-path, no snapshot built)
//   - 1sub: real Hook with one subscriber
//   - 5sub: real Hook with five subscribers
//
// Each iteration calls processMessage() once with a realistic log message.
// The benchmark calls processMessage() directly; no goroutines are started.
func BenchmarkProcessorHook(b *testing.B) {
	src := sources.NewLogSource("bench", &config.LogsConfig{Type: "file", Source: "bench"})
	content := []byte("2024-01-01T12:00:00Z INFO benchmark log message from service foo bar=baz")
	hostname, _ := hostnameinterface.NewMock(hostnameinterface.MockHostname("bench-host"))

	cases := []struct {
		name string
		h    hook.Hook[[]hook.LogSampleSnapshot]
	}{
		{"noop_hook", hook.NewNoopHook[[]hook.LogSampleSnapshot]()},
		{"0sub", makeLogHook(b, 0)},
		{"1sub", makeLogHook(b, 1)},
		{"5sub", makeLogHook(b, 5)},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			// processMessage() sends processed messages to outChan. It blocks if the
			// channel is full, which would stall the benchmark. Drain it in the background.
			outChan := make(chan *message.Message, 1000)
			stopDrain := make(chan struct{})
			go func() {
				for {
					select {
					case <-outChan:
					case <-stopDrain:
						return
					}
				}
			}()
			b.Cleanup(func() { close(stopDrain) })

			p := New(
				nil, nil, outChan,
				nil,
				JSONEncoder,
				&diagnostic.NoopMessageReceiver{},
				hostname,
				metrics.NewNoopPipelineMonitor("bench"),
				"bench",
				tc.h,
			)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				msg := message.NewMessageWithSource(content, "info", src, 0)
				p.processMessage(msg)
			}
			b.StopTimer()
			p.flushLogHookBatch()
		})
	}
}

// makeLogHook returns a real hook with n no-op subscribers.
// Channel sized to b.N + logHookBatchSize so sends never block during benchmark.
func makeLogHook(b *testing.B, n int) hook.Hook[[]hook.LogSampleSnapshot] {
	b.Helper()
	h := hook.NewHook[[]hook.LogSampleSnapshot]("bench-logs")
	for i := range n {
		h.Subscribe(
			fmt.Sprintf("bench-%d", i),
			func(_ []hook.LogSampleSnapshot) {},
			hook.WithBufferSize[[]hook.LogSampleSnapshot](b.N+logHookBatchSize+1),
		)
	}
	return h
}
