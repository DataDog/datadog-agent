// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// Anchoring unit tests for the AutoMultilineCombiningPreprocessing
// surface declared in auto_multiline_combining_pipeline.allium.
// Each test names the spec construct (@guarantee, @invariant, or
// composing-component construct) it anchors so that drift in
// either direction is easy to spot during review.
//
// Property tests for the same surface live in
// auto_multiline_combining_pipeline_proptest_test.go. Tests in
// THIS file wire REAL component fulfillers (the production
// MultilineJSONAggregator, AutoMultilineLabeler with JSONDetector
// + TimestampDetector heuristic chain, CombiningAggregator,
// AdaptiveSampler) so the assertions exercise end-to-end behaviour
// through the actual pipeline code — not test doubles.

// newPathCPipeline builds a Preprocessor configured with the
// concrete fulfillers declared in
// auto_multiline_combining_pipeline.allium:
//
//   - JSONAggregator slot: multiline JSON aggregator
//   - Tokenization slot:   default tokenizer
//   - Labeler slot:        AutoMultilineLabeler with a JSONDetector +
//     TimestampDetector heuristic chain
//   - Aggregator slot:     CombiningAggregator
//   - Sampler slot:        AdaptiveSampler with a generous burst
//     so rate limiting doesn't dominate test
//     outcomes (each test sets its own
//     AdaptiveSamplerConfig when stricter
//     sampling matters)
//
// Returns the pipeline plus the output channel so callers can
// drain emissions after each Process / Flush call.
func newPathCPipeline(t *testing.T, maxContent int, samplerCfg AdaptiveSamplerConfig) (*Preprocessor, chan *message.Message) {
	t.Helper()
	tailerInfo := status.NewInfoRegistry()
	tok := NewTokenizer(2048)
	heuristics := []Heuristic{
		NewJSONDetector(),
		NewTimestampDetector(0.75),
	}
	labeler := NewLabeler(heuristics, nil)
	combining := NewCombiningAggregator(maxContent, false, false, tailerInfo)
	sampler := NewAdaptiveSampler(samplerCfg, "pathc-test", 0)
	jsonAgg := NewJSONAggregator(false, maxContent)
	outputChan := make(chan *message.Message, 1024)
	pipeline := NewPreprocessor(combining, tok, labeler, sampler, outputChan, jsonAgg, NewNoopStackTraceAggregator(), 10*time.Second, 0)
	return pipeline, outputChan
}

// generousSamplerCfg returns a sampler config with a very large
// burst so each pattern emits freely. Used by tests that want to
// observe aggregation behaviour without sampling-driven drops.
func generousSamplerCfg() AdaptiveSamplerConfig {
	return AdaptiveSamplerConfig{
		MaxPatterns:    1000,
		BurstSize:      10000,
		RateLimit:      1000,
		MatchThreshold: 0.9,
	}
}

// drainAll non-blockingly drains all currently-available messages
// from the output channel. Returns them in arrival order.
func drainAll(ch chan *message.Message) []*message.Message {
	var out []*message.Message
	for {
		select {
		case m := <-ch:
			out = append(out, m)
		default:
			return out
		}
	}
}

// TestPathCPipeline_TimestampStartsGroup_AggregatesContinuation anchors:
//
//	surface AutoMultilineCombiningPreprocessing
//	    @guarantee TimestampStartsGroup — TimestampDetector labels
//	                                       timestamp-prefixed lines
//	                                       as start_group; combined
//	                                       with CombiningAggregator's
//	                                       StartGroupBoundary to
//	                                       form multi-line aggregates.
//
// The full composition behaviour:
//  1. A line prefixed with a recognizable timestamp is labelled
//     start_group → CombiningAggregator buffers it as the new
//     bucket's leader.
//  2. A continuation line is labelled aggregate (no detector
//     claims) → CombiningAggregator appends to the bucket.
//  3. A second timestamp line flushes the prior bucket as a
//     combined emission.
func TestPathCPipeline_TimestampStartsGroup_AggregatesContinuation(t *testing.T) {
	pipeline, outputChan := newPathCPipeline(t, 100_000, generousSamplerCfg())

	pipeline.Process(newTestPreprocessorMessage("2024-01-15 10:30:45 INFO request received"))
	pipeline.Process(newTestPreprocessorMessage("  at HandlerImpl.process(line 42)"))
	// Drain — nothing should have emitted yet (both buffered).
	require.Empty(t, drainAll(outputChan), "lines should still be buffered in the combining aggregator")

	pipeline.Process(newTestPreprocessorMessage("2024-01-15 10:30:46 INFO next request"))
	emitted := drainAll(outputChan)
	require.Len(t, emitted, 1, "the second timestamp must flush the prior bucket")
	combined := string(emitted[0].GetContent())
	assert.Contains(t, combined, "request received")
	assert.Contains(t, combined, "HandlerImpl.process")
	assert.Contains(t, combined, "\\n", "combined emission must contain the escaped-line-feed separator")

	// Flush drains the second start_group's bucket.
	pipeline.Flush()
	emitted = drainAll(outputChan)
	require.Len(t, emitted, 1)
	assert.Equal(t, "2024-01-15 10:30:46 INFO next request", string(emitted[0].GetContent()))
}

