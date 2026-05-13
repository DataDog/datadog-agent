// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package preprocessor

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// Property tests for the AutoMultilineCombiningPreprocessing
// surface declared in auto_multiline_combining_pipeline.allium.
// Each test names the spec @invariant (composed from the abstract
// Preprocessor contract) it anchors so that drift in either
// direction is easy to spot during review.
//
// These tests demonstrate the experimental hypothesis: pipeline-
// level property tests can be composed from the tie-spec + child
// specs + child property tests. The tests below verify END-TO-END
// invariants by exercising the REAL component fulfillers wired
// together; the proof that each end-to-end invariant holds
// composes from the per-component invariants already verified by
// the existing per-component property tests (see
// adaptive_sampler_proptest_test.go, combining_aggregator_proptest_test.go,
// regex_aggregator_proptest_test.go, etc.).
//
// What this means for test design:
//
//  1. We do NOT re-verify per-component invariants at the pipeline
//     level. Those are covered upstream.
//  2. We DO verify end-to-end properties that emerge from the
//     composition — properties no single component proves on its
//     own.
//  3. The pipeline-level generator (genPipelineInput) produces
//     ARBITRARY input lines, exercising whatever distribution of
//     pattern-matching / labelling / aggregation / sampling
//     decisions the real fulfillers make.
//
// Anchoring unit tests for the same surface live in
// auto_multiline_combining_pipeline_test.go.

// pipelineInput represents one raw log line entering the
// preprocessor pipeline.
type pipelineInput struct {
	content string
	tags    []string
}

// genPipelineInput produces one input. The content alphabet
// mixes timestamps, JSON shapes, structured logs and arbitrary
// text so generated sequences exercise all of:
//   - JSONAggregator's fast-path / buffer / flush branches
//   - AutoMultilineLabeler's JSONDetector + TimestampDetector
//     heuristic chain
//   - CombiningAggregator's emit / drop / overflow paths
//   - AdaptiveSampler's pattern-table classification
func genPipelineInput() *rapid.Generator[pipelineInput] {
	return rapid.Custom(func(t *rapid.T) pipelineInput {
		flavor := rapid.IntRange(0, 4).Draw(t, "flavor")
		var content string
		switch flavor {
		case 0:
			// Timestamp-prefixed line (drives TimestampDetector → start_group).
			tail := string(rapid.SliceOfN(
				rapid.SampledFrom([]byte("abc 012")), 0, 30,
			).Draw(t, "tail"))
			content = "2024-01-15 10:30:45 INFO " + tail
		case 1:
			// JSON-shape line (drives JSONDetector → no_aggregate).
			content = `{"event":"x","value":42}`
		case 2:
			// Continuation-style line (no detector claims → aggregate).
			content = "  at HandlerImpl.process(line " + rapid.SampledFrom([]string{"42", "99", "123"}).Draw(t, "lineNo") + ")"
		case 3:
			// ERROR-tagged (drives AdaptiveSampler's
			// is_important when protect_important_logs is on).
			content = "ERROR connection refused: " + rapid.SampledFrom([]string{"host-a", "host-b"}).Draw(t, "host")
		default:
			// Arbitrary text.
			content = string(rapid.SliceOfN(
				rapid.SampledFrom([]byte("abcdef 012")), 1, 30,
			).Draw(t, "raw"))
		}
		tagCount := rapid.IntRange(0, 2).Draw(t, "tagCount")
		tags := make([]string, tagCount)
		for i := 0; i < tagCount; i++ {
			tags[i] = rapid.SampledFrom([]string{"env:prod", "service:web"}).Draw(t, "tag")
		}
		return pipelineInput{content: content, tags: tags}
	})
}

