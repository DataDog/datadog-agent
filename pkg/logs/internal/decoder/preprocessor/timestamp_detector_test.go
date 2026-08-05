// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package preprocessor contains auto multiline detection and aggregation logic.
package preprocessor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// Anchoring unit tests for the TimestampDetection surface declared
// in timestamp_detector.allium. Each test names the spec construct
// (@guarantee or @guidance case) it anchors so that drift in either
// direction is easy to spot during review.
//
// Property tests for the same surface live in
// timestamp_detector_proptest_test.go.

type testInput struct {
	label Label
	input string
}

// Use this dataset to improve the accuracy of the timestamp detector.
// Ideally logs with the start group label should be very close to a 1.0 match probability
// and logs with the aggregate label should be very close to a 0.0 match probability.
var inputs = []testInput{
	// Likely contain timestamps for aggregation
	{startGroup, "2021-03-28 13:45:30 App started successfully"},
	{startGroup, "13:45:30 2021-03-28"},
	{startGroup, "foo bar 13:45:30 2021-03-28"},
	{startGroup, "2023-03-28T14:33:53.743350Z App started successfully"},
	{startGroup, "2023-03-27 12:34:56 INFO App started successfully"},
	{startGroup, "2023-03.28T14-33:53-7430Z App started successfully"},
	{startGroup, "Datadog Agent 2023-03.28T14-33:53-7430Z App started successfully"},
	{startGroup, "[2023-03-27 12:34:56] [INFO] App started successfully"},
	{startGroup, "9/28/2022 2:23:15 PM"},
	{startGroup, "2024-05-15 17:04:12,369 - root - DEBUG -"},
	{startGroup, "[2024-05-15T18:03:23.501Z] Info : All routes applied."},
	{startGroup, "2024-05-15 14:03:13 EDT | CORE | INFO | (pkg/logs/tailers/file/tailer.go:353 in forwardMessages) | "},
	{startGroup, "Jun 14 15:16:01 combo sshd(pam_unix)[19939]: authentication failure; logname= uid=0 euid=0 tty=NODEVssh ruser= rhost=123.456.2.4 "},
	{startGroup, "Jul  1 09:00:55 calvisitor-10-105-160-95 kernel[0]: IOThunderboltSwitch<0>(0x0)::listenerCallback -"},
	{startGroup, "[Sun Dec 04 04:47:44 2005] [notice] workerEnv.init() ok /etc/httpd/conf/workers2.properties"},
	{startGroup, "2024/05/16 14:47:42 Datadog Tracer v1.64"},
	{startGroup, "2024/05/16 19:46:15 Datadog Tracer v1.64.0-rc.1 "},
	{startGroup, "127.0.0.1 - - [16/May/2024:19:49:17 +0000]"},
	{startGroup, "127.0.0.1 - - [17/May/2024:13:51:52 +0000] \"GET /probe?debug=1 HTTP/1.1\" 200 0	"},
	{startGroup, "nova-api.log.1.2017-05-16_13:53:08 2017-05-16 00:00:00.008 25746 INFO nova.osapi"},
	{startGroup, "foo bar log 2024-11-22'T'10:10:15.455 my log line"},
	{startGroup, "foo bar log 2024-07-01T14:59:55.711'+0000' my log line"},

	// A case where the timestamp has a non-matching token in the midddle of it.
	{startGroup, "acb def 10:10:10 foo 2024-05-15 hijk lmop"},

	// Likely do not contain timestamps for aggreagtion
	{aggregate, "12:30:2017 - info App started successfully"},
	{aggregate, "12:30:20 - info App started successfully"},
	{aggregate, "20171223-22:15:29:606|Step_LSC|30002312|onStandStepChanged 3579"},
	{aggregate, " .  a at some log"},
	{aggregate, "abc this 13:45:30  is a log "},
	{aggregate, "abc this 13 45:30  is a log "},
	{aggregate, " [java] 1234-12-12"},
	{aggregate, "      at system.com.blah"},
	{aggregate, "Info - this is an info message App started successfully"},
	{aggregate, "[INFO] App started successfully"},
	{aggregate, "[INFO] test.swift:123 App started successfully"},
	{aggregate, "ERROR in | myFile.go:53:123 App started successfully"},
	{aggregate, "a2de9888a8f1fc289547f77d4834e66 - - -] 10.11.10.1 "},
	{aggregate, "'/conf.d/..data/foobar_lifecycle.yaml' "},
	{aggregate, "commit: a2de9888a8f1fc289547f77d4834e669bf993e7e"},
	{aggregate, " auth.handler: auth handler stopped"},
	{aggregate, "10:10:10 foo :10: bar 10:10"},
	{aggregate, "1234-1234-1234-123-21-1"},
	{aggregate, " = '10.20.30.123' (DEBUG)"},
	{aggregate, "192.168.1.123"},
	{aggregate, "'192.168.1.123'"},
	{aggregate, "10.0.0.123"},
	{aggregate, "\"10.0.0.123\""},
	{aggregate, "2001:0db8:85a3:0000:0000:8a2e:0370:7334"},
	{aggregate, "fd12:3456:789a:1::1"},
	{aggregate, "2001:db8:0:1234::5678"},
}

