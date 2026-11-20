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

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

type stubTagFilter struct {
	dropPrefix string
}

func (f stubTagFilter) Keep(tags []string) []string {
	var kept []string
	for _, tag := range tags {
		if len(tag) >= len(f.dropPrefix) && tag[:len(f.dropPrefix)] == f.dropPrefix {
			continue
		}
		kept = append(kept, tag)
	}
	return kept
}

func (f stubTagFilter) RetainsTag(key, _ string) bool {
	return key+":" != f.dropPrefix
}

func testHTTPEndpoints(t *testing.T, extra ...config.Endpoint) *config.Endpoints {
	t.Helper()
	main := config.NewEndpoint("main-key", "", "us1.example.com", 443, "", true)
	return config.NewEndpointsWithBatchSettings(main, extra, false, true, time.Second, 2, 100, 1024*1024, 64)
}

func TestStubCompilerRejectsNonEmptyExpression(t *testing.T) {
	c := StubCompiler{}
	eval, err := c.Compile("")
	require.NoError(t, err)
	matched, err := eval.Match(NewRecordView(nil))
	require.NoError(t, err)
	require.True(t, matched)

	eval, err = c.Compile("   \t ")
	require.NoError(t, err)
	matched, err = eval.Match(NewRecordView(nil))
	require.NoError(t, err)
	require.True(t, matched)

	_, err = c.Compile(`status == "notice"`)
	require.ErrorIs(t, err, ErrOTTLNotImplemented)
}

func TestNewEndpointPolicyValidation(t *testing.T) {
	t.Run("accepts reliable main and unreliable additional with match-all defaults", func(t *testing.T) {
		us5 := config.NewEndpoint("us5-key", "", "us5.example.com", 443, "", true)
		// Mark unreliable via unmarshal-equivalent path using NewEndpoints + EndpointPolicyFromTargets.
		eps := testHTTPEndpoints(t, us5)
		policy, err := NewEndpointPolicy(PolicyOptions{Endpoints: eps})
		require.NoError(t, err)
		require.Len(t, policy.Endpoints(), 2)
		require.Equal(t, MainEndpointID, policy.Endpoints()[0].ID)
		require.Equal(t, AdditionalEndpointID(0), policy.Endpoints()[1].ID)
	})

	t.Run("rejects non-empty expression with default StubCompiler", func(t *testing.T) {
		eps := testHTTPEndpoints(t)
		_, err := NewEndpointPolicy(PolicyOptions{
			Endpoints: eps,
			Selectors: map[string]string{MainEndpointID: `severity == "notice"`},
		})
		require.ErrorIs(t, err, ErrInvalidDefinition)
		require.ErrorIs(t, err, ErrOTTLNotImplemented)
	})

	t.Run("rejects TCP, proto, MRF, unknown selector ID, and endpoint limit", func(t *testing.T) {
		main := config.NewEndpoint("main-key", "", "us1.example.com", 443, "", true)
		tcpEps := config.NewEndpointsWithBatchSettings(main, nil, false, false, time.Second, 2, 100, 1024*1024, 64)
		_, err := NewEndpointPolicy(PolicyOptions{Endpoints: tcpEps})
		require.ErrorIs(t, err, ErrInvalidDefinition)

		protoEps := config.NewEndpointsWithBatchSettings(main, nil, true, true, time.Second, 2, 100, 1024*1024, 64)
		_, err = NewEndpointPolicy(PolicyOptions{Endpoints: protoEps})
		require.ErrorIs(t, err, ErrInvalidDefinition)

		mrfEp := config.NewEndpoint("mrf-key", "", "mrf.example.com", 443, "", true)
		mrfEp.IsMRF = true
		mrfEps := testHTTPEndpoints(t, mrfEp)
		_, err = NewEndpointPolicy(PolicyOptions{Endpoints: mrfEps})
		require.ErrorIs(t, err, ErrInvalidDefinition)

		validEps := testHTTPEndpoints(t)
		_, err = NewEndpointPolicy(PolicyOptions{
			Endpoints: validEps,
			Selectors: map[string]string{"does-not-exist": ""},
		})
		require.ErrorIs(t, err, ErrInvalidDefinition)

		twoEps := testHTTPEndpoints(t, config.NewEndpoint("k2", "", "e2.example.com", 443, "", true))
		_, err = NewEndpointPolicy(PolicyOptions{Endpoints: twoEps, MaxEndpoints: 1})
		require.ErrorIs(t, err, ErrInvalidDefinition)
	})
}