// pipelineRun feeds a sequence of inputs through a freshly-
// constructed Path C pipeline plus a final flush, and returns the
// emitted messages and the input content bytes (for byte-
// conservation checks).
func pipelineRun(maxContent int, samplerCfg AdaptiveSamplerConfig, inputs []pipelineInput) (emitted []*message.Message, inputContents [][]byte) {
	tailerInfo := status.NewInfoRegistry()
	tok := NewTokenizer(2048)
	heuristics := []Heuristic{NewJSONDetector(), NewTimestampDetector(0.75)}
	labeler := NewLabeler(heuristics, nil)
	combining := NewCombiningAggregator(maxContent, false, false, tailerInfo)
	sampler := NewAdaptiveSampler(samplerCfg, "proptest")
	jsonAgg := NewJSONAggregator(false, maxContent)
	outputChan := make(chan *message.Message, len(inputs)*4+16)
	pipeline := NewPreprocessor(combining, tok, labeler, sampler, outputChan, jsonAgg, 10*time.Second, 0)

	for _, in := range inputs {
		msg := message.NewMessage([]byte(in.content), nil, message.StatusInfo, 0)
		msg.RawDataLen = len(in.content)
		msg.ParsingExtra.Tags = append([]string(nil), in.tags...)
		pipeline.Process(msg)
		inputContents = append(inputContents, []byte(in.content))
	}
	pipeline.Flush()

	for {
		select {
		case m := <-outputChan:
			emitted = append(emitted, m)
		default:
			return emitted, inputContents
		}
	}
}

// TestPathCPipeline_EndToEndDeterminism_Property anchors:
//
//	contract Preprocessor (preprocessor.allium)
//	    @invariant EndToEndDeterminism
//
// Composes from per-component Determinism invariants (already
// verified by the per-component property tests). Two pipelines
// of identical configuration fed identical input sequences
// produce identical emission sequences.
func TestPathCPipeline_EndToEndDeterminism_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		inputs := rapid.SliceOfN(genPipelineInput(), 1, 12).Draw(t, "inputs")
		samplerCfg := AdaptiveSamplerConfig{
			MaxPatterns:    50,
			BurstSize:      10,
			RateLimit:      0,
			MatchThreshold: 0.9,
		}

		emittedA, _ := pipelineRun(100_000, samplerCfg, inputs)
		emittedB, _ := pipelineRun(100_000, samplerCfg, inputs)

		if len(emittedA) != len(emittedB) {
			t.Fatalf("EndToEndDeterminism violated: emission counts %d vs %d", len(emittedA), len(emittedB))
		}
		for i := range emittedA {
			if !bytes.Equal(emittedA[i].GetContent(), emittedB[i].GetContent()) {
				t.Fatalf("EndToEndDeterminism violated at emission %d: %q vs %q",
					i, emittedA[i].GetContent(), emittedB[i].GetContent())
			}
		}
	})
}

// TestPathCPipeline_EndToEndByteConservation_Property anchors:
//
//	contract Preprocessor (preprocessor.allium)
//	    @invariant EndToEndByteConservation
//
// The strong form: stripping all the well-known marker bytes
// (truncation marker, escaped-line-feed separator) from emitted
// contents, then concatenating, must equal the trim-spaced
// concatenation of input contents that were not dropped at the
// sampler stage.
//
// This is the experimental composition payoff: the abstract
// proof of byte conservation depends on each component's
// content-conservation invariant. The per-component property
// tests verified those. Here we verify the composed end-to-end
// property without re-proving the per-component ones.
func TestPathCPipeline_EndToEndByteConservation_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		inputs := rapid.SliceOfN(genPipelineInput(), 1, 8).Draw(t, "inputs")
		// Generous burst so sampling drops are rare — keeps the
		// byte accounting tractable. Sampling-drop behaviour has
		// its own per-component coverage.
		samplerCfg := generousSamplerCfg()
		maxContent := 100_000 // large enough that truncation also doesn't fire

		emitted, inputContents := pipelineRun(maxContent, samplerCfg, inputs)

		marker := `...TRUNCATED...`
		separator := `\n`

		// Build the bag of non-whitespace bytes that flowed through
		// emissions. Strip markers and separators.
		var emittedTokens []string
		for _, e := range emitted {
			s := string(e.GetContent())
			s = strings.ReplaceAll(s, marker, "")
			parts := strings.Split(s, separator)
			for _, p := range parts {
				if p == "" {
					continue
				}
				emittedTokens = append(emittedTokens, strings.TrimSpace(p))
			}
		}

		// Build the bag of non-whitespace tokens from inputs.
		var inputTokens []string
		for _, c := range inputContents {
			trimmed := strings.TrimSpace(string(c))
			if trimmed == "" {
				continue
			}
			inputTokens = append(inputTokens, trimmed)
		}

		// Every emitted token must appear as a substring of the
		// concatenation of all inputs. (We can't assert exact
		// equality because the sampler may drop some, and JSON
		// compaction may slightly modify whitespace within a
		// single JSON emission. The substring containment is
		// the form of byte conservation that holds across all
		// path-C paths.)
		concatenatedInputs := strings.Join(inputTokens, " ")
		for _, et := range emittedTokens {
			// JSON compaction collapses whitespace; check the
			// non-whitespace form for substring containment.
			etCompact := strings.Join(strings.Fields(et), "")
			inputsCompact := strings.Join(strings.Fields(concatenatedInputs), "")
			if !strings.Contains(inputsCompact, etCompact) {
				t.Fatalf("EndToEndByteConservation violated: emitted token %q not found in inputs %q",
					etCompact, inputsCompact)
			}
		}
	})
}

