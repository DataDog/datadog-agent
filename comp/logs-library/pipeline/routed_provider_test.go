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
	"testing/synctest"
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
	endpoint   string
	key        string
	message    string
	batchCount int
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
			requests <- routedRequest{endpoint: id, key: r.Header.Get("DD-API-KEY"), message: record.Message, batchCount: len(records)}
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

// newRoutedTestProvider supplies common dependencies. Tests keep groups,
// selection, capacity, processing, and provider lifecycle explicit.
func newRoutedTestProvider(t *testing.T, opts RoutedOptions) *RoutedProvider {
	t.Helper()
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.routing.max_groups", 4)
	opts.Config = cfg
	opts.DiagnosticMessageReceiver = &diagnostic.BufferedMessageReceiver{}
	opts.DestinationsContext = client.NewDestinationsContext()
	opts.Compression = compressionfx.NewMockCompressor()
	opts.Secrets = secretsnoopimpl.NewComponent().Comp
	p, err := NewRoutedProvider(opts)
	if err != nil {
		t.Fatal(err)
	}
	return p
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
		Generation:                func(*message.Message) (uint64, error) { return 1, nil },
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
		Generation:                func(*message.Message) (uint64, error) { return 1, nil },
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
		{"a", "key-a", "a", 1}:    1,
		{"b", "key-b", "b", 1}:    1,
		{"a", "key-a", "both", 1}: 1,
		{"b", "key-b", "both", 1}: 1,
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
	if err := provider.RetireSource(context.Background(), "file:test", 1); err != nil {
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
		Generation:                func(*message.Message) (uint64, error) { return 1, nil },
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
		Generation:                func(*message.Message) (uint64, error) { return 1, nil },
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
		Generation:                func(*message.Message) (uint64, error) { return 1, nil },
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
		Generation:                func(*message.Message) (uint64, error) { return 1, nil },
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
		Generation:                func(*message.Message) (uint64, error) { return 1, nil },
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
		Generation:                func(*message.Message) (uint64, error) { return 1, nil },
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
	if err := provider.RetireSource(context.Background(), "file:y", 1); err != nil {
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
		Generation:                func(*message.Message) (uint64, error) { return 1, nil },
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
		Generation:                func(*message.Message) (uint64, error) { return 1, nil },
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
				Generation:                func(*message.Message) (uint64, error) { return 1, nil },
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
		Generation:                func(*message.Message) (uint64, error) { return 1, nil },
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
		Generation: func(m *message.Message) (uint64, error) {
			if string(m.GetContent()) == "old" {
				return 42, nil
			}
			return 7, nil
		},
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
	currentInput, err := p.SourceInput("file:test", 7)
	if err != nil {
		t.Fatal(err)
	}
	currentInput <- makeRecord("current", "new-1")
	checkpoint("new-1")
	if err := p.RetireSource(context.Background(), "file:test", 42); err != nil {
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

func TestRoutedProviderRetirementKeepsBufferedInputBeforeStart(t *testing.T) {
	progress, err := routing.NewProgress(4, &routedTestSink{ch: make(chan *message.Payload, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer progress.Close()
	p := &RoutedProvider{
		progress:     progress,
		sourceInputs: make(map[sourceInputKey]chan *message.Message),
		inputKeys:    make(map[chan *message.Message]sourceInputKey),
	}
	input, err := p.SourceInput("file:test", 42)
	if err != nil {
		t.Fatal(err)
	}
	input <- &message.Message{}
	if err := p.RetireSource(context.Background(), "file:test", 42); !errors.Is(err, routing.ErrSourcePending) {
		t.Fatalf("retirement before admission = %v, want ErrSourcePending", err)
	}
	if !progress.OwnsSource("file:test") {
		t.Fatal("buffered input lost its source ownership")
	}
	<-input
	if err := p.RetireSource(context.Background(), "file:test", 42); err != nil {
		t.Fatal(err)
	}
}

func TestRoutedProviderRetirementWaitCancels(t *testing.T) {
	for _, stop := range []bool{false, true} {
		name := "cancel"
		if stop {
			name = "stop"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				progress, err := routing.NewProgress(4, &routedTestSink{ch: make(chan *message.Payload, 1)})
				if err != nil {
					t.Fatal(err)
				}
				defer progress.Close()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				p := &RoutedProvider{
					progress: progress, ctx: ctx, stopRequested: make(chan struct{}),
					sourceInputs: make(map[sourceInputKey]chan *message.Message),
					inputKeys:    make(map[chan *message.Message]sourceInputKey),
				}
				if _, err := p.SourceInput("file:test", 42); err != nil {
					t.Fatal(err)
				}
				p.started = true
				retired := make(chan error, 1)
				go func() { retired <- p.RetireSource(context.Background(), "file:test", 42) }()
				synctest.Wait()
				select {
				case err := <-retired:
					t.Fatalf("retirement completed before admission: %v", err)
				default:
				}
				if stop {
					close(p.stopRequested)
				} else {
					cancel()
				}
				if err := <-retired; !errors.Is(err, routing.ErrStopped) {
					t.Fatalf("cancelled retirement = %v, want ErrStopped", err)
				}
				if !progress.OwnsSource("file:test") {
					t.Fatal("cancelled retirement released ownership")
				}
			})
		})
	}
}

func TestRoutedProviderRetirementWaitsForAdmission(t *testing.T) {
	for _, buffered := range []bool{false, true} {
		name := "in-flight"
		if buffered {
			name = "buffered"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				endpoint := config.NewEndpoint("test-key", "", "localhost", 12345, "", false)
				endpoints := config.NewEndpointsWithBatchSettings(endpoint, nil, false, true, time.Second, 1, 1, 1024, 2)
				cfg := configmock.New(t)
				cfg.SetInTest("logs_config.routing.max_groups", 4)
				sink := &routedTestSink{ch: make(chan *message.Payload, 4)}
				resolving := make(chan struct{})
				release := make(chan struct{})
				p, err := NewRoutedProvider(RoutedOptions{
					NumberOfPipelines: 1, Sink: sink,
					DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
					Groups:                    []routing.Group{{ID: "a", Endpoints: endpoints}},
					Generation:                func(*message.Message) (uint64, error) { return 1, nil },
					Resolve: func(m *message.Message) (routing.Selection, error) {
						if string(m.GetContent()) == "old-first" {
							close(resolving)
							<-release
						}
						generation := uint64(7)
						if bytes.HasPrefix(m.GetContent(), []byte("old")) {
							generation = 42
						}
						return routing.Selection{Excluded: true, Generation: generation}, nil
					},
					DestinationsContext: client.NewDestinationsContext(), Config: cfg,
					Compression: compressionfx.NewMockCompressor(), Secrets: secretsnoopimpl.NewComponent().Comp,
				})
				if err != nil {
					t.Fatal(err)
				}
				p.Start()
				defer p.Stop()
				defer func() {
					select {
					case <-release:
					default:
						close(release)
					}
				}()
				oldInput, err := p.SourceInput("file:test", 42)
				if err != nil {
					t.Fatal(err)
				}
				source := sources.NewLogSource("test", &config.LogsConfig{Type: config.StringChannelType})
				makeRecord := func(content string) *message.Message {
					m := message.NewMessageWithSource([]byte(content), "info", source, time.Now().UnixNano())
					m.Origin.Identifier = "file:test"
					m.Origin.Offset = content
					return m
				}
				oldInput <- makeRecord("old-first")
				<-resolving
				if buffered {
					oldInput <- makeRecord("old-last")
				}
				retired := make(chan error, 1)
				go func() { retired <- p.RetireSource(context.Background(), "file:test", 42) }()
				synctest.Wait()
				// Admission now reserves before Resolve runs in the processor.
				// Retirement may finish its admission barrier, but must refuse
				// to release the producer while selection remains pending.
				if err := <-retired; !errors.Is(err, routing.ErrSourcePending) {
					t.Fatalf("retirement during selection = %v, want ErrSourcePending", err)
				}
				currentInput, err := p.SourceInput("file:test", 7)
				if err != nil {
					t.Fatal(err)
				}
				currentInput <- makeRecord("current")
				close(release)
				synctest.Wait()
				if err := p.RetireSource(context.Background(), "file:test", 42); err != nil {
					t.Fatal(err)
				}
				select {
				case payload := <-sink.ch:
					if got := payload.MessageMetas[0].Origin.Offset; got != "current" {
						t.Fatalf("obsolete checkpoint %q replaced current ownership", got)
					}
				default:
					t.Fatal("replacement checkpoint did not arrive")
				}
				select {
				case payload := <-sink.ch:
					t.Fatalf("unexpected checkpoint: %s", payload.MessageMetas[0].Origin.Offset)
				default:
				}
			})
		})
	}
}

func TestRoutedProviderStopCancelsRetryingHTTPAfterGrace(t *testing.T) {
	received := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case received <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	parsed, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(parsed.Port())
	endpoint := config.NewEndpoint("test-key", "", parsed.Hostname(), port, "", false)
	endpoint.BackoffBase, endpoint.BackoffMax, endpoint.BackoffFactor = 1, 2, 2
	endpoints := config.NewEndpointsWithBatchSettings(endpoint, nil, false, true, time.Millisecond, 1, 1, 1024, 2)
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.routing.max_groups", 4)
	ctx := client.NewDestinationsContext()
	ctx.Start()
	defer ctx.Stop()
	sink := &routedTestSink{ch: make(chan *message.Payload, 4)}
	p, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines: 1, Sink: sink, DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Groups:     []routing.Group{{ID: "a", Endpoints: endpoints}},
		Generation: func(*message.Message) (uint64, error) { return 1, nil },
		Resolve: func(*message.Message) (routing.Selection, error) {
			return routing.Selection{Groups: []string{"a"}, Generation: 1}, nil
		},
		DestinationsContext: ctx, Config: cfg, Compression: compressionfx.NewMockCompressor(), Secrets: secretsnoopimpl.NewComponent().Comp,
	})
	if err != nil {
		t.Fatal(err)
	}
	p.Start()
	input, err := p.SourceInput("file:audit", 1)
	if err != nil {
		t.Fatal(err)
	}
	source := sources.NewLogSource("audit", &config.LogsConfig{Type: config.StringChannelType})
	msg := message.NewMessageWithSource([]byte("audit"), "info", source, time.Now().UnixNano())
	msg.Origin.Identifier = "file:audit"
	msg.Origin.Offset = "1"
	input <- msg
	select {
	case <-received:
	case <-time.After(5 * time.Second):
		ctx.Stop()
		p.Stop()
		t.Fatal("HTTP request did not start")
	}
	stopped := make(chan struct{})
	go func() { p.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(routedStopGrace + 2*time.Second):
		ctx.Stop() // Release the existing sender for test cleanup.
		select {
		case <-stopped:
		case <-time.After(5 * time.Second):
			t.Fatal("shutdown still blocked after cancelling destinations")
		}
		t.Error("provider Stop exceeded its grace period; it required external DestinationsContext.Stop")
	}
	if ctx.Context().Err() != nil {
		t.Fatal("provider Stop cancelled the caller's shared destinations context")
	}
	if len(sink.ch) != 0 {
		t.Fatal("retrying record was acknowledged")
	}
}

