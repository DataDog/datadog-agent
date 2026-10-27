// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pipeline

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	secretsnoopimpl "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/diagnostic"
	"github.com/DataDog/datadog-agent/comp/logs-library/routing"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	compressionfx "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func BenchmarkDefaultHTTPDelivery(b *testing.B) { benchmarkHTTPDelivery(b, false) }
func BenchmarkRoutedHTTPDelivery(b *testing.B)  { benchmarkHTTPDelivery(b, true) }

func benchmarkHTTPDelivery(b *testing.B, routed bool) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	parsed, err := url.Parse(server.URL)
	if err != nil {
		b.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		b.Fatal(err)
	}
	main := config.NewEndpoint("benchmark-key", "", parsed.Hostname(), port, "", false)
	endpoints := config.NewEndpointsWithBatchSettings(main, nil, false, true, 10*time.Millisecond, 1, 1, 1024*1024, 2)
	cfg := configmock.New(b)
	cfg.SetInTest("logs_config.routing.max_groups", 4)
	cfg.SetInTest("logs_config.payload_channel_size", 64)
	cfg.SetInTest("logs_config.message_channel_size", 64)
	ctx := client.NewDestinationsContext()
	ctx.Start()
	sink := &routedTestSink{ch: make(chan *message.Payload, 256)}
	finished := make(chan struct{})
	var count atomic.Int64
	var once sync.Once
	var sinkDone sync.WaitGroup
	sinkDone.Add(1)
	go func() {
		defer sinkDone.Done()
		for payload := range sink.ch {
			if count.Add(payload.Count()) >= int64(b.N) {
				once.Do(func() { close(finished) })
			}
		}
	}()
	var provider Provider
	if routed {
		provider, err = NewRoutedProvider(RoutedOptions{
			NumberOfPipelines:         1,
			Sink:                      sink,
			DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
			Groups:                    []routing.Group{{ID: "main", Endpoints: endpoints}},
			Resolve: func(*message.Message) (routing.Selection, error) {
				return routing.Selection{Groups: []string{"main"}, Generation: 1}, nil
			},
			DestinationsContext: ctx,
			Config:              cfg,
			Compression:         compressionfx.NewMockCompressor(),
			Secrets:             secretsnoopimpl.NewComponent().Comp,
		})
		if err != nil {
			b.Fatal(err)
		}
	} else {
		provider = NewProvider(1, sink, &diagnostic.BufferedMessageReceiver{}, nil, nil, endpoints, ctx, nil, nil, cfg, compressionfx.NewMockCompressor(), false, false, secretsnoopimpl.NewComponent().Comp)
	}
	provider.Start()
	source := sources.NewLogSource("benchmark", &config.LogsConfig{Type: config.StringChannelType})
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		msg := message.NewMessageWithSource([]byte("benchmark record"), "info", source, time.Now().UnixNano())
		msg.Origin.Identifier = "file:benchmark"
		msg.Origin.Offset = strconv.Itoa(i + 1)
		provider.NextPipelineChan() <- msg
	}
	select {
	case <-finished:
	case <-time.After(30 * time.Second):
		b.Fatal("delivery benchmark timed out")
	}
	b.StopTimer()
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	b.ReportMetric(float64(int64(after.HeapAlloc)-int64(before.HeapAlloc))/1024, "KiB/live-delta")
	provider.Stop()
	ctx.Stop()
	close(sink.ch)
	sinkDone.Wait()
}
