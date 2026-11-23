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
	"net"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	secretsnoop "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/diagnostic"
	"github.com/DataDog/datadog-agent/comp/logs-library/routing"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type efIntake struct {
	server   *httptest.Server
	host     string
	port     int
	mu       sync.Mutex
	batches  [][]map[string]any
	status   atomic.Int32
	blockCh  chan struct{}
	blocking atomic.Bool
}

func newEFIntake(t *testing.T) *efIntake {
	t.Helper()
	intake := &efIntake{}
	intake.status.Store(http.StatusAccepted)
	intake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if intake.blocking.Load() {
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
		}
		code := int(intake.status.Load())
		if code != http.StatusAccepted && code != http.StatusOK {
			w.WriteHeader(code)
			return
		}
		var reader io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			defer gz.Close()
			reader = gz
		}
		raw, err := io.ReadAll(reader)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var batch []map[string]any
		if err := json.Unmarshal(raw, &batch); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		intake.mu.Lock()
		intake.batches = append(intake.batches, batch)
		intake.mu.Unlock()
		w.WriteHeader(code)
	}))
	t.Cleanup(intake.server.Close)
	host, rawPort, err := net.SplitHostPort(intake.server.Listener.Addr().String())
	require.NoError(t, err)
	port, err := strconv.Atoi(rawPort)
	require.NoError(t, err)
	intake.host = host
	intake.port = port
	return intake
}

func (i *efIntake) setStatus(code int) {
	i.status.Store(int32(code))
}

func (i *efIntake) setBlocking(block bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if block {
		if i.blockCh == nil {
			i.blockCh = make(chan struct{})
		}
		i.blocking.Store(true)
	} else {
		i.blocking.Store(false)
		if i.blockCh != nil {
			close(i.blockCh)
			i.blockCh = nil
		}
	}
}

func (i *efIntake) messages() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	var out []string
	for _, batch := range i.batches {
		for _, rec := range batch {
			if msg, ok := rec["message"].(string); ok {
				out = append(out, msg)
			}
		}
	}
	return out
}

func (i *efIntake) batchCount() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return len(i.batches)
}

func (i *efIntake) allRecords() []map[string]any {
	i.mu.Lock()
	defer i.mu.Unlock()
	var out []map[string]any
	for _, batch := range i.batches {
		out = append(out, batch...)
	}
	return out
}

func (i *efIntake) endpoint(apiKey string, reliable bool) config.Endpoint {
	ep := config.NewMockEndpointWithOptions(map[string]any{
		"api_key":         apiKey,
		"host":            i.host,
		"port":            i.port,
		"use_ssl":         false,
		"use_compression": true,
		"is_reliable":     reliable,
	})
	ep.CompressionKind = "gzip"
	ep.CompressionLevel = 1
	ep.BackoffBase = 0.01
	ep.BackoffMax = 0.05
	ep.BackoffFactor = 1.5
	ep.RecoveryInterval = 1
	return ep
}

type efSink struct {
	ch chan *message.Payload
}

func (s *efSink) Channel() chan *message.Payload { return s.ch }

func efTestConfig(t *testing.T) pkgconfigmodel.BuildableConfig {
	t.Helper()
	cfg := configmock.New(t)
	cfg.Set("logs_config.payload_channel_size", 4, pkgconfigmodel.SourceAgentRuntime)
	cfg.Set("logs_config.http_timeout", 2, pkgconfigmodel.SourceAgentRuntime)
	return cfg
}

func makeCheckpointedMessage(src *sources.LogSource, id, offset, content, status string) *message.Message {
	origin := message.NewOrigin(src)
	origin.Identifier = id
	origin.Offset = offset
	return message.NewMessage([]byte(content), origin, status, 0)
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for condition: %s", msg)
}

