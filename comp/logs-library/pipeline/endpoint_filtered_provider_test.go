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
	p.Flush(context.Background())

	// US5 acknowledgment must NEVER advance the source checkpoint!
	select {
	case cp := <-sink.ch:
		t.Fatalf("optional US5 delivery must not advance checkpoint: %v", cp.MessageMetas[0].Origin.Offset)
	default:
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

	p.Flush(context.Background())
	select {
	case cp := <-sink.ch:
		t.Fatalf("web checkpoint must not advance while US1 is down and MRF excludes web: %v", cp.MessageMetas[0].Origin.Identifier)
	default:
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

func TestEndpointFilteredStopDoesNotDeadlockWithFullReliableOutput(t *testing.T) {
	us1 := newEFIntake(t)
	us1.setBlocking(true)
	defer us1.setBlocking(false)

	eps := config.NewEndpointsWithBatchSettings(
		us1.endpoint("us1-key", true),
		nil, false, true, 20*time.Millisecond, 1, 1, 1024*1024, 16,
	)
	p, err := NewEndpointFilteredProvider(EndpointFilteredOptions{
		NumberOfPipelines:         1,
		Sink:                      &efSink{ch: make(chan *message.Payload, 16)},
		DiagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		Endpoints:                 eps,
		Selectors:                 []routing.EndpointSelector{routing.SelectorFunc(func(routing.LogView) (bool, error) { return true, nil })},
		DestinationsContext:       client.NewDestinationsContext(),
		Config:                    efTestConfig(t),
		Compression:               logscompression.NewMockCompressor(),
		Secrets:                   secretsnoop.NewComponent().Comp,
	})
	require.NoError(t, err)
	p.Start()

	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/full.log"})
	in := p.NextPipelineChan()
	// Fill reliable sender + batcher output channels while US1 is blocked.
	for i := 0; i < 20; i++ {
		in <- makeCheckpointedMessage(src, "file:/tmp/full.log", strconv.Itoa(i+1), "line-"+strconv.Itoa(i), message.StatusInfo)
	}

	done := make(chan struct{})
	go func() {
		p.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Stop deadlocked with full reliable batcher output")
	}
}

func TestEndpointFilteredPressureFlushDoesNotStallHealthyReliableEndpoint(t *testing.T) {
	stalled := newEFIntake(t)
	healthy := newEFIntake(t)
	stalled.setBlocking(true)
	defer stalled.setBlocking(false)

	eps := config.NewEndpointsWithBatchSettings(
		stalled.endpoint("r0-key", true),
		[]config.Endpoint{healthy.endpoint("r1-key", true)},
		false, true, 5*time.Second, 1, 100, 1024*1024, 16,
	)
	matchAll := routing.SelectorFunc(func(routing.LogView) (bool, error) { return true, nil })
	sink := &efSink{ch: make(chan *message.Payload, 128)}

	p, err := NewEndpointFilteredProvider(EndpointFilteredOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		Endpoints:                 eps,
		Selectors:                 []routing.EndpointSelector{matchAll, matchAll},
		DestinationsContext:       client.NewDestinationsContext(),
		Config:                    efTestConfig(t),
		Compression:               logscompression.NewMockCompressor(),
		Secrets:                   secretsnoop.NewComponent().Comp,
	})
	require.NoError(t, err)
	p.Start()
	defer p.Stop()

	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/pressure.log"})
	in, err := p.SourceInput("file:/tmp/pressure.log", 1)
	require.NoError(t, err)
	const total = 80 // Exceeds SourcePendingLimit (64) with BatchMaxSize=100 so pressure flush must trigger.
	for i := 1; i <= total; i++ {
		in <- makeCheckpointedMessage(src, "file:/tmp/pressure.log", strconv.Itoa(i), "msg-"+strconv.Itoa(i), message.StatusInfo)
	}
	p.Flush(context.Background())

	waitForCondition(t, 5*time.Second, func() bool {
		return len(healthy.messages()) == total
	}, "healthy reliable endpoint delivers all 80 records while stalled reliable endpoint is blocked")
}

func TestEndpointFilteredSharedIngressSourceChurnAndRetireReclamation(t *testing.T) {
	us1 := newEFIntake(t)
	eps := config.NewEndpointsWithBatchSettings(
		us1.endpoint("us1-key", true),
		nil, false, true, 10*time.Millisecond, 4, 64, 1024*1024, 16,
	)
	sink := &efSink{ch: make(chan *message.Payload, 512)}
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		for range sink.ch {
		}
	}()

	p, err := NewEndpointFilteredProvider(EndpointFilteredOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		Endpoints:                 eps,
		Selectors:                 []routing.EndpointSelector{routing.SelectorFunc(func(routing.LogView) (bool, error) { return true, nil })},
		DestinationsContext:       client.NewDestinationsContext(),
		Config:                    efTestConfig(t),
		Compression:               logscompression.NewMockCompressor(),
		Secrets:                   secretsnoop.NewComponent().Comp,
	})
	require.NoError(t, err)
	p.Start()
	defer func() {
		p.Stop()
		close(sink.ch)
		<-drainDone
	}()

	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/churn.log"})
	in := p.NextPipelineChan()
	// Forward records across distinct identifiers through real dispatch + delivery
	// to verify both ephemeral progress eviction and dispatch lane reclamation.
	const distinctSources = 150
	for i := 0; i < distinctSources; i++ {
		in <- makeCheckpointedMessage(src, "file:/tmp/churn-"+strconv.Itoa(i)+".log", "10", "delivered", message.StatusInfo)
	}
	p.Flush(context.Background())
	waitForCondition(t, 5*time.Second, func() bool {
		return len(us1.messages()) == distinctSources
	}, "all churn records delivered")

	waitForCondition(t, 5*time.Second, func() bool {
		p.sourceDispatchMu.Lock()
		n := len(p.sourceDispatch)
		p.sourceDispatchMu.Unlock()
		return n == 0
	}, "idle sourceDispatch lanes reclaimed")

	// Also verify bound SourceInput + RetireSource reclaims the channel cleanly.
	boundIn, err := p.SourceInput("file:/tmp/bound.log", 1)
	require.NoError(t, err)
	boundIn <- makeCheckpointedMessage(src, "file:/tmp/bound.log", "10", "bound-msg", message.StatusInfo)
	p.Flush(context.Background())
	require.NoError(t, p.RetireSource(context.Background(), "file:/tmp/bound.log", 1))
	p.lifecycle.RLock()
	_, stillTracked := p.sourceInputs[sourceInputKey{identifier: "file:/tmp/bound.log", generation: 1}]
	p.lifecycle.RUnlock()
	assert.False(t, stillTracked, "retired SourceInput must be reclaimed")
}