// TestCorrectLabelIsAssigned anchors:
//
//	surface TimestampDetection (timestamp_detector.allium)
//	    @guidance case 2 — shape match: claim by setting label to
//	                       start_group and label_assigned_by to
//	                       assigner_id
//	    @guidance case 3 — no shape match: take no action
//
// The corpus mixes positive cases (start_group, expected to match
// the static token-shape model with probability > threshold) and
// negative cases (aggregate, expected to fall below threshold). A
// regression in either direction — a positive case dropping below
// threshold or a negative case rising above it — fails this test.
//
// The corpus also doubles as the dataset for tuning the detector:
// keeping the test green is the calibration constraint when adding
// new timestamp formats to knownTimestampFormats or adjusting the
// match threshold default.
func TestCorrectLabelIsAssigned(t *testing.T) {
	mockConfig := configmock.New(t)
	tokenizer := NewTokenizer(mockConfig.GetInt("logs_config.auto_multi_line.tokenizer_max_input_bytes"))
	timestampDetector := NewTimestampDetector(mockConfig.GetFloat64("logs_config.auto_multi_line.timestamp_detector_match_threshold"))

	for _, testInput := range inputs {
		context := &messageContext{
			rawMessage: []byte(testInput.input),
			label:      aggregate,
		}

		context.tokens, context.tokenIndicies = tokenizer.Tokenize(context.rawMessage)
		assert.True(t, timestampDetector.ProcessAndContinue(context))
		match := timestampDetector.tokenGraph.MatchProbability(context.tokens)
		assert.Equal(t, testInput.label, context.label, fmt.Sprintf("input: %s had the wrong label with probability: %f", testInput.input, match.probability))

		// To assist with debugging and tuning - this prints the probability and an underline of where the input was matched
		printMatchUnderline(t, context, testInput.input, match)
	}
}

func printMatchUnderline(t *testing.T, context *messageContext, input string, match MatchContext) {
	mockConfig := configmock.New(t)
	maxLen := mockConfig.GetInt("logs_config.auto_multi_line.tokenizer_max_input_bytes")
	fmt.Printf("%.2f\t\t%v\n", match.probability, input)

	if match.start == match.end {
		return
	}

	evalStr := input
	if len(input) > maxLen {
		evalStr = input[:maxLen]
	}
	var dbgBuilder strings.Builder
	printChar := " "
	last := context.tokenIndicies[0]
	for i, idx := range context.tokenIndicies {
		dbgBuilder.WriteString(strings.Repeat(printChar, idx-last))
		if i == match.start {
			printChar = "^"
		}
		if i == match.end+1 {
			printChar = " "
		}
		last = idx
	}
	dbgBuilder.WriteString(strings.Repeat(printChar, len(evalStr)-last))
	fmt.Printf("\t\t\t%v\n", dbgBuilder.String())
}

// timestampDetectorForTests constructs a TimestampDetector with the
// default match threshold from config. Used by the unit tests below
// so the construction details don't repeat in every test.
func timestampDetectorForTests(t *testing.T) *TimestampDetector {
	t.Helper()
	mockConfig := configmock.New(t)
	return NewTimestampDetector(mockConfig.GetFloat64("logs_config.auto_multi_line.timestamp_detector_match_threshold"))
}

// tokenizerForTests constructs a Tokenizer using the default
// max-input-bytes config, matching the production wiring.
func tokenizerForTests(t *testing.T) *Tokenizer {
	t.Helper()
	mockConfig := configmock.New(t)
	return NewTokenizer(mockConfig.GetInt("logs_config.auto_multi_line.tokenizer_max_input_bytes"))
}