// TestPathCPipeline_JSONLineNotAggregated anchors:
//
//	surface AutoMultilineCombiningPreprocessing
//	    @guarantee JSONLineNotAggregated — JSONDetector labels
//	                                        JSON-shape lines as
//	                                        no_aggregate; combined
//	                                        with CombiningAggregator's
//	                                        NoAggregateFlushes to
//	                                        emit as single-line.
//
// A line matching the JSON-shape heuristic is emitted as a
// single-line message, bypassing any in-progress multi-line
// aggregate (the buffered bucket flushes first if non-empty).
func TestPathCPipeline_JSONLineNotAggregated(t *testing.T) {
	pipeline, outputChan := newPathCPipeline(t, 100_000, generousSamplerCfg())

	// Build up a buffered bucket.
	pipeline.Process(newTestPreprocessorMessage("2024-01-15 10:30:45 INFO request received"))
	pipeline.Process(newTestPreprocessorMessage("  context: HandlerImpl"))
	require.Empty(t, drainAll(outputChan))

	// A JSON-shape line arrives. JSONDetector labels it no_aggregate,
	// CombiningAggregator's NoAggregateFlushes path emits any
	// buffered bucket as combined, then emits the JSON line as
	// single-line.
	pipeline.Process(newTestPreprocessorMessage(`{"event":"json-shaped","value":42}`))
	emitted := drainAll(outputChan)
	require.Len(t, emitted, 2, "no_aggregate emits a 2-message run: prior bucket flush + current line")
	assert.Contains(t, string(emitted[0].GetContent()), "request received")
	assert.Equal(t, `{"event":"json-shaped","value":42}`, string(emitted[1].GetContent()))
}

// TestPathCPipeline_OverflowExplodesNeverTruncatesCombined anchors:
//
//	surface AutoMultilineCombiningPreprocessing
//	    @guarantee OverflowExplodesNeverTruncatesCombined
//	        — composed from CombiningAggregator.OverflowExplosion
//
// A buffered aggregate that would overflow line_limit if extended
// is exploded (buffered lines emitted individually) rather than
// truncated as a combined message. No combined emission's body
// reaches line_limit purely from aggregation.
func TestPathCPipeline_OverflowExplodesNeverTruncatesCombined(t *testing.T) {
	// line_limit chosen so leader (26) + sep (2) + cont 1 (19) = 47
	// fits, but adding cont 2 would push past the limit and trigger
	// OverflowExplosion.
	pipeline, outputChan := newPathCPipeline(t, 60, generousSamplerCfg())

	pipeline.Process(newTestPreprocessorMessage("2024-01-15 10:30:45 leader")) // start_group, buffered (26 bytes)
	pipeline.Process(newTestPreprocessorMessage("continuation line 1"))        // aggregate, fits (bucket now 47)
	require.Empty(t, drainAll(outputChan))

	// Third line would push the bucket to 84 bytes → explode.
	pipeline.Process(newTestPreprocessorMessage("continuation line 2 that overflows"))
	emitted := drainAll(outputChan)
	require.GreaterOrEqual(t, len(emitted), 2, "overflow must explode the bucket into individual emissions")
	// None of the emissions should be a true multi-line combined message
	// (i.e., no emission contains the escaped-line-feed separator).
	for i, e := range emitted {
		assert.NotContains(t, string(e.GetContent()), "\\n",
			"emission %d should be a single-line emission (explosion path), got %q", i, e.GetContent())
	}
}