func TestEndpointFilteredDisablingMRFMidFlightDoesNotStrandCheckpoint(t *testing.T) {
	primary := newEFIntake(t)
	mrf := newEFIntake(t)

	primaryEP := primary.endpoint("p-key", true)
	mrfEP := mrf.endpoint("mrf-key", true)
	mrfEP.IsMRF = true

	eps := config.NewEndpointsWithBatchSettings(
		primaryEP,
		[]config.Endpoint{mrfEP},
		false, true, 50*time.Millisecond, 1, 10, 1024*1024, 16,
	)

	cfg := efTestConfig(t)
	cfg.Set("multi_region_failover.enabled", true, pkgconfigmodel.SourceRC)
	cfg.Set("multi_region_failover.failover_logs", true, pkgconfigmodel.SourceRC)

	selectedCh := make(chan struct{}, 1)
	releaseCh := make(chan struct{})
	var firstSelected atomic.Bool
	// Filtered primary rejects records so MRF is the ONLY matching reliable endpoint.
	rejectPrimary := routing.SelectorFunc(func(v routing.LogView) (bool, error) {
		if strings.Contains(string(v.Content()), "in-flight-mrf") && firstSelected.CompareAndSwap(false, true) {
			selectedCh <- struct{}{}
			<-releaseCh
		}
		return false, nil
	})
	sink := &efSink{ch: make(chan *message.Payload, 8)}

	p, err := NewEndpointFilteredProvider(EndpointFilteredOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		Endpoints:                 eps,
		Selectors:                 []routing.EndpointSelector{rejectPrimary, nil},
		DestinationsContext:       client.NewDestinationsContext(),
		Config:                    cfg,
		Compression:               logscompression.NewMockCompressor(),
		Secrets:                   secretsnoop.NewComponent().Comp,
	})
	require.NoError(t, err)
	p.Start()
	defer p.Stop()

	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/mrf-toggle.log", Service: "payments"})
	p.NextPipelineChan() <- makeCheckpointedMessage(src, "file:/tmp/mrf-toggle.log", "10", "in-flight-mrf", message.StatusNotice)

	// Wait until record 1 has entered selection with MRF enabled, then disable
	// multi_region_failover.enabled while keeping failover_logs=true.
	select {
	case <-selectedCh:
	case <-time.After(5 * time.Second):
		t.Fatal("record 1 did not reach selection")
	}
	cfg.Set("multi_region_failover.enabled", false, pkgconfigmodel.SourceRC)
	close(releaseCh)
	p.Flush(context.Background())

	// Record 1 was selected before disablement -> delivered to MRF and advances checkpoint.
	waitForCondition(t, 5*time.Second, func() bool {
		return len(mrf.messages()) == 1
	}, "in-flight record delivered to MRF")
	select {
	case cp := <-sink.ch:
		require.Equal(t, "10", cp.MessageMetas[0].Origin.Offset)
	case <-time.After(5 * time.Second):
		t.Fatal("checkpoint stranded after MRF toggled off mid-flight")
	}

	// Record 2 admitted after multi_region_failover.enabled=false (even with
	// failover_logs=true) must NOT be sent to MRF; it completes via 0-match exclusion.
	p.NextPipelineChan() <- makeCheckpointedMessage(src, "file:/tmp/mrf-toggle.log", "20", "after-disable", message.StatusNotice)
	p.Flush(context.Background())
	select {
	case cp := <-sink.ch:
		require.Equal(t, "20", cp.MessageMetas[0].Origin.Offset)
	case <-time.After(5 * time.Second):
		t.Fatal("record 2 checkpoint did not advance")
	}
	assert.Equal(t, []string{"in-flight-mrf"}, mrf.messages(), "MRF must not receive records after multi_region_failover.enabled=false")
}

