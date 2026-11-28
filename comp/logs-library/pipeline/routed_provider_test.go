// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pipeline

import (
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
)

type routedTestSink struct{ ch chan *message.Payload }

func (s *routedTestSink) Channel() chan *message.Payload { return s.ch }

type routedRequest struct {
	endpoint   string
	message    string
	batchCount int
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
			requests <- routedRequest{endpoint: id, message: record.Message, batchCount: len(records)}
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

// newRoutedTestProvider supplies common dependencies for the lifecycle tests.
func newRoutedTestProvider(t *testing.T, opts RoutedOptions) *RoutedProvider {
	t.Helper()
	cfg := configmock.New(t)

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

var excludeAllEndpoints = routing.EvaluatorFunc(func(routing.RecordView) (bool, error) { return false, nil })

func TestRoutedProviderLocalLossesReleaseOnlyTheirRecords(t *testing.T) {
	requests := make(chan routedRequest, 2)
	server, endpoints := routedTestEndpoint(t, "a", "key-a", requests)
	defer server.Close()
	endpoints.BatchMaxContentSize = 1
	cfg := configmock.New(t)

	cfg.SetInTest("logs_config.payload_channel_size", 2)
	ctx := client.NewDestinationsContext()
	ctx.Start()
	sink := &routedTestSink{ch: make(chan *message.Payload, 2)}
	provider, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		ProcessingRules:           []*config.ProcessingRule{{Type: config.ExcludeAtMatch, Regex: regexp.MustCompile("^excluded$")}},
		Endpoints:                 []routing.EndpointTarget{{ID: "a", Endpoints: endpoints}},
		Generation:                func(*message.Message) (uint64, error) { return 1, nil },
		DestinationsContext:       ctx,
		Config:                    cfg,
		Compression:               compressionfx.NewMockCompressor(),
		Secrets:                   secretsnoopimpl.NewComponent().Comp,
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

func TestRoutedProviderStopReleasesBlockedSourceAdmission(t *testing.T) {
	requests := make(chan routedRequest, 1)
	server, endpoints := routedTestEndpoint(t, "a", "key-a", requests)
	defer server.Close()
	cfg := configmock.New(t)

	ctx := client.NewDestinationsContext()
	ctx.Start()
	defer ctx.Stop()
	sink := &routedTestSink{ch: make(chan *message.Payload, 1)}
	p, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines: 1, Sink: sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Endpoints:                 []routing.EndpointTarget{{ID: "a", Endpoints: endpoints}},
		Generation:                func(*message.Message) (uint64, error) { return 1, nil },
		DestinationsContext:       ctx, Config: cfg,
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
		if _, err := p.progress.Reserve(context.Background(), makeMessage(), 1); err != nil {
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

	cfg.SetInTest("logs_config.payload_channel_size", 2)
	ctx := client.NewDestinationsContext()
	ctx.Start()
	sink := &routedTestSink{ch: make(chan *message.Payload, 1)}
	provider, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines:         2,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Endpoints:                 []routing.EndpointTarget{{ID: "a", Endpoints: endpoints}},
		Generation:                func(*message.Message) (uint64, error) { return 1, nil },
		DestinationsContext:       ctx,
		Config:                    cfg,
		Compression:               compressionfx.NewMockCompressor(),
		Secrets:                   secretsnoopimpl.NewComponent().Comp,
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

func TestRoutedProviderRetiredRecordDoesNotBlockReplacement(t *testing.T) {
	endpoint := config.NewEndpoint("test-key", "", "localhost", 12345, "", false)
	endpoints := config.NewEndpointsWithBatchSettings(endpoint, nil, false, true, time.Second, 1, 1, 1024, 2)
	cfg := configmock.New(t)

	sink := &routedTestSink{ch: make(chan *message.Payload, 4)}
	reported := make(chan error, 1)
	p, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines: 1, Sink: sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Endpoints:                 []routing.EndpointTarget{{ID: "a", Endpoints: endpoints, Evaluator: excludeAllEndpoints}},
		Generation: func(m *message.Message) (uint64, error) {
			if string(m.GetContent()) == "old" {
				return 42, nil
			}
			return 7, nil
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

				sink := &routedTestSink{ch: make(chan *message.Payload, 4)}
				resolving := make(chan struct{})
				release := make(chan struct{})
				p, err := NewRoutedProvider(RoutedOptions{
					NumberOfPipelines: 1, Sink: sink,
					DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
					Endpoints: []routing.EndpointTarget{{ID: "a", Endpoints: endpoints, Evaluator: routing.EvaluatorFunc(func(view routing.RecordView) (bool, error) {
						if view.Status() == message.StatusError {
							close(resolving)
							<-release
						}
						return false, nil
					})}},
					Generation:          func(*message.Message) (uint64, error) { return 1, nil },
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
					status := message.StatusInfo
					if content == "old-first" {
						status = message.StatusError
					}
					m := message.NewMessageWithSource([]byte(content), status, source, time.Now().UnixNano())
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
				// Admission now reserves before endpoint evaluation runs.
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

	ctx := client.NewDestinationsContext()
	ctx.Start()
	defer ctx.Stop()
	sink := &routedTestSink{ch: make(chan *message.Payload, 4)}
	p, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines: 1, Sink: sink, DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Endpoints:           []routing.EndpointTarget{{ID: "a", Endpoints: endpoints}},
		Generation:          func(*message.Message) (uint64, error) { return 1, nil },
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

func TestRoutedProviderRetiresSourceWhoseFirstRecordIsInvalid(t *testing.T) {
	endpoint := config.NewEndpoint("key", "", "localhost", 12345, "", false)
	endpoints := config.NewEndpointsWithBatchSettings(endpoint, nil, false, true, time.Second, 1, 1, 1024, 2)
	cfg := configmock.New(t)

	p, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines: 1, Sink: &routedTestSink{ch: make(chan *message.Payload, 1)},
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		Endpoints:                 []routing.EndpointTarget{{ID: "a", Endpoints: endpoints, Evaluator: routing.EvaluatorFunc(func(routing.RecordView) (bool, error) { return false, errors.New("invalid selection") })}},
		Generation:                func(*message.Message) (uint64, error) { return 1, nil },
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

func TestRoutedPendingSelectionHoldsCheckpointsAndCancelledFlush(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		endpoint := config.NewEndpoint("test-key", "", "localhost", 12345, "", false)
		endpoints := config.NewEndpointsWithBatchSettings(endpoint, nil, false, true, time.Second, 1, 1, 1024, 2)
		cfg := configmock.New(t)

		sink := &routedTestSink{ch: make(chan *message.Payload, 2)}
		resolving, release := make(chan struct{}), make(chan struct{})
		var selections atomic.Int32
		p, err := NewRoutedProvider(RoutedOptions{
			NumberOfPipelines: 2, Sink: sink,
			DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
			Endpoints: []routing.EndpointTarget{{ID: "a", Endpoints: endpoints, Evaluator: routing.EvaluatorFunc(func(view routing.RecordView) (bool, error) {
				if view.Status() == message.StatusError {
					close(resolving)
					<-release
				}
				selections.Add(1)
				return false, nil
			})}},
			Generation:          func(*message.Message) (uint64, error) { return 1, nil },
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
			status := message.StatusInfo
			if offset == "1" {
				status = message.StatusError
			}
			m := message.NewMessageWithSource([]byte(offset), status, src, 0)
			m.Origin.Identifier, m.Origin.Offset = "file:test", offset
			p.NextPipelineChan() <- m
		}
		<-resolving
		synctest.Wait()
		if p.progress.Pending() != 2 || len(sink.ch) != 0 || selections.Load() != 1 {
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
		if selections.Load() != 2 || p.progress.Pending() != 0 {
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

				sink := &routedTestSink{ch: make(chan *message.Payload, 2)}
				p, err := NewRoutedProvider(RoutedOptions{
					NumberOfPipelines: 1, Sink: sink,
					DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
					Endpoints:                 []routing.EndpointTarget{{ID: "a", Endpoints: endpoints, Evaluator: excludeAllEndpoints}},
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
					if _, err := p.progress.Reserve(context.Background(), m, 1); err != nil {
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
					p.progress.CompleteLocal(m.RoutingToken())
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
			Endpoints: []routing.EndpointTarget{{ID: "a", Endpoints: endpoints}},
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
				token, err := p.progress.Reserve(context.Background(), m, 1)
				if err != nil {
					published <- err
					return
				}
				p.progress.CompleteLocal(token)
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

func TestRoutedPressureFlushesFullSourceWindow(t *testing.T) {
	for _, ingress := range []string{"bound", "shared"} {
		t.Run(ingress, func(t *testing.T) {
			requests := make(chan routedRequest, routing.SourcePendingLimit)
			server, endpoints := routedTestEndpoint(t, "a", "key", requests)
			defer server.Close()
			endpoints.BatchWait, endpoints.BatchMaxSize = time.Hour, 1000
			sink := &routedTestSink{ch: make(chan *message.Payload, routing.SourcePendingLimit)}
			p := newRoutedTestProvider(t, RoutedOptions{
				NumberOfPipelines: 2, Sink: sink,
				Endpoints:  []routing.EndpointTarget{{ID: "a", Endpoints: endpoints}},
				Generation: func(*message.Message) (uint64, error) { return 1, nil },
			})
			defer p.Stop()
			const id = "file:pressure"
			input := p.NextPipelineChan()
			if ingress == "bound" {
				var err error
				input, err = p.SourceInput(id, 1)
				if err != nil {
					t.Fatal(err)
				}
			}
			p.Start()
			source := sources.NewLogSource("pressure", &config.LogsConfig{})
			for i := 1; i <= routing.SourcePendingLimit; i++ {
				input <- newCheckpointedFilteringMessage(source, id, strconv.Itoa(i), "record", message.StatusInfo)
			}
			select {
			case checkpoint := <-sink.ch:
				if got := checkpoint.MessageMetas[0].Origin.Offset; got != "1" {
					t.Fatalf("first checkpoint = %q, want 1", got)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("full source window waited for the batch timer")
			}
			if ingress == "bound" {
				if got := (<-requests).batchCount; got != routing.SourcePendingLimit {
					t.Fatalf("bound window flushed %d records, want %d", got, routing.SourcePendingLimit)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			p.Flush(ctx)
			for i := 2; i <= routing.SourcePendingLimit; i++ {
				select {
				case checkpoint := <-sink.ch:
					if got := checkpoint.MessageMetas[0].Origin.Offset; got != strconv.Itoa(i) {
						t.Fatalf("checkpoint = %q, want %d", got, i)
					}
				case <-ctx.Done():
					t.Fatalf("waiting for checkpoint %d: %v", i, ctx.Err())
				}
			}
		})
	}
}

func TestRoutedPressureFlushesAggregateCapacity(t *testing.T) {
	requests := make(chan routedRequest, 1)
	server, endpoints := routedTestEndpoint(t, "a", "key", requests)
	defer server.Close()
	endpoints.BatchWait, endpoints.BatchMaxSize = time.Hour, 10000
	p := newRoutedTestProvider(t, RoutedOptions{
		NumberOfPipelines: 1, Sink: &routedTestSink{ch: make(chan *message.Payload)},
		Endpoints: []routing.EndpointTarget{{ID: "a", Endpoints: endpoints}},
	})
	defer p.Stop()
	source := sources.NewLogSource("pressure", &config.LogsConfig{})
	// Other outstanding anonymous records consume the aggregate budget without
	// a per-source window. The final reservation must request a flush as well.
	for range routedProgressCapacity - 1 {
		m := message.NewMessageWithSource([]byte("pending"), "info", source, 0)
		if _, err := p.progress.Reserve(context.Background(), m, 0); err != nil {
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
			Endpoints: []routing.EndpointTarget{{ID: "a", Endpoints: endpoints}, {ID: "b", Endpoints: endpoints}},
		})
		defer p.Stop()
		for range cap(p.streams["a"].input) {
			p.streams["a"].input <- &message.Message{}
		}
		go p.dispatchProcessed()
		defer func() { p.cancel(); close(p.processed); <-p.processedDone }()
		source := sources.NewLogSource("test", &config.LogsConfig{})
		record := func(identifier, endpointID string) *message.Message {
			m := message.NewMessageWithSource([]byte("record"), "info", source, 0)
			m.Origin.Identifier = identifier
			token, err := p.progress.Reserve(context.Background(), m, 1)
			if err != nil {
				t.Fatal(err)
			}
			if err := p.progress.AttachEndpointSelection(token, []string{endpointID}, nil, false); err != nil {
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
		case got := <-p.streams["b"].input:
			if got != bound {
				t.Fatal("wrong source reached healthy B")
			}
		default:
			t.Fatal("anonymous backlog blocked an unrelated checkpointed source")
		}
	})
}
