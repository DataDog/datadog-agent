// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && zlib && (windows || darwin)

package native

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/capture"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/output"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/safety"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	taggerimpl "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/impl"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/util/compression/selector"
)

// Exercise the actual demultiplexer: its implicit agent-up service check does
// not pass through Series, and must be disabled before serializer construction.
func TestCaptureDemultiplexerPersistsOnlySanitizedSeries(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("telemetry.enabled", false)
	logger := logmock.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	destinations, err := (safety.Config{Site: safety.Site}).Resolve(func(string) string { return "" })
	require.NoError(t, err)
	recorder := output.NewRecorder()
	sanitizer, err := capture.NewSanitizer()
	require.NoError(t, err)
	p, err := output.New(ctx, destinations, "recording-only", recorder)
	require.NoError(t, err)
	defer p.Close()
	ha, err := haagent.NewComponent(haagent.Requires{Logger: logger, AgentConfig: p.Config, Hostname: output.Hostname("RAW-CAPTURE-HOST-SECRET")})
	require.NoError(t, err)
	opts := captureDemultiplexerOptions()
	opts.CaptureTransformer = sanitizer
	compressor := selector.FromConfig(cfg)
	demux := aggregator.InitAndStartAgentDemultiplexer(logger, p.Serializer.Forwarder, nil, opts, noEvents{}, ha.Comp, compressor, taggerimpl.NewComponent(), filterlist.NewNoopFilterList(), "RAW-CAPTURE-HOST-SECRET")
	defer demux.Stop()
	sender, err := demux.GetDefaultSender()
	require.NoError(t, err)
	sender.Gauge("system.cpu.user", 5, "RAW-CAPTURE-HOST-SECRET", []string{"user:RAW-CAPTURE-HOST-SECRET"})
	sender.ServiceCheck("private.check", servicecheck.ServiceCheckOK, "RAW-CAPTURE-HOST-SECRET", nil, "RAW-CAPTURE-HOST-SECRET")
	sender.Event(event.Event{Title: "RAW-CAPTURE-HOST-SECRET", Text: "RAW-CAPTURE-HOST-SECRET", Host: "RAW-CAPTURE-HOST-SECRET"})
	sender.Commit()
	demux.ForceFlushToSerializer(time.Now().Add(time.Second), true, true)
	require.NoError(t, p.Wait(ctx))
	refs, err := recorder.Wait(ctx, 1)
	require.NoError(t, err)
	for _, ref := range refs {
		require.True(t, strings.Contains(ref.Path, "/series"), "unexpected uncaptured output path: %s", ref.Path)
		body, err := compressor.Decompress(ref.Body)
		require.NoError(t, err)
		require.False(t, bytes.Contains(body, []byte("RAW-CAPTURE-HOST-SECRET")))
		require.Contains(t, string(body), "capture-host")
	}
}
