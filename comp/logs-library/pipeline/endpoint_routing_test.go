// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pipeline

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	secretsnoopimpl "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/diagnostic"
	"github.com/DataDog/datadog-agent/comp/logs-library/routing"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	compressioncommon "github.com/DataDog/datadog-agent/pkg/util/compression"
	compressionselector "github.com/DataDog/datadog-agent/pkg/util/compression/selector"
)

type realGzipCompression struct{}

func (realGzipCompression) NewCompressor(kind string, level int) compressioncommon.Compressor {
	return compressionselector.NewCompressor(kind, level)
}

var _ logscompression.Component = realGzipCompression{}

type capturedRequest struct {
	encoding string
	records  []map[string]any
}

type mockEndpointIntake struct {
	server   *httptest.Server
	endpoint config.Endpoint

	mu         sync.Mutex
	statusCode int
	delay      time.Duration
	blockCh    chan struct{}
	requests   []capturedRequest
	notifyCh   chan struct{}
}

func newMockEndpointIntake(t *testing.T, apiKey string, useGzip bool) *mockEndpointIntake {
	t.Helper()
	m := &mockEndpointIntake{
		statusCode: http.StatusOK,
		notifyCh:   make(chan struct{}, 1),
	}
	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()

		m.mu.Lock()
		block := m.blockCh
		delay := m.delay
		code := m.statusCode
		m.mu.Unlock()

		if block != nil {
			select {
			case <-block:
			case <-r.Context().Done():
				return
			}
		}
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}

		// Re-read status code after unblock/delay so tests can switch status before unblocking.
		m.mu.Lock()
		code = m.statusCode
		m.mu.Unlock()

		var recs []map[string]any
		enc := r.Header.Get("Content-Encoding")
		body := raw
		if enc == "gzip" {
			gz, err := gzip.NewReader(strings.NewReader(string(raw)))
			if err == nil {
				body, _ = io.ReadAll(gz)
				_ = gz.Close()
			}
		}
		_ = json.Unmarshal(body, &recs)

		if code == http.StatusOK {
			m.mu.Lock()
			m.requests = append(m.requests, capturedRequest{encoding: enc, records: recs})
			m.mu.Unlock()
			select {
			case m.notifyCh <- struct{}{}:
			default:
			}
		}
		w.WriteHeader(code)
	}))
	t.Cleanup(m.server.Close)

	parsed, err := url.Parse(m.server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)
	ep := config.NewEndpoint(apiKey, "", parsed.Hostname(), port, "", false)
	ep.UseCompression = useGzip
	if useGzip {
		ep.CompressionKind = compressioncommon.GzipKind
		ep.CompressionLevel = 6
	}
	ep.BackoffBase = 0.02
	ep.BackoffFactor = 2
	ep.BackoffMax = 0.05
	m.endpoint = ep
	return m
}

func (m *mockEndpointIntake) setStatus(code int) {
	m.mu.Lock()
	m.statusCode = code
	m.mu.Unlock()
}

func (m *mockEndpointIntake) setBlock(ch chan struct{}) {
	m.mu.Lock()
	m.blockCh = ch
	m.mu.Unlock()
}

func (m *mockEndpointIntake) messages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, req := range m.requests {
		for _, rec := range req.records {
			if s, ok := rec["message"].(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func (m *mockEndpointIntake) records() []map[string]any {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []map[string]any
	for _, req := range m.requests {
		out = append(out, req.records...)
	}
	return out
}

func (m *mockEndpointIntake) snapshotRequests() []capturedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]capturedRequest(nil), m.requests...)
}

func waitForCondition(t *testing.T, desc string, cond func() bool) {
	t.Helper()
	require.Eventually(t, cond, 10*time.Second, 10*time.Millisecond, desc)
}

func newBatchTemplate(ep config.Endpoint, batchWait time.Duration, batchMaxSize int) *config.Endpoints {
	return config.NewEndpointsWithBatchSettings(ep, nil, false, true, batchWait, 2, batchMaxSize, 1024*1024, 64)
}

