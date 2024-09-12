// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

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

func TestCorrectLabelIsAssigned(t *testing.T) {
	tokenizer := NewTokenizer(config.Datadog().GetInt("logs_config.auto_multi_line.tokenizer_max_input_bytes"))
	timestampDetector := NewTimestampDetector(config.Datadog().GetFloat64("logs_config.auto_multi_line.timestamp_detector_match_threshold"))

	for _, testInput := range inputs {
		context := &messageContext{
			rawMessage: []byte(testInput.input),
			label:      aggregate,
		}

		assert.True(t, tokenizer.ProcessAndContinue(context))
		assert.True(t, timestampDetector.ProcessAndContinue(context))
		match := timestampDetector.tokenGraph.MatchProbability(context.tokens)
		assert.Equal(t, testInput.label, context.label, fmt.Sprintf("input: %s had the wrong label with probability: %f", testInput.input, match.probability))

		// To assist with debugging and tuning - this prints the probability and an underline of where the input was matched
		printMatchUnderline(context, testInput.input, match)
	}
}

func printMatchUnderline(context *messageContext, input string, match MatchContext) {
	maxLen := config.Datadog().GetInt("logs_config.auto_multi_line.tokenizer_max_input_bytes")
	fmt.Printf("%.2f\t\t%v\n", match.probability, input)

	if match.start == match.end {
		return
	}

	evalStr := input
	if len(input) > maxLen {
		evalStr = input[:maxLen]
	}
	dbgStr := ""
	printChar := " "
	last := context.tokenIndicies[0]
	for i, idx := range context.tokenIndicies {
		dbgStr += strings.Repeat(printChar, idx-last)
		if i == match.start {
			printChar = "^"
		}
		if i == match.end+1 {
			printChar = " "
		}
		last = idx
	}
	dbgStr += strings.Repeat(printChar, len(evalStr)-last)
	fmt.Printf("\t\t\t%v\n", dbgStr)
}
