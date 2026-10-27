// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pipeline

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
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
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
)

type routedTestSink struct{ ch chan *message.Payload }

func (s *routedTestSink) Channel() chan *message.Payload { return s.ch }

type routedRequest struct {
	endpoint string
	key      string
	message  string
}

type countingCompression struct{ closes atomic.Int32 }

func (c *countingCompression) NewCompressor(kind string, level int) compression.Compressor {
	return &countingCompressor{Compressor: selector.NewCompressor(kind, level), owner: c}
}

type countingCompressor struct {
	compression.Compressor
	owner *countingCompression
}

func (c *countingCompressor) NewStreamCompressor(output *bytes.Buffer) compression.StreamCompressor {
	return &countingStream{StreamCompressor: c.Compressor.NewStreamCompressor(output), owner: c.owner}
}

type countingStream struct {
	compression.StreamCompressor
	owner *countingCompression
}

func (s *countingStream) Close() error {
	s.owner.closes.Add(1)
	return s.StreamCompressor.Close()
}

func routedTestEndpoint(t *testing.T, id, key string, requests chan<- routedRequest) (*httptest.Server, *config.Endpoints) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read %s body: %v", id, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var records []struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(body, &records); err != nil {
			t.Errorf("decode %s payload: %v", id, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, record := range records {
			requests <- routedRequest{endpoint: id, key: r.Header.Get("DD-API-KEY"), message: record.Message}
		}
		w.WriteHeader(http.StatusOK)
	}))
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatal(err)
	}
	main := config.NewEndpoint(key, "", parsed.Hostname(), port, "", false)
	endpoints := config.NewEndpointsWithBatchSettings(main, nil, false, true, 10*time.Millisecond, 1, 1, 1024*1024, 2)
	return server, endpoints
}

func TestRoutedProviderExactHTTPRecipientsAndCheckpoints(t *testing.T) {
	requests := make(chan routedRequest, 8)
	serverA, endpointA := routedTestEndpoint(t, "a", "key-a", requests)
	defer serverA.Close()
	serverB, endpointB := routedTestEndpoint(t, "b", "key-b", requests)
	defer serverB.Close()
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.routing.max_groups", 4)
	cfg.SetInTest("logs_config.payload_channel_size", 2)
	ctx := client.NewDestinationsContext()
	sink := &routedTestSink{}
	provider, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines:         2,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Groups:                    []routing.Group{{ID: "a", Endpoints: endpointA}, {ID: "b", Endpoints: endpointB}},
		Resolve: func(m *message.Message) (routing.Selection, error) {
			content := string(m.GetContent())
			switch content {
			case "a":
				return routing.Selection{Groups: []string{"a"}, Generation: 1}, nil
			case "b":
				return routing.Selection{Groups: []string{"b"}, Generation: 1}, nil
			default:
				return routing.Selection{Groups: []string{"a", "b", "a"}, Generation: 1}, nil
			}
		},
		DestinationsContext: ctx,
		Config:              cfg,
		Compression:         compressionfx.NewMockCompressor(),
		Secrets:             secretsnoopimpl.NewComponent().Comp,
	})
	if err != nil {
		t.Fatal(err)
	}
	sink.ch = make(chan *message.Payload, 8)
	ctx.Start()
	provider.Start()
	defer provider.Stop()
	source := sources.NewLogSource("test", &config.LogsConfig{Type: config.StringChannelType})
	for i, content := range []string{"a", "b", "both"} {
		msg := message.NewMessageWithSource([]byte(content), "info", source, time.Now().UnixNano())
		msg.Origin.Identifier = "file:test"
		msg.Origin.Offset = strconv.Itoa(i + 1)
		provider.NextPipelineChan() <- msg
	}
	want := map[routedRequest]int{
		{"a", "key-a", "a"}:    1,
		{"b", "key-b", "b"}:    1,
		{"a", "key-a", "both"}: 1,
		{"b", "key-b", "both"}: 1,
	}
	for range 4 {
		select {
		case got := <-requests:
			want[got]--
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for routed HTTP request")
		}
	}
	for request, count := range want {
		if count != 0 {
			t.Errorf("recipient %+v: remaining count %d", request, count)
		}
	}
	for i := 1; i <= 3; i++ {
		select {
		case checkpoint := <-sink.ch:
			if got := checkpoint.MessageMetas[0].Origin.Offset; got != strconv.Itoa(i) {
				t.Fatalf("checkpoint %d advanced to %q", i, got)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("checkpoint %d did not arrive", i)
		}
	}
}