func TestEndpointRoutingUS1US5MembershipTagsStructuredAndLayoutB(t *testing.T) {
	us1 := newMockEndpointIntake(t, "us1-key", true)
	us5 := newMockEndpointIntake(t, "us5-key", true)

	policy, err := routing.NewEndpointPolicyFromTargets([]routing.EndpointTarget{
		{ID: "us1", Endpoint: us1.endpoint, Reliable: true, Evaluator: routing.MatchAllEvaluator{}},
		{
			ID:       "us5",
			Endpoint: us5.endpoint,
			Reliable: false,
			Evaluator: routing.EvaluatorFunc(func(v routing.RecordView) (bool, error) {
				if v.Status() == message.StatusNotice || v.Status() == message.StatusError {
					return true, nil
				}
				if sev, ok := v.StructuredAttribute("sev"); ok && sev == "notice" {
					return true, nil
				}
				if slices.Contains(v.LocalTags(), "internal_route:us5") && v.Source() == "remapped-svc" {
					return true, nil
				}
				return false, nil
			}),
		},
	}, newBatchTemplate(us1.endpoint, 2*time.Second, 64), 4, nil)
	require.NoError(t, err)

	rules := []*config.ProcessingRule{
		{Type: config.ExcludeAtMatch, Name: "drop_common", Pattern: "COMMON_DROP"},
		{Type: config.MaskSequences, Name: "mask_secret", Pattern: "secret=raw", ReplacePlaceholder: "secret=masked"},
		{Type: config.RemapSource, Name: "remap_src", Matching: []*config.SourceMatchEntry{{Attribute: "kind", Value: "audit", NewSource: "remapped-svc"}}},
	}
	err = config.CompileProcessingRules(rules)
	require.NoError(t, err)

	exportFilters, _ := (&config.TagFilters{Exclude: []string{"internal_route:*"}}).Compile()

	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.payload_channel_size", 32)
	destCtx := client.NewDestinationsContext()
	destCtx.Start()
	defer destCtx.Stop()

	sink := &routedTestSink{ch: make(chan *message.Payload, 256)}
	var lastOffset atomic.Value
	var sinkWg sync.WaitGroup
	sinkWg.Add(1)
	go func() {
		defer sinkWg.Done()
		for p := range sink.ch {
			for _, m := range p.MessageMetas {
				lastOffset.Store(m.Origin.Offset)
			}
		}
	}()

	provider, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		ProcessingRules:           rules,
		TagFilters:                exportFilters,
		EndpointPolicy:            policy,
		DestinationsContext:       destCtx,
		Config:                    cfg,
		Compression:               realGzipCompression{},
		Secrets:                   secretsnoopimpl.NewComponent().Comp,
	})
	require.NoError(t, err)
	provider.Start()
	defer func() {
		provider.Stop()
		close(sink.ch)
		sinkWg.Wait()
	}()

	src := sources.NewLogSource("app", &config.LogsConfig{
		Type:    config.FileType,
		Service: "billing",
		Source:  "orig-src",
		Tags:    []string{"env:prod", "internal_route:us5"},
	})
	ch, err := provider.SourceInput("file:/tmp/us1us5.log", 1)
	require.NoError(t, err)

	send := func(offset string, msg *message.Message) {
		msg.Origin.Identifier = "file:/tmp/us1us5.log"
		msg.Origin.Offset = offset
		ch <- msg
	}

	// 1. Common drop -> neither US1 nor US5 receives it.
	send("10", message.NewMessageWithSource([]byte("COMMON_DROP notice"), message.StatusNotice, src, 0))
	// 2. INFO -> US1 only (not US5).
	send("20", message.NewMessageWithSource([]byte("plain info secret=raw"), message.StatusInfo, src, 0))
	// 3. NOTICE -> US1 + US5 (masked).
	send("30", message.NewMessageWithSource([]byte("notice event secret=raw"), message.StatusNotice, src, 0))
	// 4. Structured JSON with remap_source + export-filtered local tag -> matches US5 via local tag & remapped source,
	//    while exported ddtags strips internal_route:us5!
	sc := &message.BasicStructuredContent{Data: map[string]any{"message": "audit event", "kind": "audit"}}
	send("40", message.NewStructuredMessage(sc, message.NewOrigin(src), message.StatusInfo, 0))

	provider.Flush(context.Background())

	waitForCondition(t, "US1 receives 3 surviving records", func() bool { return len(us1.messages()) == 3 })
	waitForCondition(t, "US5 receives 2 matching records", func() bool { return len(us5.messages()) == 2 })
	waitForCondition(t, "checkpoint reaches 40", func() bool { v, _ := lastOffset.Load().(string); return v == "40" })

	// Verify common masking applied and common drop removed from both.
	for _, msg := range us1.messages() {
		require.NotContains(t, msg, "COMMON_DROP")
		require.NotContains(t, msg, "secret=raw")
	}
	for _, msg := range us5.messages() {
		require.NotContains(t, msg, "COMMON_DROP")
		require.NotContains(t, msg, "plain info")
	}

	// Verify export tag filtering stripped internal_route:us5 from outgoing JSON while local tag matched!
	for _, rec := range us5.records() {
		tags, _ := rec["ddtags"].(string)
		require.Contains(t, tags, "env:prod")
		require.NotContains(t, tags, "internal_route:us5")
	}

	// Layout B check: 64-record window with sparse US5 matches -> US1 receives ONE full gzip batch of 64!
	beforeReqs := len(us1.snapshotRequests())
	for i := 0; i < 64; i++ {
		status := message.StatusInfo
		if i == 10 || i == 40 {
			status = message.StatusNotice
		}
		send(strconv.Itoa(100+i), message.NewMessageWithSource([]byte("win-"+strconv.Itoa(i)), status, src, 0))
	}
	waitForCondition(t, "US1 receives 64-record window", func() bool { return len(us1.messages()) == 3+64 })
	waitForCondition(t, "US5 receives 2 sparse records", func() bool { return len(us5.messages()) == 2+2 })
	reqs := us1.snapshotRequests()[beforeReqs:]
	require.Len(t, reqs, 1, "Layout B must not split primary batch on sparse secondary matches")
	require.Equal(t, "gzip", reqs[0].encoding)
	require.Len(t, reqs[0].records, 64)
}