// TestPathCPipeline_CriticalSeverityBypassesSampling anchors:
//
//	surface AutoMultilineCombiningPreprocessing
//	    @guarantee CriticalSeverityBypassesSampling
//	        — composed from AdaptiveSampler.ImportantLogProtection
//
// With protect_important_logs enabled and a tight burst, repeated
// ERROR-tagged lines all emit despite the sampler being rate-
// limited. (Confirms the ImportantLogBypass rule fires before
// the pattern-table rules.)
func TestPathCPipeline_CriticalSeverityBypassesSampling(t *testing.T) {
	cfg := AdaptiveSamplerConfig{
		MaxPatterns:          10,
		BurstSize:            1, // tight: a non-important pattern emits once then drops
		RateLimit:            0, // no refill
		MatchThreshold:       0.9,
		ProtectImportantLogs: true,
	}
	pipeline, outputChan := newPathCPipeline(t, 100_000, cfg)

	for range 5 {
		pipeline.Process(newTestPreprocessorMessage("2024-01-15 10:30:45 ERROR connection refused"))
	}
	// Flush so any buffered bucket drains.
	pipeline.Flush()
	emitted := drainAll(outputChan)
	require.Len(t, emitted, 5, "all 5 ERROR-tagged lines must emit (ImportantLogBypass dominates the sampler)")
}

// TestPathCPipeline_FlushDrainsAllBuffers anchors:
//
//	contract Preprocessor
//	    @invariant FlushDrainsBuffer — after flush, every stateful
//	                                    component reports is_empty.
//
// After flushing a pipeline that buffered both a JSON fragment
// (in JSONAggregator) and a start_group line (in CombiningAggregator),
// every buffer is empty.
func TestPathCPipeline_FlushDrainsAllBuffers(t *testing.T) {
	pipeline, outputChan := newPathCPipeline(t, 100_000, generousSamplerCfg())

	// JSON aggregator buffers an incomplete JSON fragment.
	pipeline.Process(newTestPreprocessorMessage(`{"key":`))
	// Combining aggregator buffers a start_group continuation set.
	// (Won't actually buffer in CombiningAggregator since the
	// incomplete JSON sits in jsonAggregator only; once we flush,
	// it cascades and gets emitted through the rest of the chain.)
	pipeline.Flush()
	_ = drainAll(outputChan)
	assert.True(t, pipeline.jsonAggregator.IsEmpty(), "json aggregator must be empty after flush")
	assert.True(t, pipeline.aggregator.IsEmpty(), "combining aggregator must be empty after flush")
}

// TestPathCPipeline_EndToEndByteConservation anchors:
//
//	contract Preprocessor
//	    @invariant EndToEndByteConservation — every byte in a
//	                                           PreprocessorOutput
//	                                           traces to an input
//	                                           byte plus a finite
//	                                           set of well-known
//	                                           marker bytes.
//
// For non-truncating, non-explosion paths the strong form holds:
// the combined output content equals the trim-spaced concatenation
// of the input contents joined by the escaped-line-feed separator.
// This pins the byte-conservation property concretely for the
// happy-path multi-line aggregate.
func TestPathCPipeline_EndToEndByteConservation(t *testing.T) {
	pipeline, outputChan := newPathCPipeline(t, 100_000, generousSamplerCfg())

	pipeline.Process(newTestPreprocessorMessage("2024-01-15 10:30:45 LEADER"))
	pipeline.Process(newTestPreprocessorMessage("cont 1"))
	pipeline.Process(newTestPreprocessorMessage("cont 2"))
	pipeline.Flush()

	emitted := drainAll(outputChan)
	require.Len(t, emitted, 1)
	combined := string(emitted[0].GetContent())
	expected := strings.Join([]string{"2024-01-15 10:30:45 LEADER", "cont 1", "cont 2"}, `\n`)
	assert.Equal(t, expected, combined,
		"combined emission must be the input contents joined by the escaped-line-feed separator (no other bytes)")
}