func TestEndpointFilteredUS1ReliableUS5FilteredUnreliableLayoutB(t *testing.T) {
	us1 := newEFIntake(t)
	us5 := newEFIntake(t)

	mainEP := us1.endpoint("us1-key", true)
	us5EP := us5.endpoint("us5-key", false)
	eps := config.NewEndpointsWithBatchSettings(mainEP, []config.Endpoint{us5EP}, false, true, 5*time.Second, 2, 10, 1024*1024, 16)

	noticeOnly := routing.SelectorFunc(func(v routing.LogView) (bool, error) {
		return v.Status() == message.StatusNotice || v.Status() == message.StatusError, nil
	})

	sink := &efSink{ch: make(chan *message.Payload, 16)}
	cfg := efTestConfig(t)
	commonRules := []*config.ProcessingRule{
		{Type: config.ExcludeAtMatch, Name: "drop_secret", Regex: regexp.MustCompile("DROP_ME")},
	}

	destCtx := client.NewDestinationsContext()
	p, err := NewEndpointFilteredProvider(EndpointFilteredOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		ProcessingRules:           commonRules,
		Endpoints:                 eps,
		Selectors:                 []routing.EndpointSelector{nil, noticeOnly},
		DestinationsContext:       destCtx,
		Config:                    cfg,
		Compression:               logscompression.NewMockCompressor(),
		Secrets:                   secretsnoop.NewComponent().Comp,
	})
	require.NoError(t, err)
	p.Start()
	defer p.Stop()

	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/app.log"})
	in := p.NextPipelineChan()

	// Layout B example flow from Confluence:
	// a: INFO -> US1 only
	// b: NOTICE -> US1 + US5
	// c: INFO -> US1 only
	// d: DROP_ME (NOTICE) -> dropped by common processing (neither US1 nor US5)
	in <- makeCheckpointedMessage(src, "file:/tmp/app.log", "10", "a-info", message.StatusInfo)
	in <- makeCheckpointedMessage(src, "file:/tmp/app.log", "20", "b-notice", message.StatusNotice)
	in <- makeCheckpointedMessage(src, "file:/tmp/app.log", "30", "c-info", message.StatusInfo)
	in <- makeCheckpointedMessage(src, "file:/tmp/app.log", "40", "DROP_ME-notice", message.StatusNotice)

	p.Flush(context.Background())

	waitForCondition(t, 5*time.Second, func() bool {
		return len(us1.messages()) == 3 && len(us5.messages()) == 1
	}, "US1=3 and US5=1")

	assert.Equal(t, []string{"a-info", "b-notice", "c-info"}, us1.messages())
	assert.Equal(t, []string{"b-notice"}, us5.messages())
	// Layout B: US1 received all 3 records in a single batch request (not split!).
	assert.Equal(t, 1, us1.batchCount(), "Layout B must not split primary batch on sparse secondary matches")
	assert.Equal(t, 1, us5.batchCount())

	// All 4 records (including common drop) must advance checkpoints in order.
	var offsets []string
	for len(offsets) < 4 {
		select {
		case cp := <-sink.ch:
			for _, m := range cp.MessageMetas {
				offsets = append(offsets, m.Origin.Offset)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for checkpoints, got %v", offsets)
		}
	}
	assert.Equal(t, []string{"10", "20", "30", "40"}, offsets)
}