func TestEndpointRoutingOptionalIsolationAndPrimaryOutage(t *testing.T) {
	us1 := newMockEndpointIntake(t, "us1-key", false)
	us5 := newMockEndpointIntake(t, "us5-key", false)

	// Block US5 completely so its optional queue saturates.
	us5Block := make(chan struct{})
	var unblockOnce sync.Once
	unblockUS5 := func() { unblockOnce.Do(func() { close(us5Block) }) }
	us5.setBlock(us5Block)

	policy, err := routing.NewEndpointPolicyFromTargets([]routing.EndpointTarget{
		{ID: "us1", Endpoint: us1.endpoint, Reliable: true, Evaluator: routing.MatchAllEvaluator{}},
		{ID: "us5", Endpoint: us5.endpoint, Reliable: false, Evaluator: routing.MatchAllEvaluator{}},
	}, newBatchTemplate(us1.endpoint, 50*time.Millisecond, 16), 4, nil)
	require.NoError(t, err)

	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.payload_channel_size", 4)
	destCtx := client.NewDestinationsContext()
	destCtx.Start()
	defer destCtx.Stop()

	sink := &routedTestSink{ch: make(chan *message.Payload, 512)}
	offsets := make(map[string]string)
	var offMu sync.Mutex
	var sinkWg sync.WaitGroup
	sinkWg.Add(1)
	go func() {
		defer sinkWg.Done()
		for p := range sink.ch {
			offMu.Lock()
			for _, m := range p.MessageMetas {
				offsets[m.Origin.Identifier] = m.Origin.Offset
			}
			offMu.Unlock()
		}
	}()
	getOffset := func(id string) string {
		offMu.Lock()
		defer offMu.Unlock()
		return offsets[id]
	}

	provider, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		EndpointPolicy:            policy,
		OptionalQueueSize:         2, // tiny optional queue to force saturation
		DestinationsContext:       destCtx,
		Config:                    cfg,
		Compression:               realGzipCompression{},
		Secrets:                   secretsnoopimpl.NewComponent().Comp,
	})
	require.NoError(t, err)
	provider.Start()
	defer func() {
		unblockUS5()
		provider.Stop()
		close(sink.ch)
		sinkWg.Wait()
	}()

	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType})
	chX, err := provider.SourceInput("file:/tmp/x.log", 1)
	require.NoError(t, err)

	// Send 100 records while US5 is completely blocked/saturated: US1 MUST deliver all 100 and checkpoint 100!
	for i := 1; i <= 100; i++ {
		m := message.NewMessageWithSource([]byte("msg-"+strconv.Itoa(i)), message.StatusNotice, src, 0)
		m.Origin.Identifier = "file:/tmp/x.log"
		m.Origin.Offset = strconv.Itoa(i)
		chX <- m
	}
	waitForCondition(t, "US1 delivers all 100 despite blocked US5", func() bool { return len(us1.messages()) == 100 })
	waitForCondition(t, "checkpoint advances to 100 despite blocked US5", func() bool { return getOffset("file:/tmp/x.log") == "100" })

	// Now unblock US5 and put US1 into 503 outage: US5 may receive copies, but checkpoint MUST HOLD at 100!
	us5.setBlock(nil)
	unblockUS5()
	us1.setStatus(http.StatusServiceUnavailable)
	for i := 101; i <= 116; i++ {
		m := message.NewMessageWithSource([]byte("outage-"+strconv.Itoa(i)), message.StatusNotice, src, 0)
		m.Origin.Identifier = "file:/tmp/x.log"
		m.Origin.Offset = strconv.Itoa(i)
		chX <- m
	}
	waitForCondition(t, "US5 receives records during US1 outage", func() bool { return len(us5.messages()) >= 16 })
	require.Equal(t, "100", getOffset("file:/tmp/x.log"), "US5 success must never advance pending US1 checkpoint")

	// Recover US1 -> delivers all 16 pending records and advances checkpoint to 116.
	us1.setStatus(http.StatusOK)
	waitForCondition(t, "US1 recovers and delivers all 116", func() bool { return len(us1.messages()) == 116 })
	waitForCondition(t, "checkpoint advances to 116 after US1 recovery", func() bool { return getOffset("file:/tmp/x.log") == "116" })
}

