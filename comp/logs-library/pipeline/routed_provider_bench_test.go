// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pipeline

import (
	"context"
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
	compressioncommon "github.com/DataDog/datadog-agent/pkg/util/compression"
)

func BenchmarkDefaultHTTPDelivery(b *testing.B) { benchmarkHTTPDelivery(b, false, 1) }
func BenchmarkRoutedHTTPDelivery(b *testing.B)  { benchmarkHTTPDelivery(b, true, 1) }

func BenchmarkDefaultHTTPBatchedDelivery(b *testing.B) { benchmarkHTTPDelivery(b, false, 1000) }
func BenchmarkRoutedHTTPBatchedDelivery(b *testing.B)  { benchmarkHTTPDelivery(b, true, 1000) }

func benchmarkHTTPDelivery(b *testing.B, routed bool, batchSize int) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		requests.Add(1)
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
	batchWait := 10 * time.Millisecond
	if batchSize > 1 {
		batchWait = 100 * time.Millisecond
	}
	endpoints := config.NewEndpointsWithBatchSettings(main, nil, false, true, batchWait, 1, batchSize, 1024*1024, 2)
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
			Generation:                func(*message.Message) (uint64, error) { return 1, nil },
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
	input := provider.NextPipelineChan()
	if routed {
		input, err = provider.(*RoutedProvider).SourceInput("file:benchmark", 1)
		if err != nil {
			b.Fatal(err)
		}
	}
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
		input <- msg
	}
	select {
	case <-finished:
	case <-time.After(30 * time.Second):
		b.Fatal("delivery benchmark timed out")
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)/float64(requests.Load()), "records/request")
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	b.ReportMetric(float64(int64(after.HeapAlloc)-int64(before.HeapAlloc))/1024, "KiB/live-delta")
	if routed {
		if err := provider.(*RoutedProvider).RetireSource(context.Background(), "file:benchmark", 1); err != nil {
			b.Fatal(err)
		}
	}
	provider.Stop()
	ctx.Stop()
	close(sink.ch)
	sinkDone.Wait()
}

func BenchmarkRoutedFanoutAvailableGroups(b *testing.B) {
	a, c := make(chan *message.Message, 1), make(chan *message.Message, 1)
	m := &message.Message{}
	offers := []channelOffer[*message.Message]{{input: a, value: m}, {input: c, value: m}}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		offerToChannels(context.Background(), offers)
		<-a
		<-c
	}
}

func BenchmarkEndpointRoutingSparse1PctGzip(b *testing.B) {
	benchmarkEndpointRoutingGzip(b, 100, 0)
}

func BenchmarkEndpointRoutingOverlap50PctGzip(b *testing.B) {
	benchmarkEndpointRoutingGzip(b, 2, 0)
}

func BenchmarkEndpointRoutingSlowOptionalGzip(b *testing.B) {
	benchmarkEndpointRoutingGzip(b, 2, 15*time.Millisecond)
}

func benchmarkEndpointRoutingGzip(b *testing.B, optionalEvery int, optionalDelay time.Duration) {
	var primaryReqs, optionalReqs atomic.Int64
	primarySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		primaryReqs.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer primarySrv.Close()

	optionalSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
		if optionalDelay > 0 {
			select {
			case <-time.After(optionalDelay):
			case <-r.Context().Done():
				return
			}
		}
		optionalReqs.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer optionalSrv.Close()

	makeEP := func(srvURL, key string) config.Endpoint {
		parsed, err := url.Parse(srvURL)
		if err != nil {
			b.Fatal(err)
		}
		port, err := strconv.Atoi(parsed.Port())
		if err != nil {
			b.Fatal(err)
		}
		ep := config.NewEndpoint(key, "", parsed.Hostname(), port, "", false)
		ep.UseCompression = true
		ep.CompressionKind = compressioncommon.GzipKind
		ep.CompressionLevel = 6
		return ep
	}

	us1 := makeEP(primarySrv.URL, "us1-bench")
	us5 := makeEP(optionalSrv.URL, "us5-bench")
	batchTemplate := config.NewEndpointsWithBatchSettings(us1, nil, false, true, 50*time.Millisecond, 2, 1000, 1024*1024, 64)
	policy, err := routing.NewEndpointPolicyFromTargets([]routing.EndpointTarget{
		{ID: "us1", Endpoint: us1, Reliable: true, Evaluator: routing.MatchAllEvaluator{}},
		{
			ID:       "us5",
			Endpoint: us5,
			Reliable: false,
			Evaluator: routing.EvaluatorFunc(func(v routing.RecordView) (bool, error) {
				return v.Status() == message.StatusNotice, nil
			}),
		},
	}, batchTemplate, 4, nil)
	if err != nil {
		b.Fatal(err)
	}

	cfg := configmock.New(b)
	cfg.SetInTest("logs_config.payload_channel_size", 64)
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

	provider, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		EndpointPolicy:            policy,
		DestinationsContext:       ctx,
		Config:                    cfg,
		Compression:               realGzipCompression{},
		Secrets:                   secretsnoopimpl.NewComponent().Comp,
	})
	if err != nil {
		b.Fatal(err)
	}
	provider.Start()
	input, err := provider.SourceInput("file:bench-endpoint", 1)
	if err != nil {
		b.Fatal(err)
	}
	source := sources.NewLogSource("benchmark", &config.LogsConfig{Type: config.StringChannelType})
	payloadBytes := []byte(`{"service":"checkout","msg":"order processed successfully for customer","region":"us-east-1","code":200}`)

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		status := message.StatusInfo
		if i%optionalEvery == 0 {
			status = message.StatusNotice
		}
		msg := message.NewMessageWithSource(payloadBytes, status, source, time.Now().UnixNano())
		msg.Origin.Identifier = "file:bench-endpoint"
		msg.Origin.Offset = strconv.Itoa(i + 1)
		input <- msg
	}
	select {
	case <-finished:
	case <-time.After(30 * time.Second):
		b.Fatal("endpoint routing benchmark timed out")
	}
	b.StopTimer()
	if reqs := primaryReqs.Load(); reqs > 0 {
		b.ReportMetric(float64(b.N)/float64(reqs), "primary-rec/req")
	}
	b.ReportMetric(float64(optionalReqs.Load()), "opt-reqs")
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	b.ReportMetric(float64(int64(after.HeapAlloc)-int64(before.HeapAlloc))/1024, "KiB/live-delta")
	_ = provider.RetireSource(context.Background(), "file:bench-endpoint", 1)
	provider.Stop()
	ctx.Stop()
	close(sink.ch)
	sinkDone.Wait()
}