func TestEndpointFilteredLocalTagsStructuredFieldsAndOutgoingTagFilter(t *testing.T) {
	us1 := newEFIntake(t)
	us5 := newEFIntake(t)

	eps := config.NewEndpointsWithBatchSettings(
		us1.endpoint("us1-key", true),
		[]config.Endpoint{us5.endpoint("us5-key", false)},
		false, true, 5*time.Second, 2, 10, 1024*1024, 16,
	)

	// Match on local tag (even though tag_filters removes it before export) AND structured attribute.
	us5Selector := routing.SelectorFunc(func(v routing.LogView) (bool, error) {
		if !slices.Contains(v.Tags(), "route_target:us5") {
			return false, nil
		}
		sev, ok := v.StructuredAttribute("severity")
		return ok && sev == "notice", nil
	})

	tagFilters, _ := (&config.TagFilters{Exclude: []string{"route_target:*"}}).Compile()
	sink := &efSink{ch: make(chan *message.Payload, 8)}

	p, err := NewEndpointFilteredProvider(EndpointFilteredOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		TagFilters:                tagFilters,
		Endpoints:                 eps,
		Selectors:                 []routing.EndpointSelector{nil, us5Selector},
		DestinationsContext:       client.NewDestinationsContext(),
		Config:                    efTestConfig(t),
		Compression:               logscompression.NewMockCompressor(),
		Secrets:                   secretsnoop.NewComponent().Comp,
	})
	require.NoError(t, err)
	p.Start()
	defer p.Stop()

	src := sources.NewLogSource("payments", &config.LogsConfig{
		Service: "payments",
		Source:  "go",
		Tags:    []string{"env:prod", "route_target:us5"},
	})
	msg1 := message.NewStructuredMessage(
		&message.BasicStructuredContent{Data: map[string]any{"message": "matched-structured", "severity": "notice"}},
		message.NewOrigin(src), message.StatusNotice, 0,
	)
	msg2 := message.NewStructuredMessage(
		&message.BasicStructuredContent{Data: map[string]any{"message": "unmatched-structured", "severity": "info"}},
		message.NewOrigin(src), message.StatusInfo, 0,
	)

	p.NextPipelineChan() <- msg1
	p.NextPipelineChan() <- msg2
	p.Flush(context.Background())

	waitForCondition(t, 5*time.Second, func() bool {
		return len(us1.allRecords()) == 2 && len(us5.allRecords()) == 1
	}, "US1=2, US5=1")

	// Verify outgoing tag_filters stripped route_target:us5 from delivered JSON payloads while preserving env:prod.
	us5Rec := us5.allRecords()[0]
	ddtags, _ := us5Rec["ddtags"].(string)
	assert.Contains(t, ddtags, "env:prod")
	assert.NotContains(t, ddtags, "route_target:us5")
}

func TestEndpointFilteredIsolatesOptionalFailureAndSaturation(t *testing.T) {
	us1 := newEFIntake(t)
	us5 := newEFIntake(t)
	// Stall US5 completely so its HTTP worker and queue saturate.
	us5.setBlocking(true)
	defer us5.setBlocking(false)

	eps := config.NewEndpointsWithBatchSettings(
		us1.endpoint("us1-key", true),
		[]config.Endpoint{us5.endpoint("us5-key", false)},
		false, true, 50*time.Millisecond, 1, 2, 1024*1024, 16,
	)
	matchAll := routing.SelectorFunc(func(routing.LogView) (bool, error) { return true, nil })
	sink := &efSink{ch: make(chan *message.Payload, 300)}

	p, err := NewEndpointFilteredProvider(EndpointFilteredOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		Endpoints:                 eps,
		Selectors:                 []routing.EndpointSelector{nil, matchAll},
		DestinationsContext:       client.NewDestinationsContext(),
		Config:                    efTestConfig(t),
		Compression:               logscompression.NewMockCompressor(),
		Secrets:                   secretsnoop.NewComponent().Comp,
	})
	require.NoError(t, err)
	p.Start()
	defer p.Stop()

	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/iso.log"})
	in := p.NextPipelineChan()
	const total = 200 // Exceeds optionalStreamCapacity (128) to exercise non-blocking drops.
	for i := 1; i <= total; i++ {
		in <- makeCheckpointedMessage(src, "file:/tmp/iso.log", strconv.Itoa(i), "msg-"+strconv.Itoa(i), message.StatusNotice)
	}
	p.Flush(context.Background())

	waitForCondition(t, 5*time.Second, func() bool {
		return len(us1.messages()) == total
	}, "US1 delivers all 200 records while US5 is stalled")

	// Checkpoints must advance all the way to 200 despite US5 being completely stalled.
	var lastOffset string
	for count := 0; count < total; {
		select {
		case cp := <-sink.ch:
			for _, m := range cp.MessageMetas {
				lastOffset = m.Origin.Offset
				count++
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for checkpoints, got %d/%d", count, total)
		}
	}
	assert.Equal(t, strconv.Itoa(total), lastOffset)
}

