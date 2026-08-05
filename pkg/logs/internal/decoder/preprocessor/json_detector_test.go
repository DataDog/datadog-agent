// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Anchoring unit tests for the JSONDetection surface declared in
// json_detector.allium. Each test names the spec construct
// (@guarantee or @guidance case) it anchors so that drift in either
// direction is easy to spot during review.
//
// Property tests for the same surface live in
// json_detector_proptest_test.go.

// TestJsonDetector anchors:
//
//	surface JSONDetection (json_detector.allium)
//	    @guidance case 2 — shape match with no prior claim:
//	                       set label = no_aggregate, return false
//	    @guidance case 3 — no shape match: take no action, return true
//
// Each row exercises one of the two cases. The detection condition
// is "opening brace `{` followed (possibly with intervening
// whitespace) by either a string-opening `\"` or a closing `}`,
// with leading whitespace permitted." Rows that match this shape
// claim no_aggregate and return false; rows that do not match
// leave the default `aggregate` label and return true.
func TestJsonDetector(t *testing.T) {
	jsonDetector := NewJSONDetector()
	testCases := []struct {
		rawMessage     string
		expectedLabel  Label
		expectedResult bool
	}{
		{`{"key": "value"}`, noAggregate, false},
		{`    {"key": "value"}`, noAggregate, false},
		{`    { "key": "value"}`, noAggregate, false},
		{`    {."key": "value"}`, aggregate, true},
		{`.{"key": "value"}`, aggregate, true},
		{`{"another_key": "another_value"}`, noAggregate, false},
		{`{"key": 12345}`, noAggregate, false},
		{`{"array": [1,2,3]}`, noAggregate, false},
		{`not json`, aggregate, true},
		{`{foo}`, aggregate, true},
		{`{bar"}`, aggregate, true},
		{`"FOO"}`, aggregate, true},
		{`{}`, noAggregate, false},
		{` {}`, noAggregate, false},
		{` {    }`, noAggregate, false},
		{`{    }`, noAggregate, false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.rawMessage), func(t *testing.T) {
			messageContext := &messageContext{
				rawMessage:      []byte(tc.rawMessage),
				label:           aggregate,
				labelAssignedBy: defaultLabelSource,
			}
			assert.Equal(t, tc.expectedResult, jsonDetector.ProcessAndContinue(messageContext))
			assert.Equal(t, tc.expectedLabel, messageContext.label)
		})
	}
}

// TestJsonDetectorDoesntOverrideAssignedLabel anchors:
//
//	surface JSONDetection (json_detector.allium)
//	    @guidance case 1 — if context.label_assigned_by indicates
//	                       that a prior heuristic has already
//	                       claimed the label, JSONDetector takes
//	                       no action: it returns true and yields
//
// Even when the raw message matches the JSON shape, a non-default
// label_assigned_by causes JSONDetector to defer: the label is
// preserved, the return value is true (chain proceeds).
func TestJsonDetectorDoesntOverrideAssignedLabel(t *testing.T) {
	jsonDetector := NewJSONDetector()
	messageContext := &messageContext{
		rawMessage:      []byte(`{"key": "value"}`),
		label:           aggregate,
		labelAssignedBy: "Not default!",
	}
	assert.Equal(t, true, jsonDetector.ProcessAndContinue(messageContext))
	assert.Equal(t, aggregate, messageContext.label)
}