func TestRoutedProviderSharedStreamCompressesOnceAndSendsBothGroups(t *testing.T) {
	type received struct {
		key  string
		body []byte
	}
	requests := make(chan received, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		encoded, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- received{key: r.Header.Get("DD-API-KEY"), body: encoded}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(parsed.Port())
	makeGroup := func(id, key string) routing.Group {
		main := config.NewEndpoint(key, "", parsed.Hostname(), port, "", false)
		main.UseCompression = true
		main.CompressionKind = compression.GzipKind
		main.CompressionLevel = gzip.BestSpeed
		return routing.Group{ID: id, Stream: "identical", Endpoints: config.NewEndpointsWithBatchSettings(main, nil, false, true, 10*time.Millisecond, 1, 1, 1024*1024, 2)}
	}
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.routing.max_groups", 4)
	cfg.SetInTest("logs_config.payload_channel_size", 2)
	ctx := client.NewDestinationsContext()
	ctx.Start()
	sink := &routedTestSink{ch: make(chan *message.Payload, 2)}
	compressor := &countingCompression{}
	provider, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Groups:                    []routing.Group{makeGroup("a", "key-a"), makeGroup("b", "key-b")},
		Resolve: func(*message.Message) (routing.Selection, error) {
			return routing.Selection{Groups: []string{"a", "b"}, Generation: 1}, nil
		},
		DestinationsContext: ctx,
		Config:              cfg,
		Compression:         compressor,
		Secrets:             secretsnoopimpl.NewComponent().Comp,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider.Start()
	defer provider.Stop()
	source := sources.NewLogSource("test", &config.LogsConfig{Type: config.StringChannelType})
	msg := message.NewMessageWithSource([]byte("shared"), "info", source, time.Now().UnixNano())
	msg.Origin.Identifier = "file:shared"
	msg.Origin.Offset = "1"
	provider.NextPipelineChan() <- msg
	next := func() received {
		select {
		case request := <-requests:
			return request
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for shared stream request")
			return received{}
		}
	}
	first := next()
	second := next()
	if first.key == second.key || !bytes.Equal(first.body, second.body) {
		t.Fatalf("shared stream must make two requests with distinct credentials and identical bytes: %q, %q", first.key, second.key)
	}
	if (first.key != "key-a" && first.key != "key-b") || (second.key != "key-a" && second.key != "key-b") {
		t.Fatalf("unexpected credentials: %q, %q", first.key, second.key)
	}
	decoded, err := gzip.NewReader(bytes.NewReader(first.body))
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(decoded)
	if err != nil {
		t.Fatal(err)
	}
	var records []struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(content, &records); err != nil || len(records) != 1 || records[0].Message != "shared" {
		t.Fatalf("decoded payload: %q, %v", content, err)
	}
	if got := compressor.closes.Load(); got != 1 {
		t.Fatalf("compression operations: got %d, want 1", got)
	}
	select {
	case checkpoint := <-sink.ch:
		if checkpoint.MessageMetas[0].Origin.Offset != "1" {
			t.Fatal("wrong checkpoint")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shared groups did not complete")
	}
}

func TestRoutedProviderLocalLossesReleaseOnlyTheirRecords(t *testing.T) {
	requests := make(chan routedRequest, 2)
	server, endpoints := routedTestEndpoint(t, "a", "key-a", requests)
	defer server.Close()
	endpoints.BatchMaxContentSize = 1
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.routing.max_groups", 4)
	cfg.SetInTest("logs_config.payload_channel_size", 2)
	ctx := client.NewDestinationsContext()
	ctx.Start()
	sink := &routedTestSink{ch: make(chan *message.Payload, 2)}
	provider, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		ProcessingRules:           []*config.ProcessingRule{{Type: config.ExcludeAtMatch, Regex: regexp.MustCompile("^excluded$")}},
		Groups:                    []routing.Group{{ID: "a", Endpoints: endpoints}},
		Resolve: func(*message.Message) (routing.Selection, error) {
			return routing.Selection{Groups: []string{"a"}, Generation: 1}, nil
		},
		DestinationsContext: ctx,
		Config:              cfg,
		Compression:         compressionfx.NewMockCompressor(),
		Secrets:             secretsnoopimpl.NewComponent().Comp,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider.Start()
	defer provider.Stop()
	source := sources.NewLogSource("test", &config.LogsConfig{Type: config.StringChannelType})
	for i, content := range []string{"excluded", "too-large"} {
		msg := message.NewMessageWithSource([]byte(content), "info", source, time.Now().UnixNano())
		msg.Origin.Identifier = "file:loss"
		msg.Origin.Offset = strconv.Itoa(i + 1)
		provider.NextPipelineChan() <- msg
	}
	for i := 1; i <= 2; i++ {
		select {
		case checkpoint := <-sink.ch:
			if checkpoint.MessageMetas[0].Origin.Offset != strconv.Itoa(i) {
				t.Fatalf("local loss checkpoint %d has offset %q", i, checkpoint.MessageMetas[0].Origin.Offset)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("local loss %d did not complete", i)
		}
	}
	select {
	case request := <-requests:
		t.Fatalf("locally excluded record reached %s", request.endpoint)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestRoutedProviderInvalidSelectionNeverAdvancesLaterCheckpoint(t *testing.T) {
	requests := make(chan routedRequest, 2)
	server, endpoints := routedTestEndpoint(t, "a", "key-a", requests)
	defer server.Close()
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.routing.max_groups", 4)
	cfg.SetInTest("logs_config.payload_channel_size", 2)
	ctx := client.NewDestinationsContext()
	ctx.Start()
	sink := &routedTestSink{ch: make(chan *message.Payload, 2)}
	errorsReported := make(chan error, 2)
	provider, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Groups:                    []routing.Group{{ID: "a", Endpoints: endpoints}},
		Resolve: func(m *message.Message) (routing.Selection, error) {
			if string(m.GetContent()) == "invalid" {
				return routing.Selection{Groups: []string{"unknown"}, Generation: 1}, nil
			}
			return routing.Selection{Groups: []string{"a"}, Generation: 1}, nil
		},
		OnError:             func(err error) { errorsReported <- err },
		DestinationsContext: ctx,
		Config:              cfg,
		Compression:         compressionfx.NewMockCompressor(),
		Secrets:             secretsnoopimpl.NewComponent().Comp,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider.Start()
	defer provider.Stop()
	source := sources.NewLogSource("test", &config.LogsConfig{Type: config.StringChannelType})
	for i, content := range []string{"invalid", "valid"} {
		msg := message.NewMessageWithSource([]byte(content), "info", source, time.Now().UnixNano())
		msg.Origin.Identifier = "file:invalid"
		msg.Origin.Offset = strconv.Itoa(i + 1)
		provider.NextPipelineChan() <- msg
	}
	select {
	case err := <-errorsReported:
		if !errors.Is(err, routing.ErrInvalidSelection) {
			t.Fatalf("unexpected selection error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("invalid selection was not reported")
	}
	select {
	case request := <-requests:
		if request.message != "valid" {
			t.Fatalf("invalid selection reached endpoint: %+v", request)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("valid later record did not reach endpoint")
	}
	select {
	case checkpoint := <-sink.ch:
		t.Fatalf("invalid selection was implicitly acknowledged at %s", checkpoint.MessageMetas[0].Origin.Offset)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestRoutedProviderCancellationDoesNotAcknowledgeRetryingGroup(t *testing.T) {
	requests := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case requests <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(parsed.Port())
	main := config.NewEndpoint("test-key", "", parsed.Hostname(), port, "", false)
	endpoints := config.NewEndpointsWithBatchSettings(main, nil, false, true, 10*time.Millisecond, 1, 1, 1024*1024, 2)
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.routing.max_groups", 4)
	cfg.SetInTest("logs_config.payload_channel_size", 2)
	ctx := client.NewDestinationsContext()
	ctx.Start()
	sink := &routedTestSink{ch: make(chan *message.Payload, 2)}
	provider, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Groups:                    []routing.Group{{ID: "a", Endpoints: endpoints}},
		Resolve: func(*message.Message) (routing.Selection, error) {
			return routing.Selection{Groups: []string{"a"}, Generation: 1}, nil
		},
		DestinationsContext: ctx,
		Config:              cfg,
		Compression:         compressionfx.NewMockCompressor(),
		Secrets:             secretsnoopimpl.NewComponent().Comp,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider.Start()
	source := sources.NewLogSource("test", &config.LogsConfig{Type: config.StringChannelType})
	msg := message.NewMessageWithSource([]byte("retrying"), "info", source, time.Now().UnixNano())
	msg.Origin.Identifier = "file:retrying"
	msg.Origin.Offset = "1"
	provider.NextPipelineChan() <- msg
	select {
	case <-requests:
	case <-time.After(5 * time.Second):
		t.Fatal("retrying group was never attempted")
	}
	ctx.Stop()
	stopped := make(chan struct{})
	go func() {
		provider.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("routed shutdown blocked on retrying group")
	}
	select {
	case checkpoint := <-sink.ch:
		t.Fatalf("cancelled retry advanced checkpoint to %s", checkpoint.MessageMetas[0].Origin.Offset)
	default:
	}
}

func TestRoutedProviderHealthyGroupProgressesDuringOtherGroupRetry(t *testing.T) {
	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer unavailable.Close()
	parsedA, _ := url.Parse(unavailable.URL)
	portA, _ := strconv.Atoi(parsedA.Port())
	mainA := config.NewEndpoint("key-a", "", parsedA.Hostname(), portA, "", false)
	endpointA := config.NewEndpointsWithBatchSettings(mainA, nil, false, true, 10*time.Millisecond, 1, 1, 1024*1024, 2)
	requests := make(chan routedRequest, 4)
	available, endpointB := routedTestEndpoint(t, "b", "key-b", requests)
	defer available.Close()
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.routing.max_groups", 4)
	cfg.SetInTest("logs_config.payload_channel_size", 2)
	ctx := client.NewDestinationsContext()
	ctx.Start()
	sink := &routedTestSink{ch: make(chan *message.Payload, 4)}
	provider, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Groups:                    []routing.Group{{ID: "a", Endpoints: endpointA}, {ID: "b", Endpoints: endpointB}},
		Resolve: func(*message.Message) (routing.Selection, error) {
			return routing.Selection{Groups: []string{"a", "b"}, Generation: 1}, nil
		},
		DestinationsContext: ctx,
		Config:              cfg,
		Compression:         compressionfx.NewMockCompressor(),
		Secrets:             secretsnoopimpl.NewComponent().Comp,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider.Start()
	defer func() {
		ctx.Stop()
		provider.Stop()
	}()
	source := sources.NewLogSource("test", &config.LogsConfig{Type: config.StringChannelType})
	for i := range 3 {
		msg := message.NewMessageWithSource([]byte(strconv.Itoa(i)), "info", source, time.Now().UnixNano())
		msg.Origin.Identifier = "file:shared-outage"
		msg.Origin.Offset = strconv.Itoa(i + 1)
		provider.NextPipelineChan() <- msg
	}
	for range 3 {
		select {
		case request := <-requests:
			if request.endpoint != "b" {
				t.Fatalf("healthy group request: %+v", request)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("healthy group stalled before routing capacity filled")
		}
	}
	select {
	case checkpoint := <-sink.ch:
		t.Fatalf("retrying group obligation lost at %s", checkpoint.MessageMetas[0].Origin.Offset)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestRoutedProviderFlushIncludesPriorRecord(t *testing.T) {
	requests := make(chan routedRequest, 1)
	server, endpoints := routedTestEndpoint(t, "a", "key-a", requests)
	defer server.Close()
	endpoints.BatchWait = time.Hour
	endpoints.BatchMaxSize = 100
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.routing.max_groups", 4)
	cfg.SetInTest("logs_config.payload_channel_size", 2)
	ctx := client.NewDestinationsContext()
	ctx.Start()
	sink := &routedTestSink{ch: make(chan *message.Payload, 1)}
	provider, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines:         2,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Groups:                    []routing.Group{{ID: "a", Endpoints: endpoints}},
		Resolve: func(*message.Message) (routing.Selection, error) {
			return routing.Selection{Groups: []string{"a"}, Generation: 1}, nil
		},
		DestinationsContext: ctx,
		Config:              cfg,
		Compression:         compressionfx.NewMockCompressor(),
		Secrets:             secretsnoopimpl.NewComponent().Comp,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider.Start()
	defer provider.Stop()
	source := sources.NewLogSource("test", &config.LogsConfig{Type: config.StringChannelType})
	msg := message.NewMessageWithSource([]byte("flush-me"), "info", source, time.Now().UnixNano())
	msg.Origin.Identifier = "file:flush"
	msg.Origin.Offset = "1"
	provider.NextPipelineChan() <- msg
	flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	provider.Flush(flushCtx)
	if err := flushCtx.Err(); err != nil {
		t.Fatalf("flush did not complete: %v", err)
	}
	select {
	case got := <-requests:
		if got.message != "flush-me" {
			t.Fatalf("flush sent %q", got.message)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("flush left a prior record in the batch")
	}
	// Flush may race with lifecycle shutdown; neither may send on a closed
	// ingress channel or wait for a marker that can no longer be processed.
	flushDone := make(chan struct{})
	go func() {
		defer close(flushDone)
		for range 20 {
			provider.Flush(flushCtx)
		}
	}()
	stopDone := make(chan struct{})
	go func() {
		provider.Stop()
		close(stopDone)
	}()
	for _, done := range []<-chan struct{}{flushDone, stopDone} {
		select {
		case <-done:
		case <-flushCtx.Done():
			t.Fatal("concurrent flush and stop did not finish")
		}
	}
}

func TestRoutedProviderCredentialUnavailableKeepsGroupPending(t *testing.T) {
	for _, tc := range []struct {
		name        string
		settingPath string
	}{
		{"expected later", "logs_config.additional_endpoints"},
		{"permanently unavailable", "database_monitoring.samples.additional_endpoints"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unavailableRequests := make(chan struct{}, 1)
			unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				select {
				case unavailableRequests <- struct{}{}:
				default:
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer unavailable.Close()
			parsed, _ := url.Parse(unavailable.URL)
			port, _ := strconv.Atoi(parsed.Port())
			mainA := config.NewEndpoint("DELA[org-uuid, aws]", tc.settingPath, parsed.Hostname(), port, "", false)
			endpointA := config.NewEndpointsWithBatchSettings(mainA, nil, false, true, 10*time.Millisecond, 1, 1, 1024*1024, 2)
			availableRequests := make(chan routedRequest, 1)
			available, endpointB := routedTestEndpoint(t, "b", "key-b", availableRequests)
			defer available.Close()
			cfg := configmock.New(t)
			cfg.SetInTest("logs_config.routing.max_groups", 4)
			cfg.SetInTest("logs_config.payload_channel_size", 2)
			ctx := client.NewDestinationsContext()
			ctx.Start()
			sink := &routedTestSink{ch: make(chan *message.Payload, 1)}
			provider, err := NewRoutedProvider(RoutedOptions{
				NumberOfPipelines:         1,
				Sink:                      sink,
				DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
				Groups:                    []routing.Group{{ID: "a", Endpoints: endpointA}, {ID: "b", Endpoints: endpointB}},
				Resolve: func(*message.Message) (routing.Selection, error) {
					return routing.Selection{Groups: []string{"a", "b"}, Generation: 1}, nil
				},
				DestinationsContext: ctx,
				Config:              cfg,
				Compression:         compressionfx.NewMockCompressor(),
				Secrets:             secretsnoopimpl.NewComponent().Comp,
			})
			if err != nil {
				t.Fatal(err)
			}
			provider.Start()
			defer func() {
				ctx.Stop()
				provider.Stop()
			}()
			source := sources.NewLogSource("test", &config.LogsConfig{Type: config.StringChannelType})
			msg := message.NewMessageWithSource([]byte("needs-both"), "info", source, time.Now().UnixNano())
			msg.Origin.Identifier = "file:credential"
			msg.Origin.Offset = "1"
			provider.NextPipelineChan() <- msg
			select {
			case <-availableRequests:
			case <-time.After(5 * time.Second):
				t.Fatal("available group did not progress")
			}
			select {
			case <-unavailableRequests:
				t.Fatal("credential-less group sent an HTTP request")
			case checkpoint := <-sink.ch:
				t.Fatalf("credential-less group advanced checkpoint to %s", checkpoint.MessageMetas[0].Origin.Offset)
			case <-time.After(20 * time.Millisecond):
			}
		})
	}
}

func TestRoutedProviderPermanentHTTPRejectionIsTerminalLoss(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(parsed.Port())
	main := config.NewEndpoint("key-a", "", parsed.Hostname(), port, "", false)
	endpoints := config.NewEndpointsWithBatchSettings(main, nil, false, true, 10*time.Millisecond, 1, 1, 1024*1024, 2)
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.routing.max_groups", 4)
	cfg.SetInTest("logs_config.payload_channel_size", 2)
	ctx := client.NewDestinationsContext()
	ctx.Start()
	sink := &routedTestSink{ch: make(chan *message.Payload, 1)}
	provider, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Groups:                    []routing.Group{{ID: "a", Endpoints: endpoints}},
		Resolve: func(*message.Message) (routing.Selection, error) {
			return routing.Selection{Groups: []string{"a"}, Generation: 1}, nil
		},
		DestinationsContext: ctx,
		Config:              cfg,
		Compression:         compressionfx.NewMockCompressor(),
		Secrets:             secretsnoopimpl.NewComponent().Comp,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider.Start()
	defer provider.Stop()
	source := sources.NewLogSource("test", &config.LogsConfig{Type: config.StringChannelType})
	msg := message.NewMessageWithSource([]byte("permanent-loss"), "info", source, time.Now().UnixNano())
	msg.Origin.Identifier = "file:permanent"
	msg.Origin.Offset = "1"
	provider.NextPipelineChan() <- msg
	select {
	case checkpoint := <-sink.ch:
		if checkpoint.MessageMetas[0].Origin.Offset != "1" {
			t.Fatal("wrong terminal-loss checkpoint")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("permanent HTTP rejection did not reach existing terminal output")
	}
}