func TestEndpointFilteredPrimaryOutageHoldsCheckpointThenRecovers(t *testing.T) {
	us1 := newEFIntake(t)
	us5 := newEFIntake(t)
	us1.setStatus(http.StatusServiceUnavailable)

	eps := config.NewEndpointsWithBatchSettings(
		us1.endpoint("us1-key", true),
		[]config.Endpoint{us5.endpoint("us5-key", false)},
		false, true, 50*time.Millisecond, 1, 8, 1024*1024, 16,
	)
	noticeOnly := routing.SelectorFunc(func(v routing.LogView) (bool, error) {
		return v.Status() == message.StatusNotice, nil
	})
	sink := &efSink{ch: make(chan *message.Payload, 32)}

	p, err := NewEndpointFilteredProvider(EndpointFilteredOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		Endpoints:                 eps,
		Selectors:                 []routing.EndpointSelector{nil, noticeOnly},
		DestinationsContext:       client.NewDestinationsContext(),
		Config:                    efTestConfig(t),
		Compression:               logscompression.NewMockCompressor(),
		Secrets:                   secretsnoop.NewComponent().Comp,
	})
	require.NoError(t, err)
	p.Start()
	defer p.Stop()

	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/outage.log"})
	in := p.NextPipelineChan()
	for i := 1; i <= 4; i++ {
		in <- makeCheckpointedMessage(src, "file:/tmp/outage.log", strconv.Itoa(i*10), "notice-"+strconv.Itoa(i), message.StatusNotice)
	}

	// US5 receives all 4 best-effort copies while US1 is returning 503.
	waitForCondition(t, 5*time.Second, func() bool {
		return len(us5.messages()) == 4
	}, "US5 receives 4 copies during US1 outage")

	// US5 acknowledgment must NEVER advance the source checkpoint!
	select {
	case cp := <-sink.ch:
		t.Fatalf("optional US5 delivery must not advance checkpoint: %v", cp.MessageMetas[0].Origin.Offset)
	case <-time.After(150 * time.Millisecond):
	}

	// Recover US1 -> all 4 records deliver to US1 and checkpoint advances to 40.
	us1.setStatus(http.StatusAccepted)
	waitForCondition(t, 5*time.Second, func() bool {
		return len(us1.messages()) == 4
	}, "US1 recovers and delivers all 4 records")

	var lastOffset string
	for count := 0; count < 4; {
		select {
		case cp := <-sink.ch:
			for _, m := range cp.MessageMetas {
				lastOffset = m.Origin.Offset
				count++
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for recovered checkpoints, got %d/4", count)
		}
	}
	assert.Equal(t, "40", lastOffset)
}