// TestJSONDetector_InputImmutability anchors:
//
//	surface JSONDetection (json_detector.allium)
//	    @guarantee InputImmutability — JSONDetector reads
//	                                    context.raw_message but
//	                                    never modifies it. It does
//	                                    not read or modify
//	                                    context.tokens or
//	                                    context.token_indices.
//
// After a call to ProcessAndContinue, raw_message bytes, tokens,
// and token_indices are byte-equal to their pre-call state — on
// both the claim path and the no-claim path.
func TestJSONDetector_InputImmutability(t *testing.T) {
	cases := []struct {
		name       string
		rawMessage string
	}{
		{"shape match (claim)", `{"key": "value"}`},
		{"no shape match (pass)", `not json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rawBefore := []byte(tc.rawMessage)
			rawSnapshot := append([]byte(nil), rawBefore...)
			tokensBefore := []Token{D4, Space, C5}
			tokensSnapshot := append([]Token(nil), tokensBefore...)
			indicesBefore := []int{0, 4, 5}
			indicesSnapshot := append([]int(nil), indicesBefore...)

			ctx := &messageContext{
				rawMessage:      rawBefore,
				tokens:          tokensBefore,
				tokenIndicies:   indicesBefore,
				label:           aggregate,
				labelAssignedBy: defaultLabelSource,
			}
			NewJSONDetector().ProcessAndContinue(ctx)

			assert.Equal(t, rawSnapshot, rawBefore, "raw_message bytes must not be mutated")
			assert.Equal(t, tokensSnapshot, tokensBefore, "tokens must not be mutated")
			assert.Equal(t, indicesSnapshot, indicesBefore, "token_indices must not be mutated")
		})
	}
}

// TestJSONDetector_LabelDomain_OnClaim anchors:
//
//	surface JSONDetection (json_detector.allium)
//	    @guarantee LabelDomain — when JSONDetector claims the
//	                              label, it sets context.label to
//	                              no_aggregate
//
// Pins the specific Label value JSONDetector emits on a claim.
// JSONDetector NEVER emits start_group or aggregate as a claim
// outcome — only no_aggregate.
func TestJSONDetector_LabelDomain_OnClaim(t *testing.T) {
	ctx := &messageContext{
		rawMessage:      []byte(`{"k": "v"}`),
		label:           aggregate,
		labelAssignedBy: defaultLabelSource,
	}
	NewJSONDetector().ProcessAndContinue(ctx)
	assert.Equal(t, noAggregate, ctx.label)
}

// TestJSONDetector_LabelAssignedByConsistency_OnClaim anchors:
//
//	surface JSONDetection (json_detector.allium)
//	    @guarantee LabelAssignedByConsistency — when JSONDetector
//	                                             sets context.label,
//	                                             it also sets
//	                                             context.label_assigned_by
//	                                             to its assigner_id
//
// On a claim, label and label_assigned_by move together: the
// assigner_id observable downstream is the JSONDetector's own
// provenance tag, not the "default" sentinel and not some other
// heuristic's id.
func TestJSONDetector_LabelAssignedByConsistency_OnClaim(t *testing.T) {
	ctx := &messageContext{
		rawMessage:      []byte(`{"k": "v"}`),
		label:           aggregate,
		labelAssignedBy: defaultLabelSource,
	}
	NewJSONDetector().ProcessAndContinue(ctx)
	assert.NotEqual(t, defaultLabelSource, ctx.labelAssignedBy,
		"label_assigned_by must move off the default sentinel when JSONDetector claims")
	assert.NotEmpty(t, ctx.labelAssignedBy, "label_assigned_by must be a non-empty assigner_id")
}

// TestJSONDetector_LabelAssignedByConsistency_OnNoClaim anchors:
//
//	surface JSONDetection (json_detector.allium)
//	    @guarantee LabelAssignedByConsistency — when JSONDetector
//	                                             does NOT set
//	                                             context.label, it
//	                                             leaves
//	                                             context.label_assigned_by
//	                                             unchanged
//
// On the no-claim paths (both case 1 — prior claim — and case 3
// — no shape match), JSONDetector must not modify
// label_assigned_by. This is what allows downstream consumers to
// trust the provenance tag as identifying the heuristic that
// actually decided the label.
func TestJSONDetector_LabelAssignedByConsistency_OnNoClaim(t *testing.T) {
	cases := []struct {
		name            string
		rawMessage      string
		labelAssignedBy string
	}{
		{"case 1 — prior claim", `{"k": "v"}`, "other_heuristic"},
		{"case 3 — no shape match", `not json`, defaultLabelSource},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &messageContext{
				rawMessage:      []byte(tc.rawMessage),
				label:           aggregate,
				labelAssignedBy: tc.labelAssignedBy,
			}
			NewJSONDetector().ProcessAndContinue(ctx)
			assert.Equal(t, tc.labelAssignedBy, ctx.labelAssignedBy,
				"label_assigned_by must be unchanged when JSONDetector does not claim")
		})
	}
}

// TestJSONDetector_TerminationSemantics anchors:
//
//	surface JSONDetection (json_detector.allium)
//	    @guarantee TerminationSemantics — process_and_continue
//	                                       returns false when
//	                                       JSONDetector claims the
//	                                       label, terminating the
//	                                       labelling chain. It
//	                                       returns true when
//	                                       JSONDetector does NOT
//	                                       claim the label.
//
// Pins the return-value pairing as a named anchor. Both halves
// are exercised: claim (case 2) → false; no claim (cases 1 and 3)
// → true.
func TestJSONDetector_TerminationSemantics(t *testing.T) {
	cases := []struct {
		name            string
		rawMessage      string
		labelAssignedBy string
		expectClaim     bool
	}{
		{"case 1 — defers, returns true", `{"k": "v"}`, "other_heuristic", false},
		{"case 2 — claims, returns false", `{"k": "v"}`, defaultLabelSource, true},
		{"case 3 — no match, returns true", `not json`, defaultLabelSource, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &messageContext{
				rawMessage:      []byte(tc.rawMessage),
				label:           aggregate,
				labelAssignedBy: tc.labelAssignedBy,
			}
			result := NewJSONDetector().ProcessAndContinue(ctx)
			// claim → false (terminate); no claim → true (continue)
			assert.Equal(t, !tc.expectClaim, result)
		})
	}
}