func TestEndpointFilteredConcurrentFlushRetireAndStop(t *testing.T) {
	us1 := newEFIntake(t)
	eps := config.NewEndpointsWithBatchSettings(
		us1.endpoint("us1-key", true),
		nil, false, true, 20*time.Millisecond, 1, 8, 1024*1024, 16,
	)
	p, err := NewEndpointFilteredProvider(EndpointFilteredOptions{
		NumberOfPipelines:         1,
		Sink:                      &efSink{ch: make(chan *message.Payload, 32)},
		DiagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		Endpoints:                 eps,
		Selectors:                 []routing.EndpointSelector{routing.SelectorFunc(func(routing.LogView) (bool, error) { return true, nil })},
		DestinationsContext:       client.NewDestinationsContext(),
		Config:                    efTestConfig(t),
		Compression:               logscompression.NewMockCompressor(),
		Secrets:                   secretsnoop.NewComponent().Comp,
	})
	require.NoError(t, err)
	p.Start()

	_, err = p.SourceInput("file:/tmp/race.log", 1)
	require.NoError(t, err)

	startGate := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startGate
			p.Flush(context.Background())
		}()
	}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startGate
			_ = p.RetireSource(context.Background(), "file:/tmp/race.log", 1)
		}()
	}
	close(startGate)
	p.Stop()
	wg.Wait()
}