func TestRecordViewAndPolicyEvaluation(t *testing.T) {
	src := sources.NewLogSource("app", &config.LogsConfig{
		Type:           config.FileType,
		Service:        "payments",
		Source:         "go",
		SourceCategory: "prod",
		Tags:           []string{"env:prod", "internal_only:secret"},
	})
	src.SetTagFilterIfUnset(stubTagFilter{dropPrefix: "internal_only:"})

	sc := &message.BasicStructuredContent{
		Data: map[string]any{
			"message": "order failed",
			"level":   "error",
			"nested":  map[string]any{"region": "us5"},
		},
	}
	origin := message.NewOrigin(src)
	origin.SetTags([]string{"team:logs"})
	origin.SetMappedSource("remapped-go")
	msg := message.NewStructuredMessage(sc, origin, message.StatusError, time.Now().UnixNano())
	msg.Hostname = "host-1"

	// Verify export filtering strips internal_only:secret while RecordView.LocalTags retains it.
	require.NotContains(t, msg.Origin.TransportTags(), "internal_only:secret")
	view := NewRecordView(msg)
	require.Contains(t, view.LocalTags(), "internal_only:secret")
	require.Contains(t, view.LocalTags(), "team:logs")
	require.Contains(t, view.LocalTags(), "env:prod")
	require.Equal(t, "remapped-go", view.Source())
	require.Equal(t, "payments", view.Service())
	require.Equal(t, "prod", view.SourceCategory())
	require.Equal(t, message.StatusError, view.Status())
	require.Equal(t, "host-1", view.Hostname())
	region, ok := view.StructuredAttribute("nested.region")
	require.True(t, ok)
	require.Equal(t, "us5", region)

	batch := testHTTPEndpoints(t)
	us1 := config.NewEndpoint("us1-key", "", "us1.example.com", 443, "", true)
	us5 := config.NewEndpoint("us5-key", "", "us5.example.com", 443, "", true)
	policy, err := NewEndpointPolicyFromTargets([]EndpointTarget{
		{ID: "us1", Endpoint: us1, Reliable: true, Evaluator: MatchAllEvaluator{}},
		{
			ID:       "us5",
			Endpoint: us5,
			Reliable: false,
			Evaluator: EvaluatorFunc(func(v RecordView) (bool, error) {
				r, ok := v.StructuredAttribute("nested.region")
				return ok && r == "us5" && slices.Contains(v.LocalTags(), "internal_only:secret") && v.Source() == "remapped-go", nil
			}),
		},
	}, batch, 4, nil)
	require.NoError(t, err)

	sel, optErrs, err := policy.Evaluate(msg)
	require.NoError(t, err)
	require.Empty(t, optErrs)
	require.Equal(t, []string{"us1"}, sel.Reliable)
	require.Equal(t, []string{"us5"}, sel.Optional)
}

func TestPolicyEvaluationErrorsReliableHoldVsOptionalSuppress(t *testing.T) {
	batch := testHTTPEndpoints(t)
	ep := config.NewEndpoint("k", "", "example.com", 443, "", true)
	msg := message.NewMessage([]byte("hello"), message.NewOrigin(sources.NewLogSource("s", &config.LogsConfig{})), message.StatusInfo, 0)

	t.Run("reliable error fails evaluation with no partial selection", func(t *testing.T) {
		policy, err := NewEndpointPolicyFromTargets([]EndpointTarget{
			{ID: "r1", Endpoint: ep, Reliable: true, Evaluator: MatchAllEvaluator{}},
			{ID: "r2", Endpoint: ep, Reliable: true, Evaluator: EvaluatorFunc(func(RecordView) (bool, error) {
				return false, errors.New("boom")
			})},
			{ID: "o1", Endpoint: ep, Reliable: false, Evaluator: MatchAllEvaluator{}},
		}, batch, 4, nil)
		require.NoError(t, err)

		sel, optErrs, err := policy.Evaluate(msg)
		require.Error(t, err)
		var relErr *ReliableEvalError
		require.ErrorAs(t, err, &relErr)
		require.Equal(t, "r2", relErr.EndpointID)
		require.NotContains(t, err.Error(), "boom", "ReliableEvalError.Error() must not expose raw evaluator error text")
		require.Empty(t, sel.Reliable)
		require.Empty(t, sel.Optional)
		require.Empty(t, optErrs)
	})

	t.Run("optional error suppresses only that optional copy", func(t *testing.T) {
		policy, err := NewEndpointPolicyFromTargets([]EndpointTarget{
			{ID: "r1", Endpoint: ep, Reliable: true, Evaluator: MatchAllEvaluator{}},
			{ID: "o1", Endpoint: ep, Reliable: false, Evaluator: EvaluatorFunc(func(RecordView) (bool, error) {
				return false, errors.New("opt-boom")
			})},
			{ID: "o2", Endpoint: ep, Reliable: false, Evaluator: MatchAllEvaluator{}},
		}, batch, 4, nil)
		require.NoError(t, err)

		sel, optErrs, err := policy.Evaluate(msg)
		require.NoError(t, err)
		require.Equal(t, []string{"r1"}, sel.Reliable)
		require.Equal(t, []string{"o2"}, sel.Optional)
		require.Len(t, optErrs, 1)
		require.Equal(t, "o1", optErrs[0].EndpointID)
	})
}