// TestTimestampDetector_EmptyTokensNoAction anchors:
//
//	surface TimestampDetection (timestamp_detector.allium)
//	    @guidance case 1 — If context.tokens is empty,
//	                       TimestampDetector takes no action: it
//	                       returns true without inspecting or
//	                       modifying context. The detector cannot
//	                       evaluate a timestamp shape without
//	                       tokens to inspect.
//
// Exercises both the nil-slice path (the Go impl logs an error and
// returns) and the empty-but-non-nil-slice path.
func TestTimestampDetector_EmptyTokensNoAction(t *testing.T) {
	cases := []struct {
		name   string
		tokens []Token
	}{
		{"nil tokens", nil},
		{"empty tokens", []Token{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &messageContext{
				rawMessage:      []byte("2024-03-28T13:45:30.123456Z any content"),
				tokens:          tc.tokens,
				label:           aggregate,
				labelAssignedBy: defaultLabelSource,
			}
			result := timestampDetectorForTests(t).ProcessAndContinue(ctx)
			assert.True(t, result, "TimestampDetector must always return true")
			assert.Equal(t, aggregate, ctx.label, "label must be unchanged when no tokens are present")
			assert.Equal(t, defaultLabelSource, ctx.labelAssignedBy, "label_assigned_by must be unchanged when no tokens are present")
		})
	}
}

// TestTimestampDetector_LabelDomain_OnClaim anchors:
//
//	surface TimestampDetection (timestamp_detector.allium)
//	    @guarantee LabelDomain — when TimestampDetector claims the
//	                              label, it sets context.label to
//	                              start_group
//
// Pins the specific Label value emitted on a claim. The claim is
// always start_group — never no_aggregate or aggregate.
func TestTimestampDetector_LabelDomain_OnClaim(t *testing.T) {
	// A canonical timestamp format from the knownTimestampFormats
	// corpus; high match probability ensures the detector claims.
	content := []byte("2024-03-28T13:45:30.123456Z some log line content")
	tok := tokenizerForTests(t)
	tokens, indices := tok.Tokenize(content)
	ctx := &messageContext{
		rawMessage:      content,
		tokens:          tokens,
		tokenIndicies:   indices,
		label:           aggregate,
		labelAssignedBy: defaultLabelSource,
	}
	timestampDetectorForTests(t).ProcessAndContinue(ctx)
	assert.Equal(t, startGroup, ctx.label)
}

// TestTimestampDetector_LabelAssignedByConsistency_OnClaim anchors:
//
//	surface TimestampDetection (timestamp_detector.allium)
//	    @guarantee LabelAssignedByConsistency — when
//	                                             TimestampDetector
//	                                             sets context.label,
//	                                             it also sets
//	                                             context.label_assigned_by
//	                                             to its assigner_id
//
// On a claim, label and label_assigned_by move together: the
// assigner_id observable downstream is the TimestampDetector's own
// provenance tag, not the "default" sentinel.
func TestTimestampDetector_LabelAssignedByConsistency_OnClaim(t *testing.T) {
	content := []byte("2024-03-28T13:45:30.123456Z some log line content")
	tok := tokenizerForTests(t)
	tokens, indices := tok.Tokenize(content)
	ctx := &messageContext{
		rawMessage:      content,
		tokens:          tokens,
		tokenIndicies:   indices,
		label:           aggregate,
		labelAssignedBy: defaultLabelSource,
	}
	timestampDetectorForTests(t).ProcessAndContinue(ctx)
	assert.NotEqual(t, defaultLabelSource, ctx.labelAssignedBy,
		"label_assigned_by must move off the default sentinel when TimestampDetector claims")
	assert.NotEmpty(t, ctx.labelAssignedBy, "label_assigned_by must be a non-empty assigner_id")
}

