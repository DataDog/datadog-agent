// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package routing

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type stubTagFilter struct {
	dropPrefix string
}

func (f stubTagFilter) Keep(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if len(tag) >= len(f.dropPrefix) && tag[:len(f.dropPrefix)] == f.dropPrefix {
			continue
		}
		out = append(out, tag)
	}
	return out
}

func (f stubTagFilter) RetainsTag(key, value string) bool {
	tag := key + ":" + value
	return len(tag) < len(f.dropPrefix) || tag[:len(f.dropPrefix)] != f.dropPrefix
}

func makeFilterTestEndpoints(main config.Endpoint, additionals ...config.Endpoint) *config.Endpoints {
	return config.NewEndpointsWithBatchSettings(main, additionals, false, true, time.Second, 0, 10, 1024*1024, 10)
}

func TestStubSelectorCompiler(t *testing.T) {
	var compiler SelectorCompiler = StubSelectorCompiler{}
	sel, err := compiler.Compile("")
	if err != nil || sel != nil {
		t.Fatalf("empty expression: got (%v, %v), want (nil, nil)", sel, err)
	}
	sel, err = compiler.Compile(`status == "error"`)
	if sel != nil || !errors.Is(err, ErrSelectorCompilerNotConfigured) {
		t.Fatalf("non-empty expression: got (%v, %v), want ErrSelectorCompilerNotConfigured", sel, err)
	}
}

func TestLogViewExposesLocalUnfilteredTagsAndStructuredFields(t *testing.T) {
	src := sources.NewLogSource("app", &config.LogsConfig{
		Service: "payments",
		Source:  "go",
		Tags:    []string{"env:prod", "internal_route:us5"},
	})
	src.SetTagFilterIfUnset(stubTagFilter{dropPrefix: "internal_route:"})

	origin := message.NewOrigin(src)
	origin.SetTags([]string{"team:logs"})

	structured := &message.BasicStructuredContent{Data: map[string]any{
		"level": "notice",
		"http":  map[string]any{"status": 503},
	}}
	msg := message.NewStructuredMessage(structured, origin, message.StatusNotice, 0)
	msg.Hostname = "host-1"

	// Outgoing transport tags strip internal_route:us5.
	if slices.Contains(msg.Origin.TransportTags(), "internal_route:us5") {
		t.Fatalf("expected TransportTags to strip internal_route:us5, got %v", msg.Origin.TransportTags())
	}

	// LogView still sees unfiltered local tags, metadata, and structured fields.
	view := NewLogView(msg)
	if !slices.Contains(view.Tags(), "internal_route:us5") {
		t.Fatalf("expected LogView.Tags to keep local tag internal_route:us5, got %v", view.Tags())
	}
	if !slices.Contains(view.Tags(), "team:logs") || !slices.Contains(view.Tags(), "env:prod") {
		t.Fatalf("unexpected tags: %v", view.Tags())
	}
	if view.Service() != "payments" || view.Source() != "go" || view.Status() != message.StatusNotice || view.Hostname() != "host-1" {
		t.Fatalf("unexpected metadata: service=%q source=%q status=%q host=%q", view.Service(), view.Source(), view.Status(), view.Hostname())
	}
	if v, ok := view.StructuredAttribute("level"); !ok || v != "notice" {
		t.Fatalf("unexpected structured attribute level: (%v, %v)", v, ok)
	}
}

func TestEndpointFilterPolicyValidation(t *testing.T) {
	main := config.NewEndpoint("key-us1", "", "us1.example.com", 443, "", true)
	eps := makeFilterTestEndpoints(main)

	if _, err := NewEndpointFilterPolicy(nil, nil); !errors.Is(err, ErrInvalidEndpointFilterPolicy) {
		t.Fatalf("nil endpoints: got %v", err)
	}
	tcp := config.NewEndpointsWithBatchSettings(main, nil, false, false, time.Second, 0, 10, 1024, 10)
	if _, err := NewEndpointFilterPolicy(tcp, nil); !errors.Is(err, ErrInvalidEndpointFilterPolicy) {
		t.Fatalf("TCP endpoints: got %v", err)
	}
	if _, err := NewEndpointFilterPolicy(eps, []EndpointSelector{nil, nil}); !errors.Is(err, ErrInvalidEndpointFilterPolicy) {
		t.Fatalf("too many selectors: got %v", err)
	}

	var many []config.Endpoint
	for i := 0; i < MaxFilterEndpoints; i++ {
		many = append(many, config.NewEndpoint("k", "", "extra.example.com", 443, "", true))
	}
	over := config.NewEndpointsWithBatchSettings(main, many, false, true, time.Second, 0, 10, 1024, 10)
	if _, err := NewEndpointFilterPolicy(over, nil); !errors.Is(err, ErrInvalidEndpointFilterPolicy) {
		t.Fatalf("over endpoint limit: got %v", err)
	}
}

