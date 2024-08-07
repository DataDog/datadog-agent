// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"fmt"
	"testing"
)

type testInput struct {
	label Label
	input string
}

func TestAccuracy(t *testing.T) {
	tokenizer := NewTokenizer(40)

	inputs := []testInput{
		{startGroup, "2021-03-28 13:45:30 App started successfully"},
		{aggregate, " .  a at some log"},
		{startGroup, "13:45:30 2021-03-28 "},
		{aggregate, "abc this 13:45:30  is a log "},
		{aggregate, "abc this 13 45:30  is a log "},
	}

	for _, testInput := range inputs {
		input := testInput.input
		aboveThreshold := testInput.aboveThreshold
		if len(input) > 40 {
			input = input[:40]
		}
		p := staticTokenGraph.MatchProbability(tokenizer.tokenize([]byte(input)))

		// Uncomment to dump table
		// fmt.Printf("%.2f\t\t\t\t%v\n", p, input)

		if aboveThreshold && p <= 0.5 {
			t.Errorf("Expected above threshold but got %.2f for input: %v", p, input)
		}
		if !aboveThreshold && p > 0.5 {
			t.Errorf("Expected below threshold but got %.2f for input: %v", p, input)
		}
	}

	test := func(input string) {
		if len(input) > 40 {
			input = input[:40]
		}
		p := staticTokenGraph.MatchProbability(tokenizer.tokenize([]byte(input)))
		fmt.Printf("%.2f\t\t\t\t%v\n", p, input)
		// assert.Greater(t, p, 0.5)
	}

	test("2021-03-28 13:45:30 App started successfully")
	test(" .  a at some log")
	test("13:45:30 2021-03-28 ")
	test("abc this 13:45:30  is a log ")
	test("abc this 13 45:30  is a log ")
	test("12:30:2017 - info App started successfully")
	test("12:30:20 - info App started successfully")
	test("2023-03.28T14-33:53-7430Z App started successfully")
	test(" [java] 1234-12-12")
	test("      at system.com.blah")
	test("Info - this is an info message App started successfully")
	test("2023-03-28T14:33:53.743350Z App started successfully")
	test("2023-03-27 12:34:56 INFO App started successfully")
	test("[2023-03-27 12:34:56] [INFO] App started successfully")
	test("[INFO] App started successfully")
	test("[INFO] test.swift:123 App started successfully")
	test("ERROR in | myFile.go:53:123 App started successfully")
	test("9/28/2022 2:23:15 PM")
	test("2024-05-15 17:04:12,369 - root - DEBUG -")
	test("[2024-05-15T18:03:23.501Z] Info : All routes applied.")
	test("2024-05-15 14:03:13 EDT | CORE | INFO | (pkg/logs/tailers/file/tailer.go:353 in forwardMessages) | ")
	test("20171223-22:15:29:606|Step_LSC|30002312|onStandStepChanged 3579")
	test("Jun 14 15:16:01 combo sshd(pam_unix)[19939]: authentication failure; logname= uid=0 euid=0 tty=NODEVssh ruser= rhost=123.456.2.4 ")
	test("Jul  1 09:00:55 calvisitor-10-105-160-95 kernel[0]: IOThunderboltSwitch<0>(0x0)::listenerCallback -")
	test("nova-api.log.1.2017-05-16_13:53:08 2017-05-16 00:00:00.008 25746 INFO nova.osapi")
	test("a2de9888a8f1fc289547f77d4834e66 - - -] 10.11.10.1 ")
	test("[Sun Dec 04 04:47:44 2005] [notice] workerEnv.init() ok /etc/httpd/conf/workers2.properties")
	test("2024/05/16 14:47:42 Datadog Tracer v1.64")
	test("2024/05/16 19:46:15 Datadog Tracer v1.64.0-rc.1 ")
	test("127.0.0.1 - - [16/May/2024:19:49:17 +0000]")
	test("127.0.0.1 - - [17/May/2024:13:51:52 +0000] \"GET /probe?debug=1 HTTP/1.1\" 200 0	")
	test("'/conf.d/..data/foobar_lifecycle.yaml' ")
	test("commit: a2de9888a8f1fc289547f77d4834e669bf993e7e")
	test(" auth.handler: auth handler stopped")
}