func TestEndpointFilteredBoundInputAcceptsUncheckpointedRotationDrain(t *testing.T) {
	us1 := newEFIntake(t)
	eps := config.NewEndpointsWithBatchSettings(
		us1.endpoint("us1-key", true),
		nil, false, true, 20*time.Millisecond, 1, 8, 1024*1024, 16,
	)
	sink := &efSink{ch: make(chan *message.Payload, 8)}
	var errCount atomic.Int32
	p, err := NewEndpointFilteredProvider(EndpointFilteredOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		Endpoints:                 eps,
		Selectors:                 []routing.EndpointSelector{routing.SelectorFunc(func(routing.LogView) (bool, error) { return true, nil })},
		OnError:                   func(error) { errCount.Add(1) },
		DestinationsContext:       client.NewDestinationsContext(),
		Config:                    efTestConfig(t),
		Compression:               logscompression.NewMockCompressor(),
		Secrets:                   secretsnoop.NewComponent().Comp,
	})
	require.NoError(t, err)
	p.Start()
	defer p.Stop()

	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/rot.log"})
	boundIn, err := p.SourceInput("file:/tmp/rot.log", 1)
	require.NoError(t, err)

	// Normal checkpointed record + deliberately uncheckpointed rotated-drain record (empty Identifier).
	boundIn <- makeCheckpointedMessage(src, "file:/tmp/rot.log", "10", "before-rotate", message.StatusInfo)
	boundIn <- makeCheckpointedMessage(src, "", "", "draining-after-rotate", message.StatusInfo)
	p.Flush(context.Background())

	waitForCondition(t, 5*time.Second, func() bool {
		return len(us1.messages()) == 2
	}, "both normal and uncheckpointed rotated-drain records delivered")
	assert.ElementsMatch(t, []string{"before-rotate", "draining-after-rotate"}, us1.messages())
	assert.Zero(t, errCount.Load())

	// Only the checkpointed record emits an auditor offset.
	cp := <-sink.ch
	assert.Equal(t, "10", cp.MessageMetas[0].Origin.Offset)

	// A different nonempty identifier on the bound channel is still rejected.
	boundIn <- makeCheckpointedMessage(src, "file:/tmp/other.log", "20", "wrong-id", message.StatusInfo)
	p.Flush(context.Background())
	assert.Equal(t, int32(1), errCount.Load())
}

func TestEndpointFilteredSharedIngressStalledSourceDoesNotStarveHealthySource(t *testing.T) {
	us1 := newEFIntake(t)
	us1.setStatus(http.StatusServiceUnavailable)
	opt := newEFIntake(t)

	eps := config.NewEndpointsWithBatchSettings(
		us1.endpoint("us1-key", true),
		[]config.Endpoint{opt.endpoint("opt-key", false)},
		false, true, 20*time.Millisecond, 1, 8, 1024*1024, 16,
	)
	// Source A goes to US1 (stalled); Source B goes to optional-only (healthy).
	us1Sel := routing.SelectorFunc(func(v routing.LogView) (bool, error) {
		return v.Service() == "svc-a", nil
	})
	optSel := routing.SelectorFunc(func(v routing.LogView) (bool, error) {
		return v.Service() == "svc-b", nil
	})
	sink := &efSink{ch: make(chan *message.Payload, 32)}
	p, err := NewEndpointFilteredProvider(EndpointFilteredOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		Endpoints:                 eps,
		Selectors:                 []routing.EndpointSelector{us1Sel, optSel},
		DestinationsContext:       client.NewDestinationsContext(),
		Config:                    efTestConfig(t),
		Compression:               logscompression.NewMockCompressor(),
		Secrets:                   secretsnoop.NewComponent().Comp,
	})
	require.NoError(t, err)
	p.Start()
	defer p.Stop()

	srcA := sources.NewLogSource("a", &config.LogsConfig{Type: config.FileType, Path: "/tmp/a.log", Service: "svc-a"})
	srcB := sources.NewLogSource("b", &config.LogsConfig{Type: config.FileType, Path: "/tmp/b.log", Service: "svc-b"})
	in := p.NextPipelineChan()

	// Saturate Source A past SourcePendingLimit (64) on shared ingress while US1 is 503.
	for i := 1; i <= 70; i++ {
		in <- makeCheckpointedMessage(srcA, "file:/tmp/a.log", strconv.Itoa(i), "a-"+strconv.Itoa(i), message.StatusInfo)
	}
	// Send Source B on the SAME shared ingress channel: it must not be starved by Source A.
	in <- makeCheckpointedMessage(srcB, "file:/tmp/b.log", "10", "b-healthy", message.StatusNotice)

	waitForCondition(t, 5*time.Second, func() bool {
		return slices.Contains(opt.messages(), "b-healthy")
	}, "healthy Source B delivered on shared ingress while Source A is stalled")

	select {
	case cp := <-sink.ch:
		assert.Equal(t, "file:/tmp/b.log", cp.MessageMetas[0].Origin.Identifier)
		assert.Equal(t, "10", cp.MessageMetas[0].Origin.Offset)
	case <-time.After(5 * time.Second):
		t.Fatal("Source B checkpoint starved by stalled Source A on shared ingress")
	}
}