func TestEndpointRoutingPrimaryPredicateOptionalOnlyAndErrors(t *testing.T) {
	us1 := newMockEndpointIntake(t, "us1-key", false)
	us5 := newMockEndpointIntake(t, "us5-key", false)

	var optErrCount atomic.Int32
	policy, err := routing.NewEndpointPolicyFromTargets([]routing.EndpointTarget{
		{
			ID:       "us1",
			Endpoint: us1.endpoint,
			Reliable: true,
			Evaluator: routing.EvaluatorFunc(func(v routing.RecordView) (bool, error) {
				s := string(v.Content())
				if strings.Contains(s, "REL_ERR") {
					return false, errors.New("reliable eval error")
				}
				return strings.Contains(s, "US1"), nil
			}),
		},
		{
			ID:       "us5",
			Endpoint: us5.endpoint,
			Reliable: false,
			Evaluator: routing.EvaluatorFunc(func(v routing.RecordView) (bool, error) {
				s := string(v.Content())
				if strings.Contains(s, "OPT_ERR") {
					return false, errors.New("optional eval error")
				}
				return strings.Contains(s, "US5"), nil
			}),
		},
	}, newBatchTemplate(us1.endpoint, 50*time.Millisecond, 10), 4, nil)
	require.NoError(t, err)

	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.payload_channel_size", 16)
	destCtx := client.NewDestinationsContext()
	destCtx.Start()
	defer destCtx.Stop()

	sink := &routedTestSink{ch: make(chan *message.Payload, 64)}
	var lastOffset atomic.Value
	var sinkWg sync.WaitGroup
	sinkWg.Add(1)
	go func() {
		defer sinkWg.Done()
		for p := range sink.ch {
			for _, m := range p.MessageMetas {
				lastOffset.Store(m.Origin.Offset)
			}
		}
	}()

	provider, err := NewRoutedProvider(RoutedOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
		EndpointPolicy:            policy,
		OnOptionalError:           func(routing.OptionalEvalError) { optErrCount.Add(1) },
		DestinationsContext:       destCtx,
		Config:                    cfg,
		Compression:               realGzipCompression{},
		Secrets:                   secretsnoopimpl.NewComponent().Comp,
	})
	require.NoError(t, err)
	provider.Start()
	defer func() {
		provider.Stop()
		close(sink.ch)
		sinkWg.Wait()
	}()

	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType})
	ch, err := provider.SourceInput("file:/tmp/pred.log", 1)
	require.NoError(t, err)
	send := func(offset, text string) {
		m := message.NewMessageWithSource([]byte(text), message.StatusInfo, src, 0)
		m.Origin.Identifier = "file:/tmp/pred.log"
		m.Origin.Offset = offset
		ch <- m
	}

	// 1. Optional-only (US5 only, primary predicate false) -> US5 receives, checkpoint advances locally.
	send("10", "US5_ONLY")
	// 2. No-recipient -> neither receives, checkpoint advances locally.
	send("20", "NEITHER")
	// 3. Optional eval error + US1 match -> US1 receives, optional copy suppressed, checkpoint advances!
	send("30", "US1_OK OPT_ERR")
	provider.Flush(context.Background())

	waitForCondition(t, "US5 received optional-only", func() bool { return len(us5.messages()) == 1 })
	waitForCondition(t, "US1 received opt-err record", func() bool { return len(us1.messages()) == 1 })
	waitForCondition(t, "checkpoint reached 30", func() bool { v, _ := lastOffset.Load().(string); return v == "30" })
	require.Equal(t, int32(1), optErrCount.Load())

	// 4. Reliable eval error followed by valid US1 record -> checkpoint MUST HOLD at 30!
	send("40", "REL_ERR US5")
	send("50", "US1_AFTER_ERR")
	provider.Flush(context.Background())
	waitForCondition(t, "US1 received record after err", func() bool { return len(us1.messages()) == 2 })
	require.Equal(t, "30", lastOffset.Load().(string), "reliable evaluation error must hold source checkpoint")
	require.Equal(t, 1, len(us5.messages()), "reliable eval error must not partially deliver to US5")
}