type policyTestSink struct {
	ch chan *message.Payload
}

func (s *policyTestSink) Channel() chan *message.Payload { return s.ch }

func TestProgressReliableSetORSuccessAndAllTerminalLoss(t *testing.T) {
	sink := &policyTestSink{ch: make(chan *message.Payload, 16)}
	progress, err := NewProgress(16, sink)
	require.NoError(t, err)
	defer progress.Close()

	src := sources.NewLogSource("s", &config.LogsConfig{Type: config.FileType})
	makeMsg := func(offset string) *message.Message {
		m := message.NewMessageWithSource([]byte("line"), message.StatusInfo, src, 0)
		m.Origin.Identifier = "file:/tmp/test.log"
		m.Origin.Offset = offset
		return m
	}

	// Record 1: matches R1 + R2. R1 gets permanent 400 (terminal loss) -> must NOT advance while R2 is pending.
	// Then R2 succeeds -> advances checkpoint to offset 10.
	m1 := makeMsg("10")
	tok1, err := progress.Reserve(context.Background(), m1, 1)
	require.NoError(t, err)
	excluded, err := progress.AttachEndpointSelection(tok1, EndpointSelection{Reliable: []string{"r1", "r2"}, Optional: []string{"us5"}})
	require.NoError(t, err)
	require.False(t, excluded)

	meta1 := m1.MessageMetadata
	progress.CompleteReliableTerminalLoss("r1", message.NewPayload([]*message.MessageMetadata{&meta1}, nil, "", 0))
	require.False(t, progress.IsRecordDone(tok1))
	select {
	case p := <-sink.ch:
		t.Fatalf("unexpected checkpoint on single terminal loss: %v", p.MessageMetas[0].Origin.Offset)
	default:
	}

	// Optional endpoint ID cannot satisfy reliable set.
	progress.CompleteReliableSuccess("us5", message.NewPayload([]*message.MessageMetadata{&meta1}, nil, "", 0))
	require.False(t, progress.IsRecordDone(tok1))

	// R2 succeeds -> completes immediately (reliable-set OR).
	progress.CompleteReliableSuccess("r2", message.NewPayload([]*message.MessageMetadata{&meta1}, nil, "", 0))
	require.True(t, progress.IsRecordDone(tok1))
	select {
	case p := <-sink.ch:
		require.Equal(t, "10", p.MessageMetas[0].Origin.Offset)
	case <-time.After(2 * time.Second):
		t.Fatal("checkpoint did not advance after R2 success")
	}

	// Record 2: matches R1 + R2. Both R1 and R2 terminally lose -> completes only after BOTH lose.
	m2 := makeMsg("20")
	tok2, err := progress.Reserve(context.Background(), m2, 1)
	require.NoError(t, err)
	excluded, err = progress.AttachEndpointSelection(tok2, EndpointSelection{Reliable: []string{"r1", "r2"}})
	require.NoError(t, err)
	require.False(t, excluded)
	meta2 := m2.MessageMetadata
	progress.CompleteReliableTerminalLoss("r1", message.NewPayload([]*message.MessageMetadata{&meta2}, nil, "", 0))
	require.False(t, progress.IsRecordDone(tok2))
	progress.CompleteReliableTerminalLoss("r2", message.NewPayload([]*message.MessageMetadata{&meta2}, nil, "", 0))
	require.True(t, progress.IsRecordDone(tok2))
	select {
	case p := <-sink.ch:
		require.Equal(t, "20", p.MessageMetas[0].Origin.Offset)
	case <-time.After(2 * time.Second):
		t.Fatal("checkpoint did not advance after all-terminal-loss")
	}

	// Record 3 (mandatory R1) followed by Record 4 (no-recipient) and Record 5 (optional-only):
	// Later local exclusions/optional-only MUST NOT advance checkpoint past pending Record 3!
	m3, m4, m5 := makeMsg("30"), makeMsg("40"), makeMsg("50")
	tok3, err := progress.Reserve(context.Background(), m3, 1)
	require.NoError(t, err)
	_, err = progress.AttachEndpointSelection(tok3, EndpointSelection{Reliable: []string{"r1"}})
	require.NoError(t, err)

	tok4, err := progress.Reserve(context.Background(), m4, 1)
	require.NoError(t, err)
	excluded, err = progress.AttachEndpointSelection(tok4, EndpointSelection{})
	require.NoError(t, err)
	require.True(t, excluded)
	require.True(t, progress.IsRecordDone(tok4))

	tok5, err := progress.Reserve(context.Background(), m5, 1)
	require.NoError(t, err)
	excluded, err = progress.AttachEndpointSelection(tok5, EndpointSelection{Optional: []string{"us5"}})
	require.NoError(t, err)
	require.False(t, excluded)
	_, opts, optOnly := progress.EndpointRecipients(tok5)
	require.True(t, optOnly)
	require.Equal(t, []string{"us5"}, opts)
	progress.CompleteLocal(tok5)
	require.True(t, progress.IsRecordDone(tok5))

	// Nothing may publish while m3 is still pending.
	select {
	case p := <-sink.ch:
		t.Fatalf("later local completions crossed pending m3: %v", p.MessageMetas[0].Origin.Offset)
	default:
	}

	// Completing m3 releases 30, 40, 50 in order.
	meta3 := m3.MessageMetadata
	progress.CompleteReliableSuccess("r1", message.NewPayload([]*message.MessageMetadata{&meta3}, nil, "", 0))
	for _, expected := range []string{"30", "40", "50"} {
		select {
		case p := <-sink.ch:
			require.Equal(t, expected, p.MessageMetas[0].Origin.Offset)
		case <-time.After(2 * time.Second):
			t.Fatalf("expected checkpoint %s", expected)
		}
	}
}