func TestEndpointFilterPolicyEvaluationSemantics(t *testing.T) {
	us1 := config.NewEndpoint("key-us1", "", "us1.example.com", 443, "", true)
	us5 := config.NewMockEndpointWithOptions(map[string]any{
		"api_key":     "key-us5",
		"host":        "us5.example.com",
		"port":        443,
		"use_ssl":     true,
		"is_reliable": false,
	})
	mrf := config.NewEndpoint("key-mrf", "", "mrf.example.com", 443, "", true)
	mrf.IsMRF = true

	eps := makeFilterTestEndpoints(us1, us5, mrf)
	noticeOnly := SelectorFunc(func(v LogView) (bool, error) {
		return v.Status() == message.StatusNotice || v.Status() == message.StatusError, nil
	})
	policy, err := NewEndpointFilterPolicy(eps, []EndpointSelector{nil, noticeOnly, nil})
	if err != nil {
		t.Fatalf("NewEndpointFilterPolicy: %v", err)
	}
	if !policy.HasSelectors() {
		t.Fatal("expected HasSelectors() == true")
	}

	// INFO log, MRF inactive -> US1 only.
	infoMsg := message.NewMessage([]byte("info"), nil, message.StatusInfo, 0)
	rMask, oMask, errs := policy.Evaluate(infoMsg)
	if rMask != 1<<0 || oMask != 0 || len(errs) != 0 {
		t.Fatalf("INFO non-MRF: got (%b, %b, %v), want (1, 0, nil)", rMask, oMask, errs)
	}

	// NOTICE log, MRF inactive -> US1 + US5.
	noticeMsg := message.NewMessage([]byte("notice"), nil, message.StatusNotice, 0)
	rMask, oMask, errs = policy.Evaluate(noticeMsg)
	if rMask != 1<<0 || oMask != 1<<1 || len(errs) != 0 {
		t.Fatalf("NOTICE non-MRF: got (%b, %b, %v), want (1, 10, nil)", rMask, oMask, errs)
	}

	// INFO log, MRF active -> US1 + MRF.
	infoMsg.IsMRFAllow = true
	rMask, oMask, errs = policy.Evaluate(infoMsg)
	if rMask != (1<<0)|(1<<2) || oMask != 0 || len(errs) != 0 {
		t.Fatalf("INFO MRF: got (%b, %b, %v), want (101, 0, nil)", rMask, oMask, errs)
	}
}

func TestEndpointFilterPolicyEvalErrorFailOpenReliableFailClosedOptional(t *testing.T) {
	r0 := config.NewEndpoint("key-r0", "", "r0.example.com", 443, "", true)
	u1 := config.NewMockEndpointWithOptions(map[string]any{
		"api_key":     "key-u1",
		"host":        "u1.example.com",
		"port":        443,
		"use_ssl":     true,
		"is_reliable": false,
	})

	boom := errors.New("eval boom")
	errSel := SelectorFunc(func(LogView) (bool, error) { return false, boom })

	policy, err := NewEndpointFilterPolicy(makeFilterTestEndpoints(r0, u1), []EndpointSelector{errSel, errSel})
	if err != nil {
		t.Fatalf("NewEndpointFilterPolicy: %v", err)
	}

	msg := message.NewMessage([]byte("log"), nil, message.StatusInfo, 0)
	rMask, oMask, evalErrs := policy.Evaluate(msg)
	if rMask != 1<<0 {
		t.Fatalf("reliable endpoint should fail-open on error, got reliableMask=%b", rMask)
	}
	if oMask != 0 {
		t.Fatalf("optional endpoint should fail-closed on error, got optionalMask=%b", oMask)
	}
	if len(evalErrs) != 2 || !errors.Is(evalErrs[0].Err, boom) || !errors.Is(evalErrs[1].Err, boom) {
		t.Fatalf("expected 2 eval errors, got %v", evalErrs)
	}
}