func TestEndpointRoutingMultiReliableORD4EdgeCases(t *testing.T) {
	t.Run("400 on R1 plus 503 then recovery on R2 holds until R2 succeeds", func(t *testing.T) {
		r1 := newMockEndpointIntake(t, "r1-key", false)
		r2 := newMockEndpointIntake(t, "r2-key", false)
		r1.setStatus(http.StatusBadRequest)         // Permanent 400
		r2.setStatus(http.StatusServiceUnavailable) // Transient 503

		policy, err := routing.NewEndpointPolicyFromTargets([]routing.EndpointTarget{
			{ID: "r1", Endpoint: r1.endpoint, Reliable: true, Evaluator: routing.MatchAllEvaluator{}},
			{ID: "r2", Endpoint: r2.endpoint, Reliable: true, Evaluator: routing.MatchAllEvaluator{}},
		}, newBatchTemplate(r1.endpoint, 30*time.Millisecond, 10), 4, nil)
		require.NoError(t, err)

		cfg := configmock.New(t)
		cfg.SetInTest("logs_config.payload_channel_size", 8)
		destCtx := client.NewDestinationsContext()
		destCtx.Start()
		defer destCtx.Stop()

		sink := &routedTestSink{ch: make(chan *message.Payload, 32)}
		provider, err := NewRoutedProvider(RoutedOptions{
			NumberOfPipelines:         1,
			Sink:                      sink,
			DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
			EndpointPolicy:            policy,
			DestinationsContext:       destCtx,
			Config:                    cfg,
			Compression:               realGzipCompression{},
			Secrets:                   secretsnoopimpl.NewComponent().Comp,
		})
		require.NoError(t, err)
		provider.Start()
		defer func() {
			provider.Stop()
			close(sink.ch)
		}()

		src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType})
		ch, err := provider.SourceInput("file:/tmp/d4.log", 1)
		require.NoError(t, err)
		m := message.NewMessageWithSource([]byte("must-survive-r1-400"), message.StatusInfo, src, 0)
		m.Origin.Identifier = "file:/tmp/d4.log"
		m.Origin.Offset = "10"
		ch <- m

		// Wait briefly for R1 to get 400; checkpoint MUST NOT advance while R2 is retrying 503.
		time.Sleep(150 * time.Millisecond)
		select {
		case p := <-sink.ch:
			t.Fatalf("checkpoint advanced on R1 400 while R2 was retrying: %v", p.MessageMetas[0].Origin.Offset)
		default:
		}

		// Recover R2 -> delivers 200 -> checkpoint advances!
		r2.setStatus(http.StatusOK)
		select {
		case p := <-sink.ch:
			require.Equal(t, "10", p.MessageMetas[0].Origin.Offset)
		case <-time.After(5 * time.Second):
			t.Fatal("checkpoint did not advance after R2 recovered")
		}
		require.Equal(t, 1, len(r2.messages()))
	})

	t.Run("200 on R1 plus down R2 fills full batches and makes forward progress", func(t *testing.T) {
		r1 := newMockEndpointIntake(t, "r1-key", false)
		r2 := newMockEndpointIntake(t, "r2-key", false)
		r2.setStatus(http.StatusServiceUnavailable) // R2 down

		policy, err := routing.NewEndpointPolicyFromTargets([]routing.EndpointTarget{
			{ID: "r1", Endpoint: r1.endpoint, Reliable: true, Evaluator: routing.MatchAllEvaluator{}},
			{ID: "r2", Endpoint: r2.endpoint, Reliable: true, Evaluator: routing.MatchAllEvaluator{}},
		}, newBatchTemplate(r1.endpoint, 50*time.Millisecond, 64), 4, nil)
		require.NoError(t, err)

		cfg := configmock.New(t)
		cfg.SetInTest("logs_config.payload_channel_size", 4)
		destCtx := client.NewDestinationsContext()
		destCtx.Start()
		defer destCtx.Stop()

		sink := &routedTestSink{ch: make(chan *message.Payload, 512)}
		var count atomic.Int64
		var sinkWg sync.WaitGroup
		sinkWg.Add(1)
		go func() {
			defer sinkWg.Done()
			for p := range sink.ch {
				count.Add(p.Count())
			}
		}()

		provider, err := NewRoutedProvider(RoutedOptions{
			NumberOfPipelines:         1,
			Sink:                      sink,
			DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
			EndpointPolicy:            policy,
			DestinationsContext:       destCtx,
			Config:                    cfg,
			Compression:               realGzipCompression{},
			Secrets:                   secretsnoopimpl.NewComponent().Comp,
		})
		require.NoError(t, err)
		provider.Start()
		defer func() {
			provider.Stop()
			close(sink.ch)
			sinkWg.Wait()
		}()

		src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType})
		ch, err := provider.SourceInput("file:/tmp/r1fast.log", 1)
		require.NoError(t, err)

		// Send 200 records (> SourcePendingLimit=64): healthy R1 MUST deliver all 200 and advance checkpoint without stalling on down R2!
		for i := 1; i <= 200; i++ {
			m := message.NewMessageWithSource([]byte("fast-"+strconv.Itoa(i)), message.StatusInfo, src, 0)
			m.Origin.Identifier = "file:/tmp/r1fast.log"
			m.Origin.Offset = strconv.Itoa(i)
			ch <- m
		}
		waitForCondition(t, "R1 delivers all 200 despite down R2", func() bool { return len(r1.messages()) == 200 })
		waitForCondition(t, "all 200 checkpoints advance", func() bool { return count.Load() == 200 })
	})

	t.Run("all terminal loss (400 on R1 and 400 on R2) completes via local-loss policy", func(t *testing.T) {
		r1 := newMockEndpointIntake(t, "r1-key", false)
		r2 := newMockEndpointIntake(t, "r2-key", false)
		r1.setStatus(http.StatusBadRequest)
		r2.setStatus(http.StatusBadRequest)

		policy, err := routing.NewEndpointPolicyFromTargets([]routing.EndpointTarget{
			{ID: "r1", Endpoint: r1.endpoint, Reliable: true, Evaluator: routing.MatchAllEvaluator{}},
			{ID: "r2", Endpoint: r2.endpoint, Reliable: true, Evaluator: routing.MatchAllEvaluator{}},
		}, newBatchTemplate(r1.endpoint, 30*time.Millisecond, 10), 4, nil)
		require.NoError(t, err)

		cfg := configmock.New(t)
		cfg.SetInTest("logs_config.payload_channel_size", 8)
		destCtx := client.NewDestinationsContext()
		destCtx.Start()
		defer destCtx.Stop()

		sink := &routedTestSink{ch: make(chan *message.Payload, 16)}
		provider, err := NewRoutedProvider(RoutedOptions{
			NumberOfPipelines:         1,
			Sink:                      sink,
			DiagnosticMessageReceiver: &diagnostic.BufferedMessageReceiver{},
			EndpointPolicy:            policy,
			DestinationsContext:       destCtx,
			Config:                    cfg,
			Compression:               realGzipCompression{},
			Secrets:                   secretsnoopimpl.NewComponent().Comp,
		})
		require.NoError(t, err)
		provider.Start()
		defer func() {
			provider.Stop()
			close(sink.ch)
		}()

		src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType})
		ch, err := provider.SourceInput("file:/tmp/all400.log", 1)
		require.NoError(t, err)
		m := message.NewMessageWithSource([]byte("bad-payload"), message.StatusInfo, src, 0)
		m.Origin.Identifier = "file:/tmp/all400.log"
		m.Origin.Offset = "10"
		ch <- m

		select {
		case p := <-sink.ch:
			require.Equal(t, "10", p.MessageMetas[0].Origin.Offset)
		case <-time.After(5 * time.Second):
			t.Fatal("checkpoint did not advance after all reliable endpoints permanently rejected")
		}
	})
}