func TestProgressReserveSharedReclaimsChurningIdentifiers(t *testing.T) {
	sink := &policyTestSink{ch: make(chan *message.Payload, 64)}
	// Tiny capacity (4) to prove churning >4 distinct shared-ingress identifiers never exhausts capacity.
	progress, err := NewProgress(4, sink)
	require.NoError(t, err)
	defer progress.Close()

	src := sources.NewLogSource("docker", &config.LogsConfig{Type: config.DockerType})
	for i := 0; i < 50; i++ {
		id := "docker:container-" + string(rune('a'+i%26)) + "-" + time.Duration(i).String()
		m := message.NewMessageWithSource([]byte("line"), message.StatusInfo, src, 0)
		m.Origin.Identifier = id
		m.Origin.Offset = "1"
		tok, err := progress.ReserveShared(context.Background(), m, 1)
		require.NoError(t, err)
		_, err = progress.AttachEndpointSelection(tok, EndpointSelection{Reliable: []string{"us1"}})
		require.NoError(t, err)
		meta := m.MessageMetadata
		progress.CompleteReliableSuccess("us1", message.NewPayload([]*message.MessageMetadata{&meta}, nil, "", 0))
		select {
		case <-sink.ch:
		case <-time.After(2 * time.Second):
			t.Fatalf("container %d checkpoint did not advance", i)
		}
		require.False(t, progress.OwnsSource(id), "idle shared-ingress source must be reclaimed")
	}

	// Explicit file source registration must still have full capacity available.
	require.NoError(t, progress.RegisterSource("file:/tmp/active.log", 1))
	require.True(t, progress.OwnsSource("file:/tmp/active.log"))

	// PendingSharedGroups returns unresolved groups for ephemeral shared-ingress
	// sources while PendingGroups("") remains strictly anonymous.
	anonMsg := message.NewMessageWithSource([]byte("anon"), message.StatusInfo, src, 0)
	anonTok, err := progress.Reserve(context.Background(), anonMsg, 0)
	require.NoError(t, err)
	_, err = progress.AttachEndpointSelection(anonTok, EndpointSelection{Reliable: []string{"anon-ep"}})
	require.NoError(t, err)

	sharedMsg := message.NewMessageWithSource([]byte("shared"), message.StatusInfo, src, 0)
	sharedMsg.Origin.Identifier = "docker:pending-shared"
	sharedMsg.Origin.Offset = "1"
	sharedTok, err := progress.ReserveShared(context.Background(), sharedMsg, 1)
	require.NoError(t, err)
	_, err = progress.AttachEndpointSelection(sharedTok, EndpointSelection{Reliable: []string{"shared-ep"}})
	require.NoError(t, err)

	require.Equal(t, []string{"anon-ep"}, progress.PendingGroups(""))
	require.Equal(t, []string{"shared-ep"}, progress.PendingSharedGroups())
}