// TestTimestampDetector_LabelAssignedByConsistency_OnNoClaim anchors:
//
//	surface TimestampDetection (timestamp_detector.allium)
//	    @guarantee LabelAssignedByConsistency — when
//	                                             TimestampDetector
//	                                             does NOT set
//	                                             context.label, it
//	                                             leaves
//	                                             context.label_assigned_by
//	                                             unchanged
//
// On the no-claim paths (case 1 — empty tokens — and case 3 — no
// shape match), TimestampDetector must not modify
// label_assigned_by. This is what allows downstream consumers to
// trust the provenance tag as identifying the heuristic that
// actually decided the label.
func TestTimestampDetector_LabelAssignedByConsistency_OnNoClaim(t *testing.T) {
	cases := []struct {
		name            string
		content         string
		populateTokens  bool
		labelAssignedBy string
	}{
		{"case 1 — empty tokens", "2024-03-28T13:45:30.123456Z content", false, defaultLabelSource},
		{"case 3 — no shape match", "this is just an ordinary log line without dates", true, "prior_heuristic"},
		{"case 3 — no shape match, default sentinel", "ordinary log without dates", true, defaultLabelSource},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := []byte(tc.content)
			var tokens []Token
			var indices []int
			if tc.populateTokens {
				tokens, indices = tokenizerForTests(t).Tokenize(content)
			}
			ctx := &messageContext{
				rawMessage:      content,
				tokens:          tokens,
				tokenIndicies:   indices,
				label:           aggregate,
				labelAssignedBy: tc.labelAssignedBy,
			}
			timestampDetectorForTests(t).ProcessAndContinue(ctx)
			assert.Equal(t, tc.labelAssignedBy, ctx.labelAssignedBy,
				"label_assigned_by must be unchanged when TimestampDetector does not claim")
		})
	}
}

// TestTimestampDetector_TerminationSemantics_AlwaysContinues anchors:
//
//	surface TimestampDetection (timestamp_detector.allium)
//	    @guarantee TerminationSemantics — process_and_continue
//	                                       always returns true:
//	                                       TimestampDetector is an
//	                                       advisory heuristic that
//	                                       never terminates the
//	                                       labelling chain
//
// Pins the always-true return as a named anchor. Exercised across
// all three @guidance cases (empty tokens, claim, no claim).
func TestTimestampDetector_TerminationSemantics_AlwaysContinues(t *testing.T) {
	cases := []struct {
		name            string
		content         string
		populateTokens  bool
		labelAssignedBy string
	}{
		{"case 1 — empty tokens", "2024-03-28T13:45:30.123456Z log", false, defaultLabelSource},
		{"case 2 — claim", "2024-03-28T13:45:30.123456Z log", true, defaultLabelSource},
		{"case 3 — no shape match", "ordinary log line", true, defaultLabelSource},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := []byte(tc.content)
			var tokens []Token
			var indices []int
			if tc.populateTokens {
				tokens, indices = tokenizerForTests(t).Tokenize(content)
			}
			ctx := &messageContext{
				rawMessage:      content,
				tokens:          tokens,
				tokenIndicies:   indices,
				label:           aggregate,
				labelAssignedBy: tc.labelAssignedBy,
			}
			result := timestampDetectorForTests(t).ProcessAndContinue(ctx)
			assert.True(t, result, "TimestampDetector must always return true (advisory; never terminates)")
		})
	}
}

// TestTimestampDetector_InputImmutability anchors:
//
//	surface TimestampDetection (timestamp_detector.allium)
//	    @guarantee InputImmutability — TimestampDetector reads
//	                                    context.tokens but never
//	                                    modifies it. It does not
//	                                    read or modify
//	                                    context.raw_message or
//	                                    context.token_indices.
//
// After a call to ProcessAndContinue, raw_message bytes, tokens,
// and token_indices are byte-equal to their pre-call state — on
// the claim path, the no-shape-match path, and the empty-tokens
// path.
func TestTimestampDetector_InputImmutability(t *testing.T) {
	cases := []struct {
		name           string
		content        string
		populateTokens bool
	}{
		{"claim", "2024-03-28T13:45:30.123456Z log content", true},
		{"no shape match", "ordinary log line", true},
		{"empty tokens", "2024-03-28T13:45:30.123456Z log content", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			content := []byte(tc.content)
			rawSnapshot := append([]byte(nil), content...)
			var tokens []Token
			var indices []int
			if tc.populateTokens {
				tokens, indices = tokenizerForTests(t).Tokenize(content)
			}
			tokensSnapshot := append([]Token(nil), tokens...)
			indicesSnapshot := append([]int(nil), indices...)

			ctx := &messageContext{
				rawMessage:      content,
				tokens:          tokens,
				tokenIndicies:   indices,
				label:           aggregate,
				labelAssignedBy: defaultLabelSource,
			}
			timestampDetectorForTests(t).ProcessAndContinue(ctx)

			assert.Equal(t, rawSnapshot, content, "raw_message bytes must not be mutated")
			assert.Equal(t, tokensSnapshot, tokens, "tokens must not be mutated")
			assert.Equal(t, indicesSnapshot, indices, "token_indices must not be mutated")
		})
	}
}