func TestRoutedProviderCancelledFlushDoesNotStallHealthySource(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg := configmock.New(t)
		cfg.SetInTest("logs_config.routing.max_groups", 4)
		endpoint := config.NewEndpoint("key", "", "localhost", 12345, "", false)
		endpoints := config.NewEndpointsWithBatchSettings(endpoint, nil, false, true, time.Second, 1, 1, 1024, 2)
		p, err := NewRoutedProvider(RoutedOptions{
			NumberOfPipelines: 1, Sink: &routedTestSink{ch: make(chan *message.Payload, 4)},
			DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
			Groups:                    []routing.Group{{ID: "a", Endpoints: endpoints}, {ID: "b", Endpoints: endpoints}},
			Generation:                func(*message.Message) (uint64, error) { return 1, nil },
			Resolve:                   func(*message.Message) (routing.Selection, error) { return routing.Selection{}, nil },
			DestinationsContext:       client.NewDestinationsContext(), Config: cfg,
			Compression: compressionfx.NewMockCompressor(), Secrets: secretsnoopimpl.NewComponent().Comp,
		})
		if err != nil {
			t.Fatal(err)
		}
		// Exercise processing and dispatch with a saturated A queue, without HTTP.
		// This represents other A-bound sources filling that bounded stage.
		for i := 0; i < cap(p.streams["group:a"].input); i++ {
			p.streams["group:a"].input <- &message.Message{}
		}
		p.processors[0].Start()
		go p.dispatchProcessed()
		defer func() {
			p.cancel()
			p.processors[0].Stop()
			close(p.processed)
			<-p.processedDone
			p.started = false
			p.Stop()
		}()
		source := sources.NewLogSource("audit", &config.LogsConfig{Type: config.StringChannelType})
		record := func(id string, groups []string) *message.Message {
			m := message.NewMessageWithSource([]byte(id), "info", source, time.Now().UnixNano())
			m.Origin.Identifier = id
			m.Origin.Offset = "1"
			if _, err := p.progress.Register(context.Background(), m, routing.Selection{Groups: groups, Generation: 1}, groups); err != nil {
				t.Fatal(err)
			}
			return m
		}
		p.processed <- record("x", []string{"a", "b"})
		<-p.streams["group:b"].input // X reached B; its source worker waits for A.
		// The public Flush has no ingress to drain in this dispatcher-only setup.
		p.inputs = nil
		p.started = true
		for i := 0; i < 2*routing.SourcePendingLimit; i++ {
			flushCtx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { p.Flush(flushCtx); close(done) }()
			synctest.Wait() // Flush waits for X's stalled dispatch, without queue markers.
			cancel()
			<-done
		}
		// More X work must fit in its data queue despite all abandoned flushes.
		p.processed <- record("x", []string{"a", "b"})
		p.processed <- record("y", []string{"b"})
		synctest.Wait()
		select {
		case m := <-p.streams["group:b"].input:
			if m.Origin == nil || m.Origin.Identifier != "y" {
				t.Fatal("wrong record")
			}
		default:
			t.Error("cancelled Flush left a barrier blocking healthy Y behind stalled X")
		}
	})
}