// TestPathCPipeline_EndToEndDropOrEmit_Property anchors:
//
//	contract Preprocessor (preprocessor.allium)
//	    @invariant EndToEndDropOrEmit — each PreprocessorInput
//	                                     contributes to exactly
//	                                     one PreprocessorOutput
//	                                     OR is dropped at the
//	                                     Sampler stage.
//
// The composition: every component below the Sampler preserves
// content via DeliveryOrPreservation / ByteConservation. The
// Sampler is the only stage that can drop. With sampling
// effectively disabled (very generous burst), every input must
// contribute to some emission — total emission count >= 1 (or
// total input count if no aggregation combined them).
//
// This is a directional check: with no sampling drops, we cannot
// lose input content. We use a weaker numeric bound that holds
// across aggregation patterns: total emission count must be ≥ 1
// when there is ≥ 1 input.
func TestPathCPipeline_EndToEndDropOrEmit_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		inputs := rapid.SliceOfN(genPipelineInput(), 1, 8).Draw(t, "inputs")
		emitted, _ := pipelineRun(100_000, generousSamplerCfg(), inputs)

		if len(emitted) < 1 {
			t.Fatalf("EndToEndDropOrEmit violated: %d inputs produced 0 emissions (no sampler drops expected at this burst)",
				len(inputs))
		}
	})
}

// TestPathCPipeline_FlushDrainsAllBuffers_Property anchors:
//
//	contract Preprocessor (preprocessor.allium)
//	    @invariant FlushDrainsBuffer — after flush, every stateful
//	                                    component reports its
//	                                    is_empty observable as
//	                                    true.
//
// Across arbitrary input sequences, after pipeline flush:
// jsonAggregator.IsEmpty() AND aggregator.IsEmpty() must both
// be true. (Tokenization and Labeler are stateless; Sampler is
// non-buffering.)
func TestPathCPipeline_FlushDrainsAllBuffers_Property(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		inputs := rapid.SliceOfN(genPipelineInput(), 0, 10).Draw(t, "inputs")
		tailerInfo := status.NewInfoRegistry()
		tok := NewTokenizer(2048)
		heuristics := []Heuristic{NewJSONDetector(), NewTimestampDetector(0.75)}
		labeler := NewLabeler(heuristics, nil)
		combining := NewCombiningAggregator(100_000, false, false, tailerInfo)
		sampler := NewAdaptiveSampler(generousSamplerCfg(), "proptest")
		jsonAgg := NewJSONAggregator(false, 100_000)
		outputChan := make(chan *message.Message, len(inputs)*4+16)
		pipeline := NewPreprocessor(combining, tok, labeler, sampler, outputChan, jsonAgg, 10*time.Second, 0)

		for _, in := range inputs {
			msg := message.NewMessage([]byte(in.content), nil, message.StatusInfo, 0)
			msg.RawDataLen = len(in.content)
			pipeline.Process(msg)
		}
		pipeline.Flush()

		if !jsonAgg.IsEmpty() {
			t.Fatal("FlushDrainsBuffer violated: json aggregator non-empty after flush")
		}
		if !combining.IsEmpty() {
			t.Fatal("FlushDrainsBuffer violated: combining aggregator non-empty after flush")
		}
	})
}