func TestEndpointFilteredZeroMatchOptionalOnlyAndEvalErrors(t *testing.T) {
	primary := newEFIntake(t)
	optional := newEFIntake(t)

	eps := config.NewEndpointsWithBatchSettings(
		primary.endpoint("p-key", true),
		[]config.Endpoint{optional.endpoint("o-key", false)},
		false, true, 50*time.Millisecond, 1, 10, 1024*1024, 16,
	)

	primarySel := routing.SelectorFunc(func(v routing.LogView) (bool, error) {
		s := string(v.Content())
		if strings.Contains(s, "eval-err") {
			return false, errors.New("primary eval error")
		}
		return strings.Contains(s, "primary"), nil
	})
	optionalSel := routing.SelectorFunc(func(v routing.LogView) (bool, error) {
		s := string(v.Content())
		if strings.Contains(s, "eval-err") {
			return false, errors.New("optional eval error")
		}
		return strings.Contains(s, "opt"), nil
	})

	var errCount atomic.Int32
	sink := &efSink{ch: make(chan *message.Payload, 16)}
	p, err := NewEndpointFilteredProvider(EndpointFilteredOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		Endpoints:                 eps,
		Selectors:                 []routing.EndpointSelector{primarySel, optionalSel},
		OnError:                   func(error) { errCount.Add(1) },
		DestinationsContext:       client.NewDestinationsContext(),
		Config:                    efTestConfig(t),
		Compression:               logscompression.NewMockCompressor(),
		Secrets:                   secretsnoop.NewComponent().Comp,
	})
	require.NoError(t, err)
	p.Start()
	defer p.Stop()

	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/edge.log"})
	in := p.NextPipelineChan()
	// 1. 0-match: neither receives it, checkpoint advances.
	in <- makeCheckpointedMessage(src, "file:/tmp/edge.log", "10", "none-match", message.StatusInfo)
	// 2. Optional-only: primary skips, optional receives it, checkpoint advances.
	in <- makeCheckpointedMessage(src, "file:/tmp/edge.log", "20", "opt-only", message.StatusInfo)
	// 3. Eval error on both: primary fails-open (receives), optional fails-closed (skips), checkpoint advances.
	in <- makeCheckpointedMessage(src, "file:/tmp/edge.log", "30", "eval-err", message.StatusInfo)
	// 4. Primary-only: primary receives, optional skips.
	in <- makeCheckpointedMessage(src, "file:/tmp/edge.log", "40", "primary-only", message.StatusInfo)

	p.Flush(context.Background())

	waitForCondition(t, 5*time.Second, func() bool {
		return len(primary.messages()) == 2 && len(optional.messages()) == 1
	}, "primary=2 (eval-err + primary-only), optional=1 (opt-only)")

	assert.Equal(t, []string{"eval-err", "primary-only"}, primary.messages())
	assert.Equal(t, []string{"opt-only"}, optional.messages())
	assert.Equal(t, int32(2), errCount.Load())

	var offsets []string
	for len(offsets) < 4 {
		select {
		case cp := <-sink.ch:
			for _, m := range cp.MessageMetas {
				offsets = append(offsets, m.Origin.Offset)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for checkpoints, got %v", offsets)
		}
	}
	assert.Equal(t, []string{"10", "20", "30", "40"}, offsets)
}