type selectorTestSink struct {
	ch chan *message.Payload
}

func (s *selectorTestSink) Channel() chan *message.Payload { return s.ch }

func TestProgressRequireAnyAndFailGroupLocal(t *testing.T) {
	sink := &selectorTestSink{ch: make(chan *message.Payload, 8)}
	progress, err := NewProgressWithMode(8, sink, CompletionRequireAny)
	if err != nil {
		t.Fatalf("NewProgressWithMode: %v", err)
	}
	defer progress.Close()

	src := sources.NewLogSource("file", &config.LogsConfig{Type: config.FileType, Path: "/tmp/a.log"})
	makeMsg := func(offset string) *message.Message {
		o := message.NewOrigin(src)
		o.Identifier = "file:/tmp/a.log"
		o.Offset = offset
		return message.NewMessage([]byte(offset), o, message.StatusInfo, 0)
	}

	m1 := makeMsg("10")
	m2 := makeMsg("20")
	m3 := makeMsg("30")
	ctx := context.Background()

	// m1 -> r0 only; m2 -> r0 + r1; m3 -> r0 + r1.
	if _, err := progress.Register(ctx, m1, Selection{Generation: 1}, []string{"r0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := progress.Register(ctx, m2, Selection{Generation: 1}, []string{"r0", "r1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := progress.Register(ctx, m3, Selection{Generation: 1}, []string{"r0", "r1"}); err != nil {
		t.Fatal(err)
	}

	metaOf := func(m *message.Message) *message.Payload {
		meta := m.MessageMetadata
		return message.NewPayload([]*message.MessageMetadata{&meta}, nil, "", 0)
	}

	// r1 completes m2 (RequireAny marks m2 done), but m1 is still pending -> no checkpoint yet!
	progress.CompleteGroup("r1", metaOf(m2))
	select {
	case cp := <-sink.ch:
		t.Fatalf("m2 must not advance checkpoint before m1: %v", cp)
	default:
	}

	// Local batch loss on r0 for m3 must NOT complete m3 while r1 is still pending.
	progress.FailGroupLocal("r0", metaOf(m3))
	// Completing m1 on r0 advances m1 and m2, NOT m3.
	progress.CompleteGroup("r0", metaOf(m1))
	got1 := <-sink.ch
	got2 := <-sink.ch
	if got1.MessageMetas[0].Origin.Offset != "10" || got2.MessageMetas[0].Origin.Offset != "20" {
		t.Fatalf("unexpected prefix order: %s, %s", got1.MessageMetas[0].Origin.Offset, got2.MessageMetas[0].Origin.Offset)
	}
	select {
	case cp := <-sink.ch:
		t.Fatalf("m3 should still be pending on r1: %v", cp)
	default:
	}

	// Second completion of m2 on r0 is idempotent.
	progress.CompleteGroup("r0", metaOf(m2))

	// Local loss on the last remaining reliable endpoint (r1) completes m3.
	progress.FailGroupLocal("r1", metaOf(m3))
	got3 := <-sink.ch
	if got3.MessageMetas[0].Origin.Offset != "30" {
		t.Fatalf("expected m3 offset 30, got %s", got3.MessageMetas[0].Origin.Offset)
	}
}

func TestProgressRequireAnyOverlappingRotationsReclaimHistoryBudget(t *testing.T) {
	sink := &selectorTestSink{ch: make(chan *message.Payload, 16)}
	// Small budget (limit=4) must survive >20 overlapping rotations of the same identifier.
	progress, err := NewProgressWithMode(4, sink, CompletionRequireAny)
	if err != nil {
		t.Fatalf("NewProgressWithMode: %v", err)
	}
	defer progress.Close()

	const id = "file:/tmp/rotated.log"
	if err := progress.RegisterSource(id, 1); err != nil {
		t.Fatalf("RegisterSource gen 1: %v", err)
	}
	for gen := uint64(2); gen <= 25; gen++ {
		// Overlap: register replacement generation before retiring old generation.
		if err := progress.RegisterSource(id, gen); err != nil {
			t.Fatalf("RegisterSource gen %d failed (history budget leaked): %v", gen, err)
		}
		if err := progress.RetireSource(id, gen-1); err != nil {
			t.Fatalf("RetireSource gen %d: %v", gen-1, err)
		}
	}
}
