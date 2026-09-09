// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Anchoring unit tests for the AdaptiveSampling surface declared
// in adaptive_sampler.allium. Each test names the spec construct
// (@guarantee, rule, or @invariant) it anchors so that drift in
// either direction is easy to spot during review.
//
// Property tests for the same surface live in
// adaptive_sampler_proptest_test.go. The bulk of the scenario
// coverage (per-rule behaviour, credit refill, eviction,
// important-log protection) lives in sampler_test.go with its own
// anchoring docstrings; the tests in THIS file cover Sampler
// contract invariants those existing tests don't yet anchor.

// TestAdaptiveSampler_ContentBytePassthrough_Emit anchors:
//
//	contract Sampler (sampler.allium)
//	    @invariant ContentBytePassthrough — when process returns
//	                                         a Message value, the
//	                                         returned Message's
//	                                         content bytes are
//	                                         byte-equal to the
//	                                         input msg's content
//	                                         bytes.
//
// The sampler returns the input message pointer on emit (verified
// via assert.Same — strictly stronger than byte equality, since
// pointer identity implies the message's content is necessarily
// preserved unless the sampler explicitly mutates it via
// SetContent). AdaptiveSampler never calls SetContent.
func TestAdaptiveSampler_ContentBytePassthrough_Emit(t *testing.T) {
	s := newSampler(10, 5.0, 0)
	msg := message.NewMessage([]byte("original content bytes"), nil, message.StatusInfo, 0)
	out := s.Process(msg, patternA)
	require.NotNil(t, out)
	assert.Same(t, msg, out, "emitted message must be the input message pointer")
	assert.Equal(t, []byte("original content bytes"), out.GetContent(),
		"emitted content bytes must equal input content bytes")
}

// TestAdaptiveSampler_TagAugmentationOnly_PreservesInputTags anchors:
//
//	contract Sampler (sampler.allium)
//	    @invariant TagAugmentationOnly — returned message's tags
//	                                      are a superset of the
//	                                      input msg's tags.
//
// Pre-existing tags on the input message survive sampling
// unchanged. The sampler may APPEND adaptive_sampler_sampled_count
// but never removes or modifies existing tags.
func TestAdaptiveSampler_TagAugmentationOnly_PreservesInputTags(t *testing.T) {
	s := newSampler(10, 5.0, 0)
	msg := testMsg()
	msg.ParsingExtra.Tags = []string{"upstream:tag1", "upstream:tag2"}

	out := s.Process(msg, patternA)
	require.NotNil(t, out)
	assert.Contains(t, out.ParsingExtra.Tags, "upstream:tag1",
		"existing tags must be preserved on emit")
	assert.Contains(t, out.ParsingExtra.Tags, "upstream:tag2",
		"existing tags must be preserved on emit")
}

// TestAdaptiveSampler_NoMessageFabrication anchors:
//
//	contract Sampler (sampler.allium)
//	    @invariant NoMessageFabrication — every Message value
//	                                       returned by process
//	                                       originates from a
//	                                       message previously
//	                                       supplied to process.
//
// Every non-nil emission has pointer identity to the input message
// of that specific process call. AdaptiveSampler never constructs
// a Message value.
func TestAdaptiveSampler_NoMessageFabrication(t *testing.T) {
	s := newSampler(10, 5.0, 0)

	for i := range 3 {
		_ = i
		msg := testMsg()
		out := s.Process(msg, patternA)
		if out != nil {
			assert.Same(t, msg, out, "emitted message must be identical to the input of this process call")
		}
	}
}

// TestAdaptiveSampler_DropReturnsNil anchors:
//
//	contract Sampler (sampler.allium)
//	    @invariant Totality — process returns either a Message
//	                           or null.
//	rule DropMatchingLog (adaptive_sampler.allium)
//	    ensures: LogDropped(message)  →  null return from process
//
// A drop emission is exactly the null return — not a returned
// message with is_dropped set, not an empty message. Pin the
// null-return shape as a named anchor.
func TestAdaptiveSampler_DropReturnsNil(t *testing.T) {
	s := newSampler(10, 1.0, 0) // burst=1, no refill
	require.NotNil(t, s.Process(testMsg(), patternA), "first message creates pattern, emits")
	out := s.Process(testMsg(), patternA)
	assert.Nil(t, out, "drop emission must be exactly nil")
}