func TestRoutedProviderRetiresSourceWhoseFirstRecordIsInvalid(t *testing.T) {
	endpoint := config.NewEndpoint("key", "", "localhost", 12345, "", false)
	endpoints := config.NewEndpointsWithBatchSettings(endpoint, nil, false, true, time.Second, 1, 1, 1024, 2)
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.routing.max_groups", 4)
	p, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines: 1, Sink: &routedTestSink{ch: make(chan *message.Payload, 1)},
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Groups:                    []routing.Group{{ID: "a", Endpoints: endpoints}},
		Generation:                func(*message.Message) (uint64, error) { return 1, nil },
		Resolve:                   func(*message.Message) (routing.Selection, error) { return routing.Selection{Generation: 1}, nil },
		OnError:                   func(error) {}, DestinationsContext: client.NewDestinationsContext(), Config: cfg,
		Compression: compressionfx.NewMockCompressor(), Secrets: secretsnoopimpl.NewComponent().Comp,
	})
	if err != nil {
		t.Fatal(err)
	}
	p.Start()
	defer p.Stop()
	input, err := p.SourceInput("file:invalid-first", 1)
	if err != nil {
		t.Fatal(err)
	}
	source := sources.NewLogSource("test", &config.LogsConfig{Type: config.StringChannelType})
	msg := message.NewMessageWithSource([]byte("invalid"), "info", source, time.Now().UnixNano())
	msg.Origin.Identifier = "file:invalid-first"
	input <- msg
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	p.Flush(ctx)
	if err := ctx.Err(); err != nil {
		t.Fatal(err)
	}
	if err := p.RetireSource(context.Background(), "file:invalid-first", 1); err != nil {
		t.Fatalf("could not retire producer after initial invalid selection: %v", err)
	}
	if !p.progress.OwnsSource("file:invalid-first") || p.progress.Pending() != 1 {
		t.Fatal("retirement removed the checkpoint barrier")
	}
	p.lifecycle.RLock()
	_, retained := p.sourceInputs[sourceInputKey{identifier: "file:invalid-first", generation: 1}]
	p.lifecycle.RUnlock()
	if retained {
		t.Fatal("retired producer retained its source input")
	}
}

func TestRoutedSelectionSeesProcessedStructuredRecordBeforeRendering(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		endpoint := config.NewEndpoint("test-key", "", "localhost", 12345, "", false)
		endpoints := config.NewEndpointsWithBatchSettings(endpoint, nil, false, true, time.Second, 1, 1, 1024, 2)
		cfg := configmock.New(t)
		cfg.SetInTest("logs_config.routing.max_groups", 4)
		sink := &routedTestSink{ch: make(chan *message.Payload, 1)}
		selected := make(chan bool, 1)
		p, err := NewRoutedProvider(RoutedOptions{
			NumberOfPipelines: 1, Sink: sink,
			DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
			Groups:                    []routing.Group{{ID: "a", Endpoints: endpoints}},
			ProcessingRules:           []*config.ProcessingRule{{Type: config.MaskSequences, Regex: regexp.MustCompile("secret"), Placeholder: []byte("masked")}},
			Generation:                func(*message.Message) (uint64, error) { return 1, nil },
			Resolve: func(m *message.Message) (routing.Selection, error) {
				vendor, ok := m.GetStructuredAttribute("device.vendor")
				selected <- m.State == message.StateStructured && string(m.GetContent()) == "masked" && ok && vendor == "example" && m.Origin.Source() == "mapped"
				return routing.Selection{Excluded: true, Generation: 1}, nil
			},
			DestinationsContext: client.NewDestinationsContext(), Config: cfg,
			Compression: compressionfx.NewMockCompressor(), Secrets: secretsnoopimpl.NewComponent().Comp,
		})
		if err != nil {
			t.Fatal(err)
		}
		input, err := p.SourceInput("file:test", 1)
		if err != nil {
			t.Fatal(err)
		}
		p.Start()
		defer p.Stop()
		src := sources.NewLogSource("test", &config.LogsConfig{ProcessingRules: []*config.ProcessingRule{{
			Type: config.RemapSource, Matching: []*config.SourceMatchEntry{{Attribute: "device.vendor", Value: "example", NewSource: "mapped"}},
		}}})
		m := message.NewStructuredMessage(&message.BasicStructuredContent{Data: map[string]interface{}{
			"message": "secret", "device": map[string]interface{}{"vendor": "example"},
		}}, message.NewOrigin(src), "info", 0)
		m.Origin.Identifier, m.Origin.Offset = "file:test", "1"
		input <- m
		synctest.Wait()
		select {
		case ok := <-selected:
			if !ok {
				t.Error("Resolve did not see processed structured fields before rendering/encoding")
			}
		default:
			t.Fatal("Resolve did not run")
		}
	})
}

func TestRoutedSelectionIdentityAndLocalOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name                                                     string
		shared, anonymous, identity, identityError, drop         bool
		generation                                               uint64
		selection                                                routing.Selection
		selectionError                                           bool
		wantSelections, wantPending, wantCheckpoints, wantErrors int
	}{
		{name: "bound identity", selection: routing.Selection{Excluded: true}, wantSelections: 1, wantCheckpoints: 1},
		{name: "shared identity", shared: true, identity: true, generation: 1, selection: routing.Selection{Excluded: true}, wantSelections: 1, wantCheckpoints: 1},
		{name: "missing shared identity", shared: true, wantPending: 1, wantErrors: 1},
		{name: "failed shared identity", shared: true, identity: true, identityError: true, wantPending: 1, wantErrors: 1},
		{name: "zero shared identity", shared: true, identity: true, wantPending: 1, wantErrors: 1},
		{name: "anonymous exclusion", shared: true, anonymous: true, selection: routing.Selection{Excluded: true}, wantSelections: 1},
		{name: "anonymous failure", shared: true, anonymous: true, selectionError: true, wantSelections: 1, wantErrors: 1},
		{name: "selection identity mismatch", selection: routing.Selection{Excluded: true, Generation: 2}, wantSelections: 1, wantPending: 1, wantErrors: 1},
		{name: "selection error", selectionError: true, wantSelections: 1, wantPending: 1, wantErrors: 1},
		{name: "unknown group", selection: routing.Selection{Groups: []string{"unknown"}}, wantSelections: 1, wantPending: 1, wantErrors: 1},
		{name: "exclusion with recipients", selection: routing.Selection{Excluded: true, Groups: []string{"a"}}, wantSelections: 1, wantPending: 1, wantErrors: 1},
		{name: "missing recipients", wantSelections: 1, wantPending: 1, wantErrors: 1},
		{name: "common processing drop", drop: true, wantCheckpoints: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				endpoint := config.NewEndpoint("test-key", "", "localhost", 12345, "", false)
				endpoints := config.NewEndpointsWithBatchSettings(endpoint, nil, false, true, time.Second, 1, 1, 1024, 2)
				cfg := configmock.New(t)
				cfg.SetInTest("logs_config.routing.max_groups", 4)
				sink := &routedTestSink{ch: make(chan *message.Payload, 1)}
				selections, identities, reports := 0, 0, 0
				opts := RoutedOptions{
					NumberOfPipelines: 1, Sink: sink,
					DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
					Groups:                    []routing.Group{{ID: "a", Endpoints: endpoints}},
					Resolve: func(*message.Message) (routing.Selection, error) {
						selections++
						if tc.selectionError {
							return routing.Selection{}, errors.New("selection failed")
						}
						return tc.selection, nil
					},
					OnError:             func(error) { reports++ },
					DestinationsContext: client.NewDestinationsContext(), Config: cfg,
					Compression: compressionfx.NewMockCompressor(), Secrets: secretsnoopimpl.NewComponent().Comp,
				}
				if tc.identity {
					opts.Generation = func(*message.Message) (uint64, error) {
						identities++
						if tc.identityError {
							return 0, errors.New("identity failed")
						}
						return tc.generation, nil
					}
				}
				if tc.drop {
					opts.ProcessingRules = []*config.ProcessingRule{{Type: config.ExcludeAtMatch, Regex: regexp.MustCompile("drop")}}
				}
				p, err := NewRoutedProvider(opts)
				if err != nil {
					t.Fatal(err)
				}
				input := p.NextPipelineChan()
				if !tc.shared {
					input, err = p.SourceInput("file:test", 1)
					if err != nil {
						t.Fatal(err)
					}
				}
				p.Start()
				defer p.Stop()
				m := message.NewMessageWithSource([]byte("drop"), "info", sources.NewLogSource("test", &config.LogsConfig{}), 0)
				if !tc.anonymous {
					m.Origin.Identifier, m.Origin.Offset = "file:test", "1"
				}
				input <- m
				synctest.Wait()
				if selections != tc.wantSelections || p.progress.Pending() != tc.wantPending || len(sink.ch) != tc.wantCheckpoints || reports != tc.wantErrors {
					t.Fatalf("selections/pending/checkpoints/errors = %d/%d/%d/%d, want %d/%d/%d/%d", selections, p.progress.Pending(), len(sink.ch), reports, tc.wantSelections, tc.wantPending, tc.wantCheckpoints, tc.wantErrors)
				}
				if tc.identity && !tc.anonymous && identities != 1 {
					t.Fatalf("identity calls = %d", identities)
				}
				if !tc.shared {
					if err := p.RetireSource(context.Background(), "file:test", 1); err != nil {
						t.Fatal(err)
					}
					if p.progress.OwnsSource("file:test") != (tc.wantPending != 0) {
						t.Fatal("retirement lost source barrier or retained completed source")
					}
				}
				for _, stream := range p.streams {
					if len(stream.input) != 0 {
						t.Fatal("excluded or invalid record entered batches")
					}
				}
			})
		})
	}
}

