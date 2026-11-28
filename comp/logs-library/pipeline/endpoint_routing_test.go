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
	"regexp"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	secretsnoopimpl "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/diagnostic"
	"github.com/DataDog/datadog-agent/comp/logs-library/routing"
	"github.com/DataDog/datadog-agent/comp/logs-library/tagfilter"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
)

type endpointFilteringRecord struct {
	Message string `json:"message"`
	Status  string `json:"status"`
	DDTags  string `json:"ddtags"`
}

type endpointFilteringBatch struct {
	APIKey          string
	ContentEncoding string
	Records         []endpointFilteringRecord
}

type endpointFilteringIntake struct {
	server     *httptest.Server
	host       string
	port       int
	statusCode atomic.Int32
	shortReply atomic.Bool
	requests   atomic.Int32
	mu         sync.Mutex
	blockCh    chan struct{}
	batches    []endpointFilteringBatch
}

func newEndpointFilteringIntake(t *testing.T) *endpointFilteringIntake {
	t.Helper()
	intake := &endpointFilteringIntake{}
	intake.statusCode.Store(http.StatusOK)
	intake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		intake.requests.Add(1)

		intake.mu.Lock()
		ch := intake.blockCh
		intake.mu.Unlock()
		if ch != nil {
			select {
			case <-ch:
			case <-r.Context().Done():
				return
			}
		}

		code := int(intake.statusCode.Load())
		if r.Method == http.MethodGet && r.URL.Path == "/redirect-target" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if code == http.StatusFound {
			http.Redirect(w, r, "/redirect-target", code)
			return
		}
		if intake.shortReply.Load() {
			w.Header().Set("Content-Length", "1")
			w.WriteHeader(code)
			return
		}
		if code != http.StatusOK {
			w.WriteHeader(code)
			return
		}

		var reader io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				t.Errorf("gzip reader: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer gz.Close()
			reader = gz
		}
		body, err := io.ReadAll(reader)
		if err != nil {
			t.Errorf("read body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var records []endpointFilteringRecord
		if err := json.Unmarshal(body, &records); err != nil {
			t.Errorf("decode payload: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		intake.mu.Lock()
		intake.batches = append(intake.batches, endpointFilteringBatch{
			APIKey:          r.Header.Get("DD-API-KEY"),
			ContentEncoding: r.Header.Get("Content-Encoding"),
			Records:         records,
		})
		intake.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(func() {
		intake.unblock()
		intake.server.Close()
	})

	parsed, err := url.Parse(intake.server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)
	intake.host = parsed.Hostname()
	intake.port = port
	return intake
}

func (i *endpointFilteringIntake) setStatus(code int) {
	i.statusCode.Store(int32(code))
}

func (i *endpointFilteringIntake) block() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.blockCh == nil {
		i.blockCh = make(chan struct{})
	}
}

func (i *endpointFilteringIntake) unblock() {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.blockCh != nil {
		close(i.blockCh)
		i.blockCh = nil
	}
}

func (i *endpointFilteringIntake) endpoints(apiKey string, reliable bool) *config.Endpoints {
	return i.endpointsWithBatch(apiKey, reliable, 5*time.Second, 10)
}

func (i *endpointFilteringIntake) endpointsWithBatch(apiKey string, reliable bool, batchWait time.Duration, batchMaxSize int) *config.Endpoints {
	ep := config.NewMockEndpointWithOptions(map[string]any{
		"api_key":         apiKey,
		"host":            i.host,
		"port":            i.port,
		"use_ssl":         false,
		"use_compression": false,
		"is_reliable":     reliable,
	})
	ep.BackoffBase = 0.01
	ep.BackoffMax = 0.05
	ep.BackoffFactor = 1.5
	ep.RecoveryInterval = 1
	return config.NewEndpointsWithBatchSettings(ep, nil, false, true, batchWait, 2, batchMaxSize, 1024*1024, 16)
}

func (i *endpointFilteringIntake) snapshot() ([]string, []string, int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	var messages []string
	var keys []string
	for _, batch := range i.batches {
		keys = append(keys, batch.APIKey)
		for _, rec := range batch.Records {
			messages = append(messages, rec.Message)
		}
	}
	return messages, keys, len(i.batches)
}

func (i *endpointFilteringIntake) records() []endpointFilteringRecord {
	i.mu.Lock()
	defer i.mu.Unlock()
	var out []endpointFilteringRecord
	for _, batch := range i.batches {
		out = append(out, batch.Records...)
	}
	return out
}

func newCheckpointedFilteringMessage(src *sources.LogSource, identifier, offset, content, status string) *message.Message {
	origin := message.NewOrigin(src)
	origin.Identifier = identifier
	origin.Offset = offset
	return message.NewMessage([]byte(content), origin, status, time.Now().UnixNano())
}

func noticeAndHigherEvaluator() routing.Evaluator {
	return routing.EvaluatorFunc(func(view routing.RecordView) (bool, error) {
		switch view.Status() {
		case message.StatusNotice, message.StatusWarning, message.StatusError, message.StatusCritical:
			return true, nil
		default:
			return false, nil
		}
	})
}

type endpointFilteringCompression struct{}

func (endpointFilteringCompression) NewCompressor(kind string, level int) compression.Compressor {
	return selector.NewCompressor(kind, level)
}

func newEndpointFilteringTestProvider(t *testing.T, opts RoutedOptions) *RoutedProvider {
	t.Helper()
	cfg := configmock.New(t)
	cfg.SetInTest("logs_config.payload_channel_size", 4)
	opts.Config = cfg
	opts.DiagnosticMessageReceiver = &diagnostic.BufferedMessageReceiver{}
	opts.DestinationsContext = client.NewDestinationsContext()
	opts.Compression = endpointFilteringCompression{}
	opts.Secrets = secretsnoopimpl.NewComponent().Comp
	if opts.NumberOfPipelines == 0 {
		opts.NumberOfPipelines = 1
	}
	p, err := NewRoutedProvider(opts)
	require.NoError(t, err)
	return p
}

// TestEndpointFilteringUS1FullAndUS5FilteredBatchesWithCommonProcessing verifies
// the core Confluence v3 two-intake contract:
// - US1 (reliable, unfiltered) receives all surviving records in one Layout B batch.
// - US5 (unreliable, NOTICE-and-higher selector) receives only matching copies in its own batch.
// - Common exclude and mask rules run before endpoint selection and apply to every copy.
// - Source checkpoints advance in order for all records, including common drops.
func TestEndpointFilteringUS1FullAndUS5FilteredBatchesWithCommonProcessing(t *testing.T) {
	us1 := newEndpointFilteringIntake(t)
	us5 := newEndpointFilteringIntake(t)

	sink := &routedTestSink{ch: make(chan *message.Payload, 16)}
	commonRules := []*config.ProcessingRule{
		{
			Type:  config.ExcludeAtMatch,
			Name:  "drop_common",
			Regex: regexp.MustCompile("DROP_ME"),
		},
		{
			Type:        config.MaskSequences,
			Name:        "mask_secret",
			Regex:       regexp.MustCompile("SECRET_[0-9]+"),
			Placeholder: []byte("[MASKED]"),
		},
	}

	us5Endpoints := us5.endpoints("us5-key", false)
	us5Endpoints.Main.UseCompression = true
	us5Endpoints.Main.CompressionKind = compression.GzipKind
	us5Endpoints.Main.CompressionLevel = gzip.BestSpeed
	us5Endpoints.Endpoints[0] = us5Endpoints.Main

	p := newEndpointFilteringTestProvider(t, RoutedOptions{
		Sink:            sink,
		ProcessingRules: commonRules,
		Endpoints: []routing.EndpointTarget{
			{ID: "us1", Endpoints: us1.endpoints("us1-key", true)},
			{ID: "us5", Endpoints: us5Endpoints, Evaluator: noticeAndHigherEvaluator()},
		},
	})
	p.Start()
	defer p.Stop()

	const sourceID = "file:/tmp/endpoint-filtering.log"
	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/endpoint-filtering.log"})
	in, err := p.SourceInput(sourceID, 1)
	require.NoError(t, err)

	// Layout B example flow with common mask and common drop:
	// 10: INFO -> masked -> US1 only
	// 20: NOTICE -> masked -> US1 + US5
	// 30: INFO -> US1 only
	// 40: NOTICE with DROP_ME -> dropped by common processing (neither US1 nor US5)
	in <- newCheckpointedFilteringMessage(src, sourceID, "10", "a-info token=SECRET_1", message.StatusInfo)
	in <- newCheckpointedFilteringMessage(src, sourceID, "20", "b-notice token=SECRET_2", message.StatusNotice)
	in <- newCheckpointedFilteringMessage(src, sourceID, "30", "c-info", message.StatusInfo)
	in <- newCheckpointedFilteringMessage(src, sourceID, "40", "DROP_ME token=SECRET_3", message.StatusNotice)

	p.Flush(context.Background())

	require.Eventually(t, func() bool {
		us1Msgs, _, _ := us1.snapshot()
		us5Msgs, _, _ := us5.snapshot()
		return len(us1Msgs) == 3 && len(us5Msgs) == 1
	}, 5*time.Second, 10*time.Millisecond)

	us1Msgs, us1Keys, us1Batches := us1.snapshot()
	us5Msgs, us5Keys, us5Batches := us5.snapshot()

	assert.Equal(t, []string{
		"a-info token=[MASKED]",
		"b-notice token=[MASKED]",
		"c-info",
	}, us1Msgs)
	assert.Equal(t, 1, us1Batches, "Layout B must deliver all surviving primary records in one batch")
	assert.Equal(t, []string{"us1-key"}, us1Keys)

	assert.Equal(t, []string{
		"b-notice token=[MASKED]",
	}, us5Msgs)
	assert.Equal(t, 1, us5Batches)
	assert.Equal(t, []string{"us5-key"}, us5Keys)

	us1.mu.Lock()
	assert.Equal(t, "identity", us1.batches[0].ContentEncoding)
	us1.mu.Unlock()
	us5.mu.Lock()
	assert.Equal(t, "gzip", us5.batches[0].ContentEncoding)
	us5.mu.Unlock()

	var offsets []string
	deadline := time.After(5 * time.Second)
	for len(offsets) < 4 {
		select {
		case cp := <-sink.ch:
			for _, meta := range cp.MessageMetas {
				offsets = append(offsets, meta.Origin.Offset)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for ordered checkpoints, got %v", offsets)
		}
	}
	assert.Equal(t, []string{"10", "20", "30", "40"}, offsets)
}

// TestEndpointFilteringLocalTagsWithExportTagFilter verifies
// that selectors match unfiltered local tags before rendering, while export
// tag_filters still strip those tags on the wire.
func TestEndpointFilteringLocalTagsWithExportTagFilter(t *testing.T) {
	us1 := newEndpointFilteringIntake(t)
	us5 := newEndpointFilteringIntake(t)

	// Export tag_filter strips "internal_route:us5" from outgoing wire payloads.
	globalFilters, _ := tagfilter.Compile(nil, []string{"internal_route:*"})

	us5Selector := routing.EvaluatorFunc(func(view routing.RecordView) (bool, error) {
		return slices.Contains(view.LocalTags(), "internal_route:us5"), nil
	})

	sink := &routedTestSink{ch: make(chan *message.Payload, 16)}
	p := newEndpointFilteringTestProvider(t, RoutedOptions{
		Sink:       sink,
		TagFilters: globalFilters,
		Endpoints: []routing.EndpointTarget{
			{ID: "us1", Endpoints: us1.endpoints("us1-key", true)},
			{ID: "us5", Endpoints: us5.endpoints("us5-key", false), Evaluator: us5Selector},
		},
	})
	p.Start()
	defer p.Stop()

	const sourceID = "file:/tmp/local-tags.log"
	src := sources.NewLogSource("app", &config.LogsConfig{
		Type: config.FileType,
		Path: "/tmp/local-tags.log",
		Tags: []string{"env:prod", "internal_route:us5"},
	})
	in, err := p.SourceInput(sourceID, 1)
	require.NoError(t, err)

	in <- newCheckpointedFilteringMessage(src, sourceID, "10", "audit-event", message.StatusNotice)
	noMatch := newCheckpointedFilteringMessage(src, sourceID, "20", "regular-event", message.StatusNotice)
	noMatch.Origin = message.NewOrigin(sources.NewLogSource("app", &config.LogsConfig{Tags: []string{"env:prod"}}))
	noMatch.Origin.Identifier = sourceID
	noMatch.Origin.Offset = "20"
	in <- noMatch
	p.Flush(context.Background())

	require.Eventually(t, func() bool {
		return len(us1.records()) == 2 && len(us5.records()) == 1
	}, 5*time.Second, 10*time.Millisecond)

	for _, rec := range us5.records() {
		assert.NotContains(t, rec.DDTags, "internal_route:us5", "export tag_filter must still strip excluded tag from outgoing payload")
		assert.Contains(t, rec.DDTags, "env:prod")
		assert.Equal(t, "audit-event", rec.Message)
	}
}

// TestEndpointFilteringIsolatesOptionalFailureAndSaturation verifies that US5
// HTTP 503 failures and slow/saturated optional queues drop copies without
// blocking healthy US1 delivery or stalling US1 checkpoints.
func TestEndpointFilteringIsolatesOptionalFailureAndSaturation(t *testing.T) {
	us1 := newEndpointFilteringIntake(t)
	us5 := newEndpointFilteringIntake(t)
	us5.block() // Stall US5 HTTP handler completely to saturate optional sender queues.

	const total = 96 // More records than the configured queues can retain for US5.
	sink := &routedTestSink{ch: make(chan *message.Payload, total)}
	p := newEndpointFilteringTestProvider(t, RoutedOptions{
		Sink: sink,
		Endpoints: []routing.EndpointTarget{
			{ID: "us1", Endpoints: us1.endpointsWithBatch("us1-key", true, 10*time.Millisecond, 1)},
			{ID: "us5", Endpoints: us5.endpointsWithBatch("us5-key", false, 10*time.Millisecond, 1), Evaluator: noticeAndHigherEvaluator()},
		},
	})
	p.Start()
	defer p.Stop()

	const sourceID = "file:/tmp/us5-saturated.log"
	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/us5-saturated.log"})
	in, err := p.SourceInput(sourceID, 1)
	require.NoError(t, err)

	in <- newCheckpointedFilteringMessage(src, sourceID, "10", "notice-1", message.StatusNotice)
	require.Eventually(t, func() bool { return us5.requests.Load() > 0 }, 5*time.Second, 10*time.Millisecond)
	admissionDeadline := time.After(5 * time.Second)
	for i := 2; i <= total; i++ {
		select {
		case in <- newCheckpointedFilteringMessage(src, sourceID, strconv.Itoa(i*10), "notice-"+strconv.Itoa(i), message.StatusNotice):
		case <-admissionDeadline:
			t.Fatalf("source admission stalled at record %d", i)
		}
	}

	// Healthy US1 must deliver all records and advance all checkpoints even while US5 is completely stalled.
	require.Eventually(t, func() bool {
		us1Msgs, _, _ := us1.snapshot()
		return len(us1Msgs) == total
	}, 5*time.Second, 10*time.Millisecond)

	var offsets []string
	deadline := time.After(5 * time.Second)
	for len(offsets) < total {
		select {
		case cp := <-sink.ch:
			for _, meta := range cp.MessageMetas {
				offsets = append(offsets, meta.Origin.Offset)
			}
		case <-deadline:
			t.Fatalf("US1 checkpoints stalled while US5 blocked; got %d/%d", len(offsets), total)
		}
	}
	for i, offset := range offsets {
		assert.Equal(t, strconv.Itoa((i+1)*10), offset)
	}
	optional, _, _ := us5.snapshot()
	assert.Empty(t, optional, "US5 must still be blocked when all US1 checkpoints arrive")
}

func TestEndpointFilteringFlushDoesNotWaitForOptionalHandoff(t *testing.T) {
	reliable := make(chan *message.Message, 1)
	optional := make(chan *message.Message, 1)
	p := &RoutedProvider{
		ctx:             context.Background(),
		started:         true,
		stopRequested:   make(chan struct{}),
		streams:         map[string]*routedStream{"reliable": {input: reliable}},
		optionalStreams: map[string]*routedStream{"optional": {input: optional}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	flushed := make(chan struct{})
	go func() {
		defer close(flushed)
		p.Flush(ctx)
	}()

	takeMarker := func(input <-chan *message.Message) *message.Message {
		t.Helper()
		select {
		case marker := <-input:
			return marker
		case <-ctx.Done():
			t.Fatal("batcher did not receive a flush marker")
			return nil
		}
	}
	reliableMarker := takeMarker(reliable)
	optionalMarker := takeMarker(optional)
	defer close(optionalMarker.RoutingBarrier())

	select {
	case <-flushed:
		t.Fatal("flush returned before reliable handoff")
	case <-time.After(50 * time.Millisecond):
	}
	close(reliableMarker.RoutingBarrier())
	select {
	case <-flushed:
		require.NoError(t, ctx.Err())
	case <-time.After(time.Second):
		t.Fatal("flush waited for optional handoff")
	}
}

func TestEndpointFilteringFlushOffersFinalMarkersIndependently(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	healthy := make(chan *message.Message, 1)
	stalled := make(chan *message.Message, 1)
	stalled <- &message.Message{}
	p := &RoutedProvider{
		ctx: context.Background(), endpointDefs: &routing.EndpointDefinitions{},
		stopRequested: make(chan struct{}),
		streams: map[string]*routedStream{
			"stalled": {input: stalled},
			"healthy": {input: healthy},
		},
	}
	done := make(chan struct{})
	go p.flushPrior(ctx, done)
	select {
	case marker := <-healthy:
		close(marker.RoutingBarrier())
	case <-ctx.Done():
		t.Fatal("a full reliable queue blocked the healthy stream's final marker")
	}
	select {
	case <-done:
		t.Fatal("flush returned before the stalled stream acknowledged")
	default:
	}
	cancel()
	<-done
}

func TestEndpointFilteringFlushReleasesPartialBatchesDuringStalledFanout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	progress, err := routing.NewProgress(8, &routedTestSink{})
	require.NoError(t, err)
	healthy := make(chan *message.Message, 1)
	healthyFlush := make(chan struct{}, 1)
	stalled := make(chan *message.Message, 1)
	stalled <- &message.Message{} // Leave no queue space for this reliable sibling.
	p := &RoutedProvider{
		ctx: ctx, progress: progress,
		processed: make(chan *message.Message, 3), processedDone: make(chan struct{}),
		sourceDispatch: make(map[string]*routedSourceDispatch),
		streams: map[string]*routedStream{
			"healthy": {input: healthy, flush: healthyFlush, pressureFlush: true},
			"stalled": {input: stalled, flush: make(chan struct{}, 1), pressureFlush: true},
		},
	}
	go p.dispatchProcessed()
	defer func() {
		cancel()
		close(p.processed)
		<-p.processedDone
		progress.Close()
	}()
	const sourceID = "file:/tmp/flush-stalled.log"
	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/flush-stalled.log"})
	for i := 1; i <= 2; i++ {
		msg := newCheckpointedFilteringMessage(src, sourceID, strconv.Itoa(i), "log", message.StatusInfo)
		token, err := progress.Reserve(ctx, msg, 1)
		require.NoError(t, err)
		require.NoError(t, progress.AttachEndpointSelection(token, []string{"healthy", "stalled"}, nil, false))
		p.processed <- msg
	}
	ack := make(chan struct{})
	marker := &message.Message{}
	marker.SetRoutingBarrier(ack)
	marker.SetRoutingBarrierContext(ctx)
	p.processed <- marker

	for delivered := 0; delivered < 2; {
		// Do not drain the only data slot until the batcher gets a flush signal.
		select {
		case <-healthyFlush:
		case <-ctx.Done():
			t.Fatal("full healthy queue lost its flush request")
		}
		select {
		case msg := <-healthy:
			require.Nil(t, msg.RoutingBarrier())
			progress.CompleteReliableSuccess("healthy", message.NewPayload([]*message.MessageMetadata{&msg.MessageMetadata}, nil, "", 0))
			delivered++
		default:
			// A repeated signal can arrive after the previous record has drained.
		}
	}
	select {
	case <-ack:
		require.NoError(t, ctx.Err())
	case <-ctx.Done():
		t.Fatal("flush dispatch barrier did not complete")
	}
}

func TestEndpointFilteringFlushSignalsHealthyStreamWhileAnotherSourceStalls(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	flush := make(chan struct{}, 1)
	healthy := &routedSourceDispatch{submitted: 1, input: make(chan *message.Message)}
	healthy.completed.Store(1)
	stalled := &routedSourceDispatch{
		submitted: 1, input: make(chan *message.Message),
		advanced: make(chan struct{}, 1), flushRequested: make(chan struct{}, 1),
	}
	p := &RoutedProvider{
		ctx:       ctx,
		processed: make(chan *message.Message, 1), processedDone: make(chan struct{}),
		sourceDispatch: map[string]*routedSourceDispatch{"healthy": healthy, "stalled": stalled},
		streams: map[string]*routedStream{
			"healthy": {pressureFlush: true, flush: flush},
		},
	}
	go p.dispatchProcessed()
	defer func() {
		cancel()
		close(p.processed)
		<-p.processedDone
	}()

	ack := make(chan struct{})
	marker := &message.Message{}
	marker.SetRoutingBarrier(ack)
	marker.SetRoutingBarrierContext(ctx)
	p.processed <- marker
	select {
	case <-flush:
	case <-ctx.Done():
		t.Fatal("healthy stream got no early flush request")
	}
	select {
	case <-ack:
		t.Fatal("barrier acknowledged before the stalled source completed")
	default:
	}
	stalled.completed.Store(1)
	stalled.advanced <- struct{}{}
	select {
	case <-ack:
		require.NoError(t, ctx.Err())
	case <-ctx.Done():
		t.Fatal("barrier did not acknowledge after the stalled source completed")
	}
}

// TestEndpointFilteringStalledReliableSibling verifies that delivery to one
// matching endpoint releases offers still waiting for space at its sibling.
func TestEndpointFilteringStalledReliableSibling(t *testing.T) {
	healthy := newEndpointFilteringIntake(t)
	stalled := newEndpointFilteringIntake(t)
	stalled.block()

	const total = 4 * routedStreamCapacity
	sink := &routedTestSink{ch: make(chan *message.Payload, total)}
	p := newEndpointFilteringTestProvider(t, RoutedOptions{
		Sink: sink,
		Endpoints: []routing.EndpointTarget{
			{ID: "healthy", Endpoints: healthy.endpointsWithBatch("healthy-key", true, 10*time.Millisecond, 4)},
			{ID: "stalled", Endpoints: stalled.endpointsWithBatch("stalled-key", true, 10*time.Millisecond, 2)},
		},
	})
	p.Start()
	defer p.Stop()
	defer stalled.unblock()

	const sourceID = "file:/tmp/stalled-reliable.log"
	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/stalled-reliable.log"})
	in, err := p.SourceInput(sourceID, 1)
	require.NoError(t, err)
	feedCtx, cancel := context.WithCancel(context.Background())
	feedDone := make(chan struct{})
	go func() {
		defer close(feedDone)
		for i := 1; i <= total; i++ {
			offset := strconv.Itoa(i)
			select {
			case in <- newCheckpointedFilteringMessage(src, sourceID, offset, offset, message.StatusInfo):
			case <-feedCtx.Done():
				return
			}
		}
	}()
	defer func() {
		cancel()
		<-feedDone
	}()

	stalledInput := p.streams["stalled"].input
	require.Eventually(t, func() bool {
		return stalled.requests.Load() > 0 && len(stalledInput) == cap(stalledInput) && len(sink.ch) > 0
	}, 10*time.Second, 10*time.Millisecond, "the stalled queue must fill after healthy checkpoints advance")
	deadline := time.After(15 * time.Second)
	for i := 1; i <= total; i++ {
		select {
		case checkpoint := <-sink.ch:
			require.Equal(t, strconv.Itoa(i), checkpoint.MessageMetas[0].Origin.Offset)
		case <-deadline:
			t.Fatalf("checkpoint stalled at record %d", i)
		}
	}
	healthyLogs, _, _ := healthy.snapshot()
	require.Len(t, healthyLogs, total)
	queued := len(stalledInput)
	require.Positive(t, queued)
	stalled.unblock()
	require.Eventually(t, func() bool {
		logs, _, _ := stalled.snapshot()
		return len(logs) >= queued
	}, 10*time.Second, 10*time.Millisecond, "copies already in the queue must still send")
}

// TestEndpointFilteringPrimaryOutageHoldsCheckpointThenRecovers verifies that
// US5 optional delivery never advances source progress while US1 is retrying,
// and that US1 recovery delivers and advances the ordered checkpoint.
func TestEndpointFilteringPrimaryOutageHoldsCheckpointThenRecovers(t *testing.T) {
	us1 := newEndpointFilteringIntake(t)
	us5 := newEndpointFilteringIntake(t)
	us1.setStatus(http.StatusServiceUnavailable)

	sink := &routedTestSink{ch: make(chan *message.Payload, 16)}
	p := newEndpointFilteringTestProvider(t, RoutedOptions{
		Sink: sink,
		Endpoints: []routing.EndpointTarget{
			{ID: "us1", Endpoints: us1.endpointsWithBatch("us1-key", true, 10*time.Millisecond, 1)},
			{ID: "us5", Endpoints: us5.endpointsWithBatch("us5-key", false, 10*time.Millisecond, 1), Evaluator: noticeAndHigherEvaluator()},
		},
	})
	p.Start()
	defer p.Stop()

	const sourceID = "file:/tmp/us1-outage.log"
	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/us1-outage.log"})
	in, err := p.SourceInput(sourceID, 1)
	require.NoError(t, err)

	in <- newCheckpointedFilteringMessage(src, sourceID, "10", "during-outage", message.StatusNotice)

	// US5 receives the best-effort copy while US1 is returning 503.
	require.Eventually(t, func() bool {
		us5Msgs, _, _ := us5.snapshot()
		return len(us5Msgs) == 1 && us1.requests.Load() > 0
	}, 5*time.Second, 10*time.Millisecond)

	// US5 success MUST NOT advance the source checkpoint.
	select {
	case cp := <-sink.ch:
		t.Fatalf("optional US5 delivery must not advance checkpoint, got %v", cp.MessageMetas)
	case <-time.After(150 * time.Millisecond):
	}

	// Recover US1; delivery succeeds and advances the checkpoint.
	us1.setStatus(http.StatusOK)
	select {
	case cp := <-sink.ch:
		require.Len(t, cp.MessageMetas, 1)
		assert.Equal(t, "10", cp.MessageMetas[0].Origin.Offset)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for checkpoint after US1 recovery")
	}
}

// TestEndpointFilteringMultiReliableTriesAllAndSeparatesRejectionFromSuccess
// verifies that every matching reliable endpoint is tried, a permanent 400
// rejection on R1 does not falsely satisfy the reliable-set rule while R2 is
// retrying 503, and R2's 200 delivery satisfies the reliable-set rule.
func TestEndpointFilteringMultiReliableTriesAllAndSeparatesRejectionFromSuccess(t *testing.T) {
	r1 := newEndpointFilteringIntake(t)
	r2 := newEndpointFilteringIntake(t)
	r1.setStatus(http.StatusBadRequest)         // Permanent rejection.
	r2.setStatus(http.StatusServiceUnavailable) // Transient retryable failure.

	sink := &routedTestSink{ch: make(chan *message.Payload, 16)}
	p := newEndpointFilteringTestProvider(t, RoutedOptions{
		Sink: sink,
		Endpoints: []routing.EndpointTarget{
			{ID: "r1", Endpoints: r1.endpointsWithBatch("r1-key", true, 10*time.Millisecond, 1)},
			{ID: "r2", Endpoints: r2.endpointsWithBatch("r2-key", true, 10*time.Millisecond, 1)},
		},
	})
	p.Start()
	defer p.Stop()

	const sourceID = "file:/tmp/multi-reliable.log"
	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/multi-reliable.log"})
	in, err := p.SourceInput(sourceID, 1)
	require.NoError(t, err)

	in <- newCheckpointedFilteringMessage(src, sourceID, "10", "must-deliver", message.StatusError)

	// Both reliable endpoints must be tried.
	require.Eventually(t, func() bool {
		return r1.requests.Load() >= 1 && r2.requests.Load() >= 1
	}, 5*time.Second, 10*time.Millisecond)

	// R1's permanent 400 rejection must NOT advance the checkpoint while R2 is still retrying.
	select {
	case cp := <-sink.ch:
		t.Fatalf("permanent 400 rejection must not satisfy reliable-set rule, got %v", cp.MessageMetas)
	case <-time.After(150 * time.Millisecond):
	}

	// Once R2 succeeds with 200, the reliable-set rule ("at least one successful delivery") is satisfied.
	r2.setStatus(http.StatusOK)
	select {
	case cp := <-sink.ch:
		require.Len(t, cp.MessageMetas, 1)
		assert.Equal(t, "10", cp.MessageMetas[0].Origin.Offset)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for checkpoint after R2 delivery")
	}
}

func TestEndpointFilteringDoesNotConfirmDeliveryFromIncompleteResponse(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		shortReply bool
	}{
		{name: "redirect", status: http.StatusFound},
		{name: "truncated_success_response", status: http.StatusOK, shortReply: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			intake := newEndpointFilteringIntake(t)
			intake.setStatus(tc.status)
			intake.shortReply.Store(tc.shortReply)
			sink := &routedTestSink{ch: make(chan *message.Payload, 1)}
			p := newEndpointFilteringTestProvider(t, RoutedOptions{
				Sink: sink,
				Endpoints: []routing.EndpointTarget{{
					ID: "reliable", Endpoints: intake.endpointsWithBatch("key", true, 10*time.Millisecond, 1),
				}},
			})
			p.Start()
			defer p.Stop()

			const sourceID = "file:/tmp/redirect.log"
			src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/redirect.log"})
			in, err := p.SourceInput(sourceID, 1)
			require.NoError(t, err)
			in <- newCheckpointedFilteringMessage(src, sourceID, "10", "must-post", message.StatusInfo)
			require.Eventually(t, func() bool { return intake.requests.Load() >= 2 }, 5*time.Second, 10*time.Millisecond)
			select {
			case <-sink.ch:
				t.Fatal("incomplete response advanced the checkpoint")
			default:
			}

			intake.setStatus(http.StatusOK)
			intake.shortReply.Store(false)
			select {
			case cp := <-sink.ch:
				require.Equal(t, "10", cp.MessageMetas[0].Origin.Offset)
				logs, _, _ := intake.snapshot()
				require.Equal(t, []string{"must-post"}, logs)
			case <-time.After(5 * time.Second):
				t.Fatal("missing checkpoint after successful POST")
			}
		})
	}
}

func TestEndpointFilteringEdgeCases(t *testing.T) {
	t.Run("reliable_selector_error_blocks_checkpoint_after_known_rejection", func(t *testing.T) {
		rejected := newEndpointFilteringIntake(t)
		rejected.setStatus(http.StatusBadRequest)
		healthy := newEndpointFilteringIntake(t)
		isFirst := func(v routing.RecordView) bool { return v.Status() == message.StatusError }

		sink := &routedTestSink{ch: make(chan *message.Payload, 16)}
		p := newEndpointFilteringTestProvider(t, RoutedOptions{
			Sink: sink,
			Endpoints: []routing.EndpointTarget{
				{ID: "rejected", Endpoints: rejected.endpointsWithBatch("rejected-key", true, 10*time.Millisecond, 1), Evaluator: routing.EvaluatorFunc(func(v routing.RecordView) (bool, error) {
					return isFirst(v), nil
				})},
				{ID: "healthy", Endpoints: healthy.endpointsWithBatch("healthy-key", true, 10*time.Millisecond, 1), Evaluator: routing.EvaluatorFunc(func(v routing.RecordView) (bool, error) {
					return !isFirst(v), nil
				})},
				{ID: "unknown", Endpoints: healthy.endpointsWithBatch("unknown-key", true, 10*time.Millisecond, 1), Evaluator: routing.EvaluatorFunc(func(v routing.RecordView) (bool, error) {
					if isFirst(v) || v.Status() == message.StatusWarning {
						return false, errors.New("selector failed")
					}
					return false, nil
				})},
			},
		})
		p.Start()
		defer p.Stop()

		const sourceID = "file:/tmp/mixed-reliable-outcomes.log"
		const otherID = "file:/tmp/mixed-reliable-success.log"
		src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/mixed-reliable-outcomes.log"})
		other, err := p.SourceInput(otherID, 1)
		require.NoError(t, err)
		other <- newCheckpointedFilteringMessage(src, otherID, "5", "delivered", message.StatusWarning)
		select {
		case cp := <-sink.ch:
			require.Len(t, cp.MessageMetas, 1)
			require.Equal(t, otherID, cp.MessageMetas[0].Origin.Identifier)
			require.Equal(t, "5", cp.MessageMetas[0].Origin.Offset)
		case <-time.After(5 * time.Second):
			t.Fatal("reliable success did not resolve the selector error")
		}

		in, err := p.SourceInput(sourceID, 1)
		require.NoError(t, err)
		in <- newCheckpointedFilteringMessage(src, sourceID, "10", "first", message.StatusError)
		in <- newCheckpointedFilteringMessage(src, sourceID, "20", "later", message.StatusInfo)
		require.Eventually(t, func() bool {
			logs, _, _ := healthy.snapshot()
			return rejected.requests.Load() == 1 && len(logs) == 2
		}, 5*time.Second, 10*time.Millisecond)

		select {
		case cp := <-sink.ch:
			t.Fatalf("checkpoint passed the unresolved selector: %v", cp.MessageMetas)
		case <-time.After(150 * time.Millisecond):
		}
	})
	t.Run("skip_empty_and_complete_optional_only_before_reliable_error", func(t *testing.T) {
		us1 := newEndpointFilteringIntake(t)
		us5 := newEndpointFilteringIntake(t)

		var errCount atomic.Int32
		sink := &routedTestSink{ch: make(chan *message.Payload, 16)}
		p := newEndpointFilteringTestProvider(t, RoutedOptions{
			Sink: sink,
			OnError: func(error) {
				errCount.Add(1)
			},
			Endpoints: []routing.EndpointTarget{
				{
					ID:        "us1",
					Endpoints: us1.endpointsWithBatch("us1-key", true, 10*time.Millisecond, 1),
					Evaluator: routing.EvaluatorFunc(func(v routing.RecordView) (bool, error) {
						if v.Status() == message.StatusError {
							return false, errors.New("boom")
						}
						return v.Status() == message.StatusWarning, nil
					}),
				},
				{
					ID:        "us5",
					Endpoints: us5.endpointsWithBatch("us5-key", false, 10*time.Millisecond, 1),
					Evaluator: routing.EvaluatorFunc(func(v routing.RecordView) (bool, error) {
						if v.Status() == message.StatusWarning {
							return false, errors.New("optional selector error")
						}
						return v.Status() == message.StatusNotice, nil
					}),
				},
			},
		})
		p.Start()
		defer p.Stop()

		const sourceID = "file:/tmp/open-edge-cases.log"
		src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/open-edge-cases.log"})
		in, err := p.SourceInput(sourceID, 1)
		require.NoError(t, err)

		in <- newCheckpointedFilteringMessage(src, sourceID, "10", "empty-match", message.StatusInfo)
		in <- newCheckpointedFilteringMessage(src, sourceID, "20", "optional-only", message.StatusNotice)
		in <- newCheckpointedFilteringMessage(src, sourceID, "30", "match-us1", message.StatusWarning)
		in <- newCheckpointedFilteringMessage(src, sourceID, "40", "eval-err", message.StatusError)
		in <- newCheckpointedFilteringMessage(src, sourceID, "50", "match-us1", message.StatusWarning)

		deadline := time.After(5 * time.Second)
		for _, offset := range []string{"10", "20", "30"} {
			select {
			case cp := <-sink.ch:
				require.Equal(t, offset, cp.MessageMetas[0].Origin.Offset)
			case <-deadline:
				t.Fatalf("missing checkpoint %s", offset)
			}
		}
		require.Eventually(t, func() bool {
			primary, _, _ := us1.snapshot()
			optional, _, _ := us5.snapshot()
			return errCount.Load() == 3 && len(primary) == 2 && len(optional) == 1
		}, 5*time.Second, 10*time.Millisecond)
		optional, _, _ := us5.snapshot()
		require.Equal(t, []string{"optional-only"}, optional)

		select {
		case cp := <-sink.ch:
			t.Fatalf("open edge cases must not advance checkpoints, got %v", cp.MessageMetas)
		case <-time.After(150 * time.Millisecond):
		}
	})

	t.Run("all_rejected_reliable_advances_checkpoint_as_drop", func(t *testing.T) {
		r1 := newEndpointFilteringIntake(t)
		r2 := newEndpointFilteringIntake(t)
		r1.setStatus(http.StatusBadRequest)
		r2.setStatus(http.StatusBadRequest)

		sink := &routedTestSink{ch: make(chan *message.Payload, 16)}
		p := newEndpointFilteringTestProvider(t, RoutedOptions{
			Sink: sink,
			Endpoints: []routing.EndpointTarget{
				{ID: "r1", Endpoints: r1.endpointsWithBatch("r1-key", true, 10*time.Millisecond, 1)},
				{ID: "r2", Endpoints: r2.endpointsWithBatch("r2-key", true, 10*time.Millisecond, 1)},
			},
		})
		p.Start()
		defer p.Stop()

		const sourceID = "file:/tmp/all-rejected.log"
		src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/all-rejected.log"})
		in, err := p.SourceInput(sourceID, 1)
		require.NoError(t, err)

		in <- newCheckpointedFilteringMessage(src, sourceID, "10", "rejected-everywhere", message.StatusError)

		require.Eventually(t, func() bool {
			return r1.requests.Load() >= 1 && r2.requests.Load() >= 1
		}, 5*time.Second, 10*time.Millisecond)

		select {
		case cp := <-sink.ch:
			require.Len(t, cp.MessageMetas, 1)
			require.Equal(t, "10", cp.MessageMetas[0].Origin.Offset)
		case <-time.After(5 * time.Second):
			t.Fatal("missing checkpoint after permanent rejection from both endpoints")
		}
	})
}