func TestEndpointFilteredMRFFailoverSatisfiesReliableSet(t *testing.T) {
	us1 := newEFIntake(t)
	mrf := newEFIntake(t)
	us1.setStatus(http.StatusServiceUnavailable) // US1 is down.

	us1EP := us1.endpoint("us1-key", true)
	mrfEP := mrf.endpoint("mrf-key", true)
	mrfEP.IsMRF = true

	eps := config.NewEndpointsWithBatchSettings(
		us1EP,
		[]config.Endpoint{mrfEP},
		false, true, 50*time.Millisecond, 1, 10, 1024*1024, 16,
	)

	cfg := efTestConfig(t)
	cfg.Set("multi_region_failover.enabled", true, pkgconfigmodel.SourceRC)
	cfg.Set("multi_region_failover.failover_logs", true, pkgconfigmodel.SourceRC)
	cfg.Set("multi_region_failover.logs_service_allowlist", []string{"payments"}, pkgconfigmodel.SourceRC)

	// Add a non-nil selector on MRF as well to prove MRF respects endpoint selectors.
	mrfSel := routing.SelectorFunc(func(v routing.LogView) (bool, error) {
		return v.Status() != message.StatusDebug, nil
	})

	sink := &efSink{ch: make(chan *message.Payload, 16)}
	p, err := NewEndpointFilteredProvider(EndpointFilteredOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		Endpoints:                 eps,
		Selectors:                 []routing.EndpointSelector{nil, mrfSel},
		DestinationsContext:       client.NewDestinationsContext(),
		Config:                    cfg,
		Compression:               logscompression.NewMockCompressor(),
		Secrets:                   secretsnoop.NewComponent().Comp,
	})
	require.NoError(t, err)
	p.Start()
	defer p.Stop()

	paySrc := sources.NewLogSource("payments", &config.LogsConfig{Type: config.FileType, Path: "/tmp/pay.log", Service: "payments"})
	webSrc := sources.NewLogSource("web", &config.LogsConfig{Type: config.FileType, Path: "/tmp/web.log", Service: "web"})

	payIn, err := p.SourceInput("file:/tmp/pay.log", 1)
	require.NoError(t, err)
	webIn, err := p.SourceInput("file:/tmp/web.log", 1)
	require.NoError(t, err)

	// payments NOTICE -> allowed by MRF allowlist + selector -> MRF delivers and advances payments checkpoint!
	payIn <- makeCheckpointedMessage(paySrc, "file:/tmp/pay.log", "10", "pay-notice", message.StatusNotice)
	// web NOTICE -> NOT in MRF allowlist -> MRF does not receive it; web checkpoint stays held until US1 recovers.
	webIn <- makeCheckpointedMessage(webSrc, "file:/tmp/web.log", "10", "web-notice", message.StatusNotice)

	waitForCondition(t, 5*time.Second, func() bool {
		return len(mrf.messages()) == 1
	}, "MRF delivers payments log during US1 outage")
	assert.Equal(t, []string{"pay-notice"}, mrf.messages())

	// payments checkpoint advances via MRF (RequireAny); web does not.
	select {
	case cp := <-sink.ch:
		require.Equal(t, "file:/tmp/pay.log", cp.MessageMetas[0].Origin.Identifier)
		require.Equal(t, "10", cp.MessageMetas[0].Origin.Offset)
	case <-time.After(5 * time.Second):
		t.Fatal("payments checkpoint did not advance via MRF")
	}

	select {
	case cp := <-sink.ch:
		t.Fatalf("web checkpoint must not advance while US1 is down and MRF excludes web: %v", cp.MessageMetas[0].Origin.Identifier)
	case <-time.After(150 * time.Millisecond):
	}

	// Recover US1 -> web delivers and advances its checkpoint.
	us1.setStatus(http.StatusAccepted)
	waitForCondition(t, 5*time.Second, func() bool {
		return slices.Contains(us1.messages(), "web-notice")
	}, "US1 recovers and delivers web-notice")

	select {
	case cp := <-sink.ch:
		require.Equal(t, "file:/tmp/web.log", cp.MessageMetas[0].Origin.Identifier)
		require.Equal(t, "10", cp.MessageMetas[0].Origin.Offset)
	case <-time.After(5 * time.Second):
		t.Fatal("web checkpoint did not advance after US1 recovery")
	}
}

func TestEndpointFilteredStopWithoutStartIsSafe(t *testing.T) {
	us1 := newEFIntake(t)
	us5 := newEFIntake(t)
	eps := config.NewEndpointsWithBatchSettings(
		us1.endpoint("us1-key", true),
		[]config.Endpoint{us5.endpoint("us5-key", false)},
		false, true, time.Second, 1, 10, 1024*1024, 16,
	)
	p, err := NewEndpointFilteredProvider(EndpointFilteredOptions{
		NumberOfPipelines:         1,
		Sink:                      &efSink{ch: make(chan *message.Payload, 1)},
		DiagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		Endpoints:                 eps,
		DestinationsContext:       client.NewDestinationsContext(),
		Config:                    efTestConfig(t),
		Compression:               logscompression.NewMockCompressor(),
		Secrets:                   secretsnoop.NewComponent().Comp,
	})
	require.NoError(t, err)
	p.Stop()
}