func TestRoutedPendingSelectionHoldsCheckpointsAndCancelledFlush(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		endpoint := config.NewEndpoint("test-key", "", "localhost", 12345, "", false)
		endpoints := config.NewEndpointsWithBatchSettings(endpoint, nil, false, true, time.Second, 1, 1, 1024, 2)
		cfg := configmock.New(t)
		cfg.SetInTest("logs_config.routing.max_groups", 4)
		sink := &routedTestSink{ch: make(chan *message.Payload, 2)}
		resolving, release := make(chan struct{}), make(chan struct{})
		selections := 0
		p, err := NewRoutedProvider(RoutedOptions{
			NumberOfPipelines: 2, Sink: sink,
			DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
			Groups:                    []routing.Group{{ID: "a", Endpoints: endpoints}},
			Generation:                func(*message.Message) (uint64, error) { return 1, nil },
			Resolve: func(m *message.Message) (routing.Selection, error) {
				if m.Origin.Offset == "1" {
					close(resolving)
					<-release
				}
				selections++
				return routing.Selection{Excluded: true}, nil
			},
			DestinationsContext: client.NewDestinationsContext(), Config: cfg,
			Compression: compressionfx.NewMockCompressor(), Secrets: secretsnoopimpl.NewComponent().Comp,
		})
		if err != nil {
			t.Fatal(err)
		}
		p.Start()
		defer p.Stop()
		defer func() {
			select {
			case <-release:
			default:
				close(release)
			}
		}()
		src := sources.NewLogSource("test", &config.LogsConfig{})
		for _, offset := range []string{"1", "2"} {
			m := message.NewMessageWithSource([]byte(offset), "info", src, 0)
			m.Origin.Identifier, m.Origin.Offset = "file:test", offset
			p.NextPipelineChan() <- m
		}
		<-resolving
		synctest.Wait()
		if p.progress.Pending() != 2 || len(sink.ch) != 0 || selections != 1 {
			t.Fatal("later completion passed pending selection")
		}
		ctx, cancel := context.WithCancel(context.Background())
		flushed := make(chan struct{})
		go func() { p.Flush(ctx); close(flushed) }()
		synctest.Wait()
		select {
		case <-flushed:
			t.Fatal("flush passed blocked selector")
		default:
		}
		cancel()
		<-flushed
		if p.progress.Pending() != 2 || len(sink.ch) != 0 {
			t.Fatal("cancelled flush acknowledged pending selection")
		}
		close(release)
		synctest.Wait()
		p.Flush(context.Background())
		synctest.Wait()
		if selections != 2 || p.progress.Pending() != 0 {
			t.Fatal("selection did not resume exactly once per record")
		}
		for _, want := range []string{"1", "2"} {
			select {
			case payload := <-sink.ch:
				if got := payload.MessageMetas[0].Origin.Offset; got != want {
					t.Fatalf("checkpoint=%s, want %s", got, want)
				}
			default:
				t.Fatal("completed checkpoint missing")
			}
		}
	})
}

