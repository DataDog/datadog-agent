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

func TestRoutedFanoutOffersHealthyGroupBeforeBlockedGroup(t *testing.T) {
	blocked := make(chan *message.Payload, 1)
	healthy := make(chan *message.Payload, 1)
	blocked <- message.NewPayload(nil, nil, "", 0) // A's queue is already full.
	sink := &routedTestSink{ch: make(chan *message.Payload, 1)}
	progress, err := routing.NewProgress(2, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer progress.Close()
	source := sources.NewLogSource("test", &config.LogsConfig{Type: config.StringChannelType})
	msg := message.NewMessageWithSource([]byte("both"), "info", source, time.Now().UnixNano())
	msg.Origin.Identifier = "file:fanout"
	msg.Origin.Offset = "1"
	if _, err := progress.Register(context.Background(), msg, routing.Selection{Groups: []string{"a", "b"}, Generation: 1}, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	meta := msg.MessageMetadata
	payload := message.NewPayload([]*message.MessageMetadata{&meta}, []byte("encoded"), "", 7)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		offerPayloadToGroups(ctx, payload, []string{"a", "b"}, map[string]chan *message.Payload{"a": blocked, "b": healthy})
		close(done)
	}()
	select {
	case got := <-healthy:
		if got == payload || !bytes.Equal(got.Encoded, payload.Encoded) {
			t.Fatal("healthy group received the wrong payload")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("healthy group waited for blocked A")
	}
	select {
	case <-done:
		t.Fatal("A's pending copy was discarded")
	default:
	}
	select {
	case checkpoint := <-sink.ch:
		t.Fatalf("queue admission advanced checkpoint: %+v", checkpoint)
	default:
	}
	progress.CompleteGroup("b", payload)
	select {
	case <-sink.ch:
		t.Fatal("B completion advanced past pending A")
	default:
	}
	<-blocked // Make room for A's retained copy.
	select {
	case got := <-blocked:
		if !bytes.Equal(got.Encoded, payload.Encoded) {
			t.Fatal("A's retained payload changed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("A's retained copy was not admitted")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("fanout did not finish after A had capacity")
	}
	select {
	case <-healthy:
		t.Fatal("B received a duplicate copy")
	default:
	}
	progress.CompleteGroup("a", payload)
	select {
	case checkpoint := <-sink.ch:
		if got := checkpoint.MessageMetas[0].Origin.Offset; got != "1" {
			t.Fatalf("checkpoint offset = %q", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("completed A and B did not advance checkpoint")
	}
}

func TestRoutedFanoutOffersHealthyStreamBeforeBlockedStream(t *testing.T) {
	blocked := make(chan *message.Message, 1)
	healthy := make(chan *message.Message, 1)
	blocked <- &message.Message{}
	msg := &message.Message{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		offerToChannels(ctx, []channelOffer[*message.Message]{
			{input: blocked, value: msg},
			{input: healthy, value: msg},
		})
		close(done)
	}()
	select {
	case got := <-healthy:
		if got != msg {
			t.Fatal("healthy stream received a different record")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("healthy stream waited for blocked stream")
	}
	select {
	case <-done:
		t.Fatal("blocked stream's copy was discarded")
	default:
	}
	<-blocked
	select {
	case got := <-blocked:
		if got != msg {
			t.Fatal("blocked stream received a different record")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("blocked stream's copy was not retained")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("fanout did not finish")
	}
	select {
	case <-healthy:
		t.Fatal("healthy stream received a duplicate record")
	default:
	}
}

func TestRoutedDispatchDoesNotWaitAcrossSources(t *testing.T) {
	requests := make(chan routedRequest, 1)
	a, endpointA := routedTestEndpoint(t, "a", "key-a", requests)
	defer a.Close()
	b, endpointB := routedTestEndpoint(t, "b", "key-b", requests)
	defer b.Close()
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.routing.max_groups", 4)
	ctx := client.NewDestinationsContext()
	p, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines: 1, Sink: &routedTestSink{ch: make(chan *message.Payload, 2)},
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Groups:                    []routing.Group{{ID: "a", Endpoints: endpointA}, {ID: "b", Endpoints: endpointB}},
		Resolve:                   func(*message.Message) (routing.Selection, error) { return routing.Selection{}, nil },
		DestinationsContext:       ctx, Config: cfg,
		Compression: compressionfx.NewMockCompressor(), Secrets: secretsnoopimpl.NewComponent().Comp,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Stop()
	p.streams["group:a"].input <- &message.Message{} // A is saturated.
	source := sources.NewLogSource("test", &config.LogsConfig{Type: config.StringChannelType})
	makeMessage := func(id string, groups []string) *message.Message {
		m := message.NewMessageWithSource([]byte(id), "info", source, time.Now().UnixNano())
		m.Origin.Identifier, m.Origin.Offset = id, "1"
		if _, err := p.progress.Register(context.Background(), m, routing.Selection{Groups: groups, Generation: 1}, groups); err != nil {
			t.Fatal(err)
		}
		return m
	}
	x := makeMessage("x", []string{"a", "b"})
	y := makeMessage("y", []string{"b"})
	go p.dispatchProcessed()
	p.processed <- x
	p.processed <- y
	seen := make(map[string]bool)
	for len(seen) < 2 {
		select {
		case got := <-p.streams["group:b"].input:
			seen[got.Origin.Identifier] = true
		case <-time.After(time.Second):
			p.cancel()
			close(p.processed)
			<-p.processedDone
			t.Fatal("Y waited for X's blocked A stream")
		}
	}
	p.cancel()
	close(p.processed)
	<-p.processedDone
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
	if err := provider.RetireSource("file:test", 1); err != nil {
		t.Fatal(err)
	}
	provider.sourceDispatchMu.Lock()
	_, retained := provider.sourceDispatch["\x01file:test"]
	provider.sourceDispatchMu.Unlock()
	if retained {
		t.Fatal("retired source retained its dispatch worker")
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

// A saturated X must not hold up an unrelated file that selects only B.
// The source budget is intentionally smaller than the aggregate progress budget.
func TestRoutedProviderSaturatedSourceDoesNotBlockHealthySource(t *testing.T) {
	var aAvailable atomic.Bool
	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if !aAvailable.Load() {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer a.Close()
	parsedA, _ := url.Parse(a.URL)
	portA, _ := strconv.Atoi(parsedA.Port())
	mainA := config.NewEndpoint("key-a", "", parsedA.Hostname(), portA, "", false)
	endpointA := config.NewEndpointsWithBatchSettings(mainA, nil, false, true, 10*time.Millisecond, 1, 1, 1024*1024, 2)
	requests := make(chan routedRequest, 128)
	b, endpointB := routedTestEndpoint(t, "b", "key-b", requests)
	defer b.Close()
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.routing.max_groups", 4)
	cfg.SetInTest("logs_config.payload_channel_size", 2)
	ctx := client.NewDestinationsContext()
	ctx.Start()
	sink := &routedTestSink{ch: make(chan *message.Payload, 128)}
	provider, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines: 1, Sink: sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Groups:                    []routing.Group{{ID: "a", Endpoints: endpointA}, {ID: "b", Endpoints: endpointB}},
		Resolve: func(m *message.Message) (routing.Selection, error) {
			if m.Origin.Identifier == "file:y" {
				return routing.Selection{Groups: []string{"b"}, Generation: 1}, nil
			}
			return routing.Selection{Groups: []string{"a", "b"}, Generation: 1}, nil
		},
		DestinationsContext: ctx, Config: cfg,
		Compression: compressionfx.NewMockCompressor(), Secrets: secretsnoopimpl.NewComponent().Comp,
	})
	if err != nil {
		t.Fatal(err)
	}
	provider.Start()
	defer func() { ctx.Stop(); provider.Stop() }()
	source := sources.NewLogSource("test", &config.LogsConfig{Type: config.StringChannelType})
	xInput, err := provider.SourceInput("file:x", 1)
	if err != nil {
		t.Fatal(err)
	}
	yInput, err := provider.SourceInput("file:y", 1)
	if err != nil {
		t.Fatal(err)
	}
	makeMessage := func(id string, offset int) *message.Message {
		m := message.NewMessageWithSource([]byte(id+":"+strconv.Itoa(offset)), "info", source, time.Now().UnixNano())
		m.Origin.Identifier, m.Origin.Offset = "file:"+id, strconv.Itoa(offset)
		return m
	}
	for i := 1; i <= 64; i++ {
		xInput <- makeMessage("x", i)
	}
	seenX := make(map[string]bool)
	for i := 0; i < 64; i++ {
		select {
		case request := <-requests:
			if request.endpoint != "b" || !bytes.HasPrefix([]byte(request.message), []byte("x:")) || seenX[request.message] {
				t.Fatalf("unexpected or repeated B request %q at %d", request.message, i)
			}
			seenX[request.message] = true
		case <-time.After(10 * time.Second):
			t.Fatal("X did not reach B before its source budget filled")
		}
	}
	xSent := make(chan struct{})
	go func() { xInput <- makeMessage("x", 65); close(xSent) }()
	select {
	case <-xSent:
	case <-time.After(10 * time.Second):
		t.Fatal("X's next record could not enter bounded ingress")
	}
	xInput <- makeMessage("x", 66) // One queued record may sit behind registration.
	select {
	case xInput <- makeMessage("x", 67):
		t.Fatal("X kept accepting records beyond its bounded source input")
	default:
	}
	flushCtx, cancelFlush := context.WithTimeout(context.Background(), 20*time.Millisecond)
	provider.Flush(flushCtx)
	if flushCtx.Err() != context.DeadlineExceeded {
		t.Fatal("blocked Flush did not honor its deadline")
	}
	cancelFlush()
	yInput <- makeMessage("y", 1)
	select {
	case request := <-requests:
		if request.message != "y:1" {
			t.Fatalf("X exceeded its source budget before Y progressed: %q", request.message)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Y could not deliver to healthy B while X was saturated")
	}
	select {
	case checkpoint := <-sink.ch:
		if checkpoint.MessageMetas[0].Origin.Identifier != "file:y" {
			t.Fatalf("X checkpoint advanced with A pending: %s", checkpoint.MessageMetas[0].Origin.Identifier)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Y checkpoint did not advance independently")
	}
	if err := provider.RetireSource("file:y", 1); err != nil {
		t.Fatal(err)
	}
	provider.lifecycle.RLock()
	_, yRetained := provider.sourceInputs[sourceInputKey{identifier: "file:y", generation: 1}]
	provider.lifecycle.RUnlock()
	if yRetained {
		t.Fatal("retired Y retained its producer input")
	}
	aAvailable.Store(true)
	for _, want := range []string{"x:65", "x:66"} {
		select {
		case request := <-requests:
			if request.message != want {
				t.Fatalf("X resumed with request %q, want %q", request.message, want)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("X did not deliver to B after A recovered")
		}
	}
	deadline := time.After(30 * time.Second)
	for {
		select {
		case checkpoint := <-sink.ch:
			if checkpoint.MessageMetas[0].Origin.Identifier != "file:x" {
				t.Fatalf("unexpected checkpoint source %s", checkpoint.MessageMetas[0].Origin.Identifier)
			}
			if checkpoint.MessageMetas[0].Origin.Offset == "66" {
				return
			}
		case <-deadline:
			t.Fatal("X checkpoint did not recover after A became available")
		}
	}
}

func TestRoutedProviderStopReleasesBlockedSourceAdmission(t *testing.T) {
	requests := make(chan routedRequest, 1)
	server, endpoints := routedTestEndpoint(t, "a", "key-a", requests)
	defer server.Close()
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.routing.max_groups", 4)
	ctx := client.NewDestinationsContext()
	ctx.Start()
	defer ctx.Stop()
	sink := &routedTestSink{ch: make(chan *message.Payload, 1)}
	p, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines: 1, Sink: sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Groups:                    []routing.Group{{ID: "a", Endpoints: endpoints}},
		Resolve: func(*message.Message) (routing.Selection, error) {
			return routing.Selection{Groups: []string{"a"}, Generation: 1}, nil
		},
		DestinationsContext: ctx, Config: cfg,
		Compression: compressionfx.NewMockCompressor(), Secrets: secretsnoopimpl.NewComponent().Comp,
	})
	if err != nil {
		t.Fatal(err)
	}
	p.Start()
	input, err := p.SourceInput("file:blocked", 1)
	if err != nil {
		t.Fatal(err)
	}
	source := sources.NewLogSource("test", &config.LogsConfig{Type: config.StringChannelType})
	makeMessage := func() *message.Message {
		m := message.NewMessageWithSource([]byte("pending"), "info", source, time.Now().UnixNano())
		m.Origin.Identifier, m.Origin.Offset = "file:blocked", "1"
		return m
	}
	for i := 0; i < routing.SourcePendingLimit; i++ {
		if _, err := p.progress.Register(context.Background(), makeMessage(), routing.Selection{Generation: 1}, []string{"a"}); err != nil {
			t.Fatal(err)
		}
	}
	input <- makeMessage() // Its registration waits for X's source budget.
	done := make(chan struct{})
	go func() { p.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(routedStopGrace + 2*time.Second):
		ctx.Stop()
		t.Fatal("Stop remained blocked behind source admission")
	}
	select {
	case checkpoint := <-sink.ch:
		t.Fatalf("shutdown acknowledged a pending record: %v", checkpoint)
	default:
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

func TestRoutedProviderRetiredRecordDoesNotBlockReplacement(t *testing.T) {
	endpoint := config.NewEndpoint("test-key", "", "localhost", 12345, "", false)
	endpoints := config.NewEndpointsWithBatchSettings(endpoint, nil, false, true, time.Second, 1, 1, 1024, 2)
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.routing.max_groups", 4)
	sink := &routedTestSink{ch: make(chan *message.Payload, 4)}
	reported := make(chan error, 1)
	p, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines: 1, Sink: sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Groups:                    []routing.Group{{ID: "a", Endpoints: endpoints}},
		Resolve: func(m *message.Message) (routing.Selection, error) {
			generation := uint64(7)
			if string(m.GetContent()) == "old" {
				generation = 42
			}
			return routing.Selection{Excluded: true, Generation: generation}, nil
		},
		OnError:             func(err error) { reported <- err },
		DestinationsContext: client.NewDestinationsContext(), Config: cfg,
		Compression: compressionfx.NewMockCompressor(), Secrets: secretsnoopimpl.NewComponent().Comp,
	})
	if err != nil {
		t.Fatal(err)
	}
	p.Start()
	defer p.Stop()
	oldInput, err := p.SourceInput("file:test", 42)
	if err != nil {
		t.Fatal(err)
	}
	currentInput, err := p.SourceInput("file:test", 7)
	if err != nil {
		t.Fatal(err)
	}
	source := sources.NewLogSource("test", &config.LogsConfig{Type: config.StringChannelType})
	makeRecord := func(content, offset string) *message.Message {
		m := message.NewMessageWithSource([]byte(content), "info", source, time.Now().UnixNano())
		m.Origin.Identifier = "file:test"
		m.Origin.Offset = offset
		return m
	}
	checkpoint := func(want string) {
		t.Helper()
		select {
		case payload := <-sink.ch:
			if got := payload.MessageMetas[0].Origin.Offset; got != want {
				t.Fatalf("checkpoint=%q, want %q", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("checkpoint %s did not arrive", want)
		}
	}
	oldInput <- makeRecord("old", "old-1")
	checkpoint("old-1")
	currentInput <- makeRecord("current", "new-1")
	checkpoint("new-1")
	if err := p.RetireSource("file:test", 42); err != nil {
		t.Fatal(err)
	}
	// A caller violating retirement must not poison the replacement source.
	p.NextPipelineChan() <- makeRecord("old", "old-2")
	select {
	case err := <-reported:
		if !errors.Is(err, routing.ErrObsoleteGeneration) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("retired producer was not reported")
	}
	currentInput <- makeRecord("current", "new-2")
	checkpoint("new-2")
	if got := p.progress.Pending(); got != 0 {
		t.Fatalf("retired producer left %d pending records", got)
	}
}