func TestEndpointFilteredRetireWhilePendingAutoRetiresOnDrain(t *testing.T) {
	us1 := newEFIntake(t)
	us1.setStatus(http.StatusServiceUnavailable)
	eps := config.NewEndpointsWithBatchSettings(
		us1.endpoint("us1-key", true),
		nil, false, true, 20*time.Millisecond, 1, 8, 1024*1024, 16,
	)
	sink := &efSink{ch: make(chan *message.Payload, 8)}
	p, err := NewEndpointFilteredProvider(EndpointFilteredOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		Endpoints:                 eps,
		Selectors:                 []routing.EndpointSelector{routing.SelectorFunc(func(routing.LogView) (bool, error) { return true, nil })},
		DestinationsContext:       client.NewDestinationsContext(),
		Config:                    efTestConfig(t),
		Compression:               logscompression.NewMockCompressor(),
		Secrets:                   secretsnoop.NewComponent().Comp,
	})
	require.NoError(t, err)
	p.Start()
	defer p.Stop()

	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/retire-pending.log"})
	ch, err := p.SourceInput("file:/tmp/retire-pending.log", 1)
	require.NoError(t, err)
	ch <- makeCheckpointedMessage(src, "file:/tmp/retire-pending.log", "10", "in-flight", message.StatusInfo)

	// Retire while record is still pending on 503 US1 -> succeeds via deferred auto-retirement.
	require.NoError(t, p.RetireSource(context.Background(), "file:/tmp/retire-pending.log", 1))
	assert.True(t, p.progress.OwnsSource("file:/tmp/retire-pending.log"), "ownership held until pending record drains")

	// Recover US1 -> record delivers, checkpoint advances, and source ownership auto-retires!
	us1.setStatus(http.StatusAccepted)
	select {
	case cp := <-sink.ch:
		assert.Equal(t, "10", cp.MessageMetas[0].Origin.Offset)
	case <-time.After(5 * time.Second):
		t.Fatal("pending record did not complete after recovery")
	}
	waitForCondition(t, 5*time.Second, func() bool {
		return !p.progress.OwnsSource("file:/tmp/retire-pending.log")
	}, "retired source automatically released ownership once drained")
}

func TestEndpointFilteredGracefulStopDrainsCompletedCheckpoints(t *testing.T) {
	us1 := newEFIntake(t)
	eps := config.NewEndpointsWithBatchSettings(
		us1.endpoint("us1-key", true),
		nil, false, true, 20*time.Millisecond, 1, 8, 1024*1024, 16,
	)
	sink := &efSink{ch: make(chan *message.Payload, 16)}
	p, err := NewEndpointFilteredProvider(EndpointFilteredOptions{
		NumberOfPipelines:         1,
		Sink:                      sink,
		DiagnosticMessageReceiver: &diagnostic.NoopMessageReceiver{},
		Endpoints:                 eps,
		Selectors:                 []routing.EndpointSelector{routing.SelectorFunc(func(routing.LogView) (bool, error) { return false, nil })},
		DestinationsContext:       client.NewDestinationsContext(),
		Config:                    efTestConfig(t),
		Compression:               logscompression.NewMockCompressor(),
		Secrets:                   secretsnoop.NewComponent().Comp,
	})
	require.NoError(t, err)
	p.Start()

	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/drain.log"})
	in := p.NextPipelineChan()
	for i := 1; i <= 5; i++ {
		in <- makeCheckpointedMessage(src, "file:/tmp/drain.log", strconv.Itoa(i*10), "excluded", message.StatusInfo)
	}
	// Stop immediately without pre-reading sink.ch: DrainAndClose must deliver all 5 checkpoints.
	p.Stop()

	var offsets []string
	for len(sink.ch) > 0 {
		cp := <-sink.ch
		for _, m := range cp.MessageMetas {
			offsets = append(offsets, m.Origin.Offset)
		}
	}
	assert.Equal(t, []string{"10", "20", "30", "40", "50"}, offsets)
}