func TestRoutedProviderRetirementCancelsSaturatedAdmission(t *testing.T) {
	for _, outcome := range []string{"cancel", "deadline", "stop"} {
		t.Run(outcome, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				endpoint := config.NewEndpoint("test-key", "", "localhost", 12345, "", false)
				endpoints := config.NewEndpointsWithBatchSettings(endpoint, nil, false, true, time.Second, 1, 1, 1024, 2)
				cfg := configmock.New(t)
				cfg.SetInTest("logs_config.routing.max_groups", 4)
				sink := &routedTestSink{ch: make(chan *message.Payload, 2)}
				p, err := NewRoutedProvider(RoutedOptions{
					NumberOfPipelines: 1, Sink: sink,
					DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
					Groups:                    []routing.Group{{ID: "a", Endpoints: endpoints}},
					Resolve:                   func(*message.Message) (routing.Selection, error) { return routing.Selection{Excluded: true}, nil },
					DestinationsContext:       client.NewDestinationsContext(), Config: cfg,
					Compression: compressionfx.NewMockCompressor(), Secrets: secretsnoopimpl.NewComponent().Comp,
				})
				if err != nil {
					t.Fatal(err)
				}
				input, err := p.SourceInput("file:test", 1)
				if err != nil {
					t.Fatal(err)
				}
				p.Start()
				defer p.Stop()
				source := sources.NewLogSource("test", &config.LogsConfig{Type: config.StringChannelType})
				makeRecord := func(offset string) *message.Message {
					m := message.NewMessageWithSource([]byte("pending"), "info", source, 0)
					m.Origin.Identifier, m.Origin.Offset = "file:test", offset
					return m
				}
				var pending []*message.Message
				for range routing.SourcePendingLimit {
					m := makeRecord("old")
					if _, err := p.progress.Register(context.Background(), m, routing.Selection{Generation: 1}, []string{"a"}); err != nil {
						t.Fatal(err)
					}
					pending = append(pending, m)
				}
				input <- makeRecord("old-last")
				synctest.Wait()
				if len(input) != 0 || p.progress.Pending() != routing.SourcePendingLimit {
					t.Fatal("admission is not blocked in Reserve")
				}
				var ctx context.Context
				var cancel context.CancelFunc
				if outcome == "deadline" {
					ctx, cancel = context.WithTimeout(context.Background(), time.Second)
				} else {
					ctx, cancel = context.WithCancel(context.Background())
				}
				defer cancel()
				retired := make(chan error, 1)
				go func() { retired <- p.RetireSource(ctx, "file:test", 1) }()
				synctest.Wait()
				select {
				case err := <-retired:
					t.Fatalf("retirement passed blocked admission: %v", err)
				default:
				}
				if outcome == "stop" {
					stopped := make(chan struct{})
					go func() { p.Stop(); close(stopped) }()
					if err := <-retired; !errors.Is(err, routing.ErrStopped) {
						t.Fatal(err)
					}
					<-stopped
					select {
					case <-sink.ch:
						t.Fatal("shutdown acknowledged pending records")
					default:
					}
					return
				}
				if outcome == "deadline" {
					<-ctx.Done()
				} else {
					cancel()
				}
				if err := <-retired; !errors.Is(err, ctx.Err()) {
					t.Fatalf("retirement = %v, want %v", err, ctx.Err())
				}
				if p.progress.Pending() != routing.SourcePendingLimit || !p.progress.OwnsSource("file:test") {
					t.Fatal("cancelled retirement released unresolved progress or ownership")
				}
				select {
				case <-sink.ch:
					t.Fatal("cancelled retirement acknowledged pending records")
				default:
				}
				// An already-cancelled call must return without inserting another marker.
				if err := p.RetireSource(ctx, "file:test", 1); !errors.Is(err, ctx.Err()) {
					t.Fatal(err)
				}
				p.lifecycle.RLock()
				retained := p.sourceInputs[sourceInputKey{"file:test", 1}] == input
				p.lifecycle.RUnlock()
				if !retained {
					t.Fatal("cancelled retirement closed the producer input")
				}
				replacement, err := p.SourceInput("file:test", 2)
				if err != nil {
					t.Fatal(err)
				}
				replacement <- makeRecord("replacement")
				for _, m := range pending {
					meta := m.MessageMetadata
					p.progress.CompleteGroup("a", message.NewPayload([]*message.MessageMetadata{&meta}, nil, "", 0))
				}
				synctest.Wait()
				retryCtx, retryCancel := context.WithTimeout(context.Background(), time.Second)
				defer retryCancel()
				if err := p.RetireSource(retryCtx, "file:test", 1); err != nil {
					t.Fatal(err)
				}
				if !p.progress.OwnsSource("file:test") {
					t.Fatal("old producer retirement released replacement ownership")
				}
				select {
				case payload := <-sink.ch:
					if got := payload.MessageMetas[0].Origin.Offset; got != "replacement" {
						t.Fatalf("obsolete checkpoint %q", got)
					}
				default:
					t.Fatal("replacement checkpoint did not arrive")
				}
				select {
				case <-sink.ch:
					t.Fatal("obsolete producer published another checkpoint")
				default:
				}
				if p.progress.Pending() != 0 {
					t.Fatal("recovery retained completed progress")
				}
				if err := p.RetireSource(retryCtx, "file:test", 2); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestRoutedProviderRetirementRetriesBusyCoordinator(t *testing.T) {
	for _, lock := range []string{"lifecycle writer", "lifecycle reader", "dispatch"} {
		t.Run(lock, func(t *testing.T) {
			progress, err := routing.NewProgress(4, &routedTestSink{})
			if err != nil {
				t.Fatal(err)
			}
			defer progress.Close()
			p := &RoutedProvider{
				progress:     progress,
				sourceInputs: make(map[sourceInputKey]chan *message.Message),
				inputKeys:    make(map[chan *message.Message]sourceInputKey),
			}
			if _, err := p.SourceInput("file:test", 1); err != nil {
				t.Fatal(err)
			}
			var unlock func()
			switch lock {
			case "lifecycle writer":
				p.lifecycle.Lock()
				unlock = p.lifecycle.Unlock
			case "lifecycle reader":
				p.lifecycle.RLock()
				unlock = p.lifecycle.RUnlock
			case "dispatch":
				p.sourceDispatchMu.Lock()
				unlock = p.sourceDispatchMu.Unlock
			}
			err = p.RetireSource(context.Background(), "file:test", 1)
			unlock()
			if !errors.Is(err, routing.ErrSourcePending) {
				t.Fatalf("busy retirement = %v", err)
			}
			if !progress.OwnsSource("file:test") {
				t.Fatal("busy retirement released ownership")
			}
			if err := p.RetireSource(context.Background(), "file:test", 1); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRoutedProviderRetirementRetriesBlockedAuditor(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		endpoint := config.NewEndpoint("test-key", "", "localhost", 12345, "", false)
		endpoints := config.NewEndpointsWithBatchSettings(endpoint, nil, false, true, time.Second, 1, 1, 1024, 2)
		sink := &routedTestSink{ch: make(chan *message.Payload)}
		p := newRoutedTestProvider(t, RoutedOptions{
			NumberOfPipelines: 1, Sink: sink,
			Groups:  []routing.Group{{ID: "a", Endpoints: endpoints}},
			Resolve: func(*message.Message) (routing.Selection, error) { return routing.Selection{Excluded: true}, nil },
		})
		input, err := p.SourceInput("file:retire", 1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.SourceInput("file:retire", 2); err != nil {
			t.Fatal(err)
		}
		p.Start()
		defer p.Stop()
		source := sources.NewLogSource("test", &config.LogsConfig{})
		published := make(chan error, 1)
		count := routedProgressCapacity + 2
		go func() {
			for i := 1; i <= count; i++ {
				m := message.NewMessageWithSource([]byte("complete"), "info", source, 0)
				m.Origin.Identifier, m.Origin.Offset = "file:publish", strconv.Itoa(i)
				if _, err := p.progress.Register(context.Background(), m, routing.Selection{Excluded: true, Generation: 1}, nil); err != nil {
					published <- err
					return
				}
			}
			published <- nil
		}()
		synctest.Wait()
		// The forwarder and its queue are full; publication holds the progress mutex.
		select {
		case err := <-published:
			t.Fatalf("publication did not backpressure: %v", err)
		default:
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := p.RetireSource(ctx, "file:retire", 1); !errors.Is(err, routing.ErrSourcePending) {
			t.Fatalf("blocked auditor retirement = %v", err)
		}
		if ctx.Err() != nil {
			t.Fatal("retirement waited for its deadline")
		}
		p.lifecycle.RLock()
		retained := p.sourceInputs[sourceInputKey{"file:retire", 1}] == input
		p.lifecycle.RUnlock()
		if !retained {
			t.Fatal("busy retirement closed the producer input")
		}
		drained := make(chan struct{})
		go func() {
			defer close(drained)
			for i := 1; i <= count; i++ {
				payload := <-sink.ch
				if got := payload.MessageMetas[0].Origin.Offset; got != strconv.Itoa(i) {
					t.Errorf("checkpoint=%s, want %d", got, i)
				}
			}
		}()
		<-drained
		if err := <-published; err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if err := p.progress.RegisterSource("file:retire", 1); err != nil {
			t.Fatalf("busy attempt retired the producer: %v", err)
		}
		if err := p.RetireSource(ctx, "file:retire", 1); err != nil {
			t.Fatal(err)
		}
		if !p.progress.OwnsSource("file:retire") {
			t.Fatal("old producer retirement released replacement ownership")
		}
		if err := p.RetireSource(ctx, "file:retire", 2); err != nil {
			t.Fatal(err)
		}
		if p.progress.OwnsSource("file:retire") {
			t.Fatal("last producer retirement retained ownership")
		}
	})
}

// A healthy source must make progress without waiting for the batch timer when
// its admission window is smaller than the configured batch size.
func TestRoutedProviderFlushesFullSourceWindow(t *testing.T) {
	for _, mode := range []string{"bound", "shared", "split", "overlap", "mixed", "shared-stream", "replacement", "excluded", "dropped", "invalid", "generation-error"} {
		t.Run(mode, func(t *testing.T) {
			requests := make(chan routedRequest, 2*routing.SourcePendingLimit)
			server, endpoints := routedTestEndpoint(t, "a", "key", requests)
			defer server.Close()
			endpoints.BatchWait = time.Hour // Only a pressure flush can deliver the records.
			endpoints.BatchMaxSize = 1000
			groups := []routing.Group{{ID: "a", Endpoints: endpoints}}
			if mode == "split" || mode == "overlap" || mode == "mixed" || mode == "shared-stream" {
				other, otherEndpoints := routedTestEndpoint(t, "b", "other-key", requests)
				defer other.Close()
				otherEndpoints.BatchWait, otherEndpoints.BatchMaxSize = time.Hour, 1000
				groups = append(groups, routing.Group{ID: "b", Endpoints: otherEndpoints})
				if mode == "mixed" {
					endpoints.BatchMaxSize = 1 // A completes immediately; B still needs pressure flushing.
				}
				if mode == "shared-stream" {
					groups[0].Stream, groups[1].Stream = "identical", "identical"
				}
			}
			sink := &routedTestSink{ch: make(chan *message.Payload, routing.SourcePendingLimit)}
			p := newRoutedTestProvider(t, RoutedOptions{
				NumberOfPipelines: 2, Sink: sink,
				Groups: groups, Generation: func(m *message.Message) (uint64, error) {
					if mode == "generation-error" && m.Origin.Offset == strconv.Itoa(routing.SourcePendingLimit) {
						return 0, errors.New("invalid generation")
					}
					return 1, nil
				},
				ProcessingRules: []*config.ProcessingRule{{Type: config.ExcludeAtMatch, Regex: regexp.MustCompile("^drop$")}},
				Resolve: func(m *message.Message) (routing.Selection, error) {
					if m.Origin.Offset == strconv.Itoa(routing.SourcePendingLimit) {
						if mode == "excluded" {
							return routing.Selection{Excluded: true}, nil
						}
						if mode == "invalid" {
							return routing.Selection{}, errors.New("invalid selection")
						}
					}
					if mode == "overlap" || mode == "mixed" || mode == "shared-stream" {
						return routing.Selection{Groups: []string{"a", "b"}}, nil
					}
					if mode == "split" && m.Origin.Offset != "1" {
						return routing.Selection{Groups: []string{"b"}}, nil
					}
					return routing.Selection{Groups: []string{"a"}}, nil
				},
				OnError: func(error) {},
			})
			p.Start()
			defer p.Stop()
			input := p.NextPipelineChan()
			var err error
			if mode != "shared" && mode != "generation-error" {
				input, err = p.SourceInput("file:pressure", 1)
				if err != nil {
					t.Fatal(err)
				}
			}
			source := sources.NewLogSource("pressure", &config.LogsConfig{})
			for i := 1; i <= routing.SourcePendingLimit; i++ {
				if mode == "replacement" && i == routing.SourcePendingLimit/2+1 {
					input, err = p.SourceInput("file:pressure", 2)
					if err != nil {
						t.Fatal(err)
					}
				}
				content := "record"
				if mode == "dropped" && i == routing.SourcePendingLimit {
					content = "drop"
				}
				m := message.NewMessageWithSource([]byte(content), "info", source, 0)
				m.Origin.Identifier, m.Origin.Offset = "file:pressure", strconv.Itoa(i)
				input <- m
			}
			// Pressure must deliver a prefix before the timer fires. Once that
			// frees admission, the remaining partial batch may wait normally;
			// explicitly flush it to check every outcome and checkpoint.
			first := 1
			if mode == "replacement" {
				first = routing.SourcePendingLimit/2 + 1
			}
			select {
			case checkpoint := <-sink.ch:
				if got := checkpoint.MessageMetas[0].Origin.Offset; got != strconv.Itoa(first) {
					t.Fatalf("first checkpoint=%s, want %d", got, first)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("healthy source window waited for the batch timer")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			p.Flush(ctx)
			if err := ctx.Err(); err != nil {
				t.Fatal(err)
			}
			want := routing.SourcePendingLimit
			if mode == "invalid" || mode == "generation-error" {
				want--
			}
			deadline := time.NewTimer(5 * time.Second)
			defer deadline.Stop()
			for i := first + 1; i <= want; i++ {
				select {
				case checkpoint := <-sink.ch:
					if got := checkpoint.MessageMetas[0].Origin.Offset; got != strconv.Itoa(i) {
						t.Fatalf("checkpoint=%s, want %d", got, i)
					}
				case <-deadline.C:
					t.Fatal("healthy source window waited for the batch timer")
				}
			}
			if (mode == "invalid" || mode == "generation-error") && p.progress.Pending() != 1 {
				t.Fatal("invalid selection lost its barrier")
			}
			if mode == "bound" {
				for range routing.SourcePendingLimit {
					request := <-requests
					if request.batchCount != routing.SourcePendingLimit {
						t.Fatalf("pressure fragmented the source window: batch has %d records, want %d", request.batchCount, routing.SourcePendingLimit)
					}
				}
			}
		})
	}
}

func TestRoutedPressureFlushDoesNotWaitForBlockedStream(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		endpoint := config.NewEndpoint("key", "", "localhost", 12345, "", false)
		endpoints := config.NewEndpointsWithBatchSettings(endpoint, nil, false, true, time.Hour, 1, 1000, 1024*1024, 2)
		p := newRoutedTestProvider(t, RoutedOptions{
			NumberOfPipelines: 1, Sink: &routedTestSink{ch: make(chan *message.Payload, routing.SourcePendingLimit)},
			Groups:  []routing.Group{{ID: "a", Endpoints: endpoints}, {ID: "b", Endpoints: endpoints}},
			Resolve: func(*message.Message) (routing.Selection, error) { return routing.Selection{}, nil },
		})
		defer p.Stop()
		blocked, healthy := p.streams["group:a"], p.streams["group:b"]
		for range cap(blocked.input) {
			blocked.input <- &message.Message{}
		}
		healthy.strategy.Start()
		defer healthy.strategy.Stop()
		var records []*message.Message
		source := sources.NewLogSource("pressure", &config.LogsConfig{})
		for i := range routing.SourcePendingLimit {
			m := message.NewMessageWithSource([]byte("record"), "info", source, 0)
			m.Origin.Identifier, m.Origin.Offset = "file:pressure", strconv.Itoa(i+1)
			if _, err := p.progress.Register(context.Background(), m, routing.Selection{Generation: 1}, []string{"a", "b"}); err != nil {
				t.Fatal(err)
			}
			records = append(records, m)
		}
		lane := &routedSourceDispatch{
			input:    make(chan *message.Message, routing.SourcePendingLimit),
			advanced: make(chan struct{}, 1), flushRequested: make(chan struct{}, 1),
		}
		p.sourceDone.Add(1)
		go p.dispatchSource("file:pressure", lane)
		defer func() { p.cancel(); close(lane.input); p.sourceDone.Wait() }()
		for _, m := range records {
			lane.input <- m
		}
		synctest.Wait()
		// The pressure boundary arrives after A has blocked fanout. B still
		// needs its partial batch flushed while the window waits for A.
		lane.flushAfter.Store(routing.SourcePendingLimit)
		lane.flushRequested <- struct{}{}
		select {
		case payload := <-healthy.output:
			if payload.Count() != 1 {
				t.Fatalf("healthy payload count=%d, want 1", payload.Count())
			}
			p.progress.CompleteGroup("b", payload)
			if p.progress.Pending() != routing.SourcePendingLimit {
				t.Fatal("pressure flush completed blocked group A")
			}
		case <-time.After(time.Second):
			t.Fatal("healthy stream pressure flush waited for blocked A")
		}
	})
}

func TestRoutedPressureFlushesAggregateCapacity(t *testing.T) {
	requests := make(chan routedRequest, 1)
	server, endpoints := routedTestEndpoint(t, "a", "key", requests)
	defer server.Close()
	endpoints.BatchWait, endpoints.BatchMaxSize = time.Hour, 10000
	p := newRoutedTestProvider(t, RoutedOptions{
		NumberOfPipelines: 1, Sink: &routedTestSink{ch: make(chan *message.Payload)},
		Groups: []routing.Group{{ID: "a", Endpoints: endpoints}},
		Resolve: func(*message.Message) (routing.Selection, error) {
			return routing.Selection{Groups: []string{"a"}}, nil
		},
	})
	defer p.Stop()
	source := sources.NewLogSource("pressure", &config.LogsConfig{})
	// Other outstanding anonymous records consume the aggregate budget without
	// a per-source window. The final reservation must request a flush as well.
	for range routedProgressCapacity - 1 {
		m := message.NewMessageWithSource([]byte("pending"), "info", source, 0)
		if _, err := p.progress.Register(context.Background(), m, routing.Selection{}, []string{"a"}); err != nil {
			t.Fatal(err)
		}
	}
	p.Start()
	p.NextPipelineChan() <- message.NewMessageWithSource([]byte("last"), "info", source, 0)
	select {
	case request := <-requests:
		if request.message != "last" {
			t.Fatalf("wrong record: %s", request.message)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("aggregate capacity waited for the batch timer")
	}
}

func TestRoutedAnonymousBacklogDoesNotBlockCheckpointedSource(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		endpoint := config.NewEndpoint("key", "", "localhost", 12345, "", false)
		endpoints := config.NewEndpointsWithBatchSettings(endpoint, nil, false, true, time.Second, 1, 1, 1024, 2)
		p := newRoutedTestProvider(t, RoutedOptions{
			NumberOfPipelines: 1, Sink: &routedTestSink{ch: make(chan *message.Payload, 1)},
			Groups:  []routing.Group{{ID: "a", Endpoints: endpoints}, {ID: "b", Endpoints: endpoints}},
			Resolve: func(*message.Message) (routing.Selection, error) { return routing.Selection{}, nil },
		})
		defer p.Stop()
		for range cap(p.streams["group:a"].input) {
			p.streams["group:a"].input <- &message.Message{}
		}
		go p.dispatchProcessed()
		defer func() { p.cancel(); close(p.processed); <-p.processedDone }()
		source := sources.NewLogSource("test", &config.LogsConfig{})
		record := func(identifier, group string) *message.Message {
			m := message.NewMessageWithSource([]byte("record"), "info", source, 0)
			m.Origin.Identifier = identifier
			if _, err := p.progress.Register(context.Background(), m, routing.Selection{Generation: 1}, []string{group}); err != nil {
				t.Fatal(err)
			}
			return m
		}
		// One record waits on A, 64 fill the old anonymous lane, and the next
		// must not trap the shared dispatcher ahead of an unrelated source.
		for range routing.SourcePendingLimit + 2 {
			p.processed <- record("", "a")
		}
		bound := record("file:healthy", "b")
		p.processed <- bound
		synctest.Wait()
		select {
		case got := <-p.streams["group:b"].input:
			if got != bound {
				t.Fatal("wrong source reached healthy B")
			}
		default:
			t.Fatal("anonymous backlog blocked an unrelated checkpointed source")
		}
	})
}
