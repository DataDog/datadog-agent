// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package routing

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func testSingleHTTPEndpoint(host, key string, reliable bool) *config.Endpoints {
	ep := config.NewMockEndpointWithOptions(map[string]any{
		"api_key":     key,
		"host":        host,
		"port":        443,
		"use_ssl":     true,
		"is_reliable": reliable,
	})
	return config.NewEndpointsWithBatchSettings(ep, nil, false, true, time.Second, 1, 10, 1024*1024, 16)
}

func TestRecordViewExposesStatusAndLocalTags(t *testing.T) {
	src := sources.NewLogSource("app", &config.LogsConfig{Tags: []string{"env:prod", "local_only:yes"}})
	origin := message.NewOrigin(src)
	origin.SetTags([]string{"origin_tag:v1"})
	view := NewRecordView(message.NewMessage([]byte("hello"), origin, message.StatusNotice, 0))
	assert.Equal(t, message.StatusNotice, view.Status())
	assert.ElementsMatch(t, []string{"env:prod", "local_only:yes", "origin_tag:v1"}, view.LocalTags())
}

func TestEndpointDefinitionsValidationAndEvaluation(t *testing.T) {
	_, err := NewEndpointDefinitions(nil)
	require.ErrorIs(t, err, ErrInvalidDefinition)

	// Unreliable-only definitions are rejected.
	_, err = NewEndpointDefinitions([]EndpointTarget{
		{ID: "us5", Endpoints: testSingleHTTPEndpoint("us5.example", "k5", false)},
	})
	require.ErrorIs(t, err, ErrInvalidDefinition)

	// MRF endpoints are rejected in narrow scope.
	mrfEP := testSingleHTTPEndpoint("mrf.example", "km", true)
	mrfEP.Main.IsMRF = true
	mrfEP.Endpoints[0].IsMRF = true
	_, err = NewEndpointDefinitions([]EndpointTarget{
		{ID: "mrf", Endpoints: mrfEP},
	})
	require.ErrorIs(t, err, ErrInvalidDefinition)

	// Valid US1 (reliable, unfiltered) + US5 (unreliable, NOTICE-only).
	defs, err := NewEndpointDefinitions([]EndpointTarget{
		{ID: "us1", Endpoints: testSingleHTTPEndpoint("us1.example", "k1", true)},
		{
			ID:        "us5",
			Endpoints: testSingleHTTPEndpoint("us5.example", "k5", false),
			Evaluator: EvaluatorFunc(func(v RecordView) (bool, error) {
				return v.Status() == message.StatusNotice, nil
			}),
		},
	})
	require.NoError(t, err)

	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/app.log"})
	infoMsg := message.NewMessage([]byte("info"), message.NewOrigin(src), message.StatusInfo, 0)
	rel, opt, _, err := defs.Evaluate(NewRecordView(infoMsg))
	require.NoError(t, err)
	assert.Equal(t, []string{"us1"}, rel)
	assert.Empty(t, opt)

	noticeMsg := message.NewMessage([]byte("notice"), message.NewOrigin(src), message.StatusNotice, 0)
	rel, opt, _, err = defs.Evaluate(NewRecordView(noticeMsg))
	require.NoError(t, err)
	assert.Equal(t, []string{"us1"}, rel)
	assert.Equal(t, []string{"us5"}, opt)

	// Selector error fails evaluation without inventing fallback defaults.
	errDefs, err := NewEndpointDefinitions([]EndpointTarget{
		{
			ID:        "us1",
			Endpoints: testSingleHTTPEndpoint("us1.example", "k1", true),
			Evaluator: EvaluatorFunc(func(RecordView) (bool, error) {
				return false, errors.New("eval failure")
			}),
		},
		{ID: "other", Endpoints: testSingleHTTPEndpoint("other.example", "k2", true)},
	})
	require.NoError(t, err)
	rel, _, reliableError, err := errDefs.Evaluate(NewRecordView(noticeMsg))
	require.ErrorIs(t, err, ErrInvalidSelection)
	assert.True(t, reliableError)
	assert.Equal(t, []string{"other"}, rel)
}

func TestEndpointDefinitionsTotalLimitIncludesOptionalEndpoints(t *testing.T) {
	require.Equal(t, 32, MaxEndpoints)
	targets := []EndpointTarget{{ID: "reliable", Endpoints: testSingleHTTPEndpoint("reliable.example", "key", true)}}
	optional := testSingleHTTPEndpoint("optional.example", "key", false)
	for i := 1; i <= MaxEndpoints; i++ {
		targets = append(targets, EndpointTarget{ID: "optional-" + strconv.Itoa(i), Endpoints: optional})
	}
	defs, err := NewEndpointDefinitions(targets[:MaxEndpoints])
	require.NoError(t, err)
	require.Len(t, defs.Targets(), MaxEndpoints)
	_, err = NewEndpointDefinitions(targets)
	require.ErrorIs(t, err, ErrInvalidDefinition)
}

type endpointTestSink struct{ ch chan *message.Payload }

func (s *endpointTestSink) Channel() chan *message.Payload { return s.ch }

func TestProgressReliableSetORAndTerminalLossSeparation(t *testing.T) {
	sink := &endpointTestSink{ch: make(chan *message.Payload, 8)}
	p, err := NewProgress(8, sink)
	require.NoError(t, err)
	defer p.Close()

	src := sources.NewLogSource("app", &config.LogsConfig{Type: config.FileType, Path: "/tmp/app.log"})
	origin := message.NewOrigin(src)
	origin.Identifier = "file:/tmp/app.log"
	origin.Offset = "10"
	msg := message.NewMessage([]byte("msg"), origin, message.StatusNotice, 0)

	token, err := p.Reserve(context.Background(), msg, 1)
	require.NoError(t, err)
	require.NoError(t, p.AttachEndpointSelection(token, []string{"r1", "r2"}, []string{"us5"}, false))

	rel, opt, delivered := p.EndpointRecipients(token)
	require.NotNil(t, delivered)
	assert.Equal(t, []string{"r1", "r2"}, rel)
	assert.Equal(t, []string{"us5"}, opt)

	payload := message.NewPayload([]*message.MessageMetadata{&msg.MessageMetadata}, nil, "", 0)

	// Optional endpoint ID cannot satisfy reliable progress.
	p.CompleteReliableSuccess("us5", payload)
	assert.Equal(t, 1, p.Pending())

	// Permanent rejection on R1 does not complete the record.
	p.CompleteReliableTerminalLoss("r1", payload)
	assert.Equal(t, 1, p.Pending())

	select {
	case <-delivered:
		t.Fatal("optional success and reliable rejection must not signal reliable delivery")
	default:
	}

	// Success on R2 satisfies reliable-set OR and advances checkpoint.
	p.CompleteReliableSuccess("r2", payload)
	select {
	case <-delivered:
	default:
		t.Fatal("reliable success must signal waiting offers")
	}
	assert.Equal(t, 0, p.Pending())
	select {
	case cp := <-sink.ch:
		require.Len(t, cp.MessageMetas, 1)
		assert.Equal(t, "10", cp.MessageMetas[0].Origin.Offset)
	case <-time.After(time.Second):
		t.Fatal("expected checkpoint after R2 success")
	}
}
