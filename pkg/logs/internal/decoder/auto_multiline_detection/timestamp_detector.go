// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// knownTimestampFormats is a list of known timestamp formats used to build the TokenGraph.
// Adding similar or partial duplicate timestamps does not impact accuracy since related
// tokens are inherently deduped in the graph. Be sure to test the accuracy of the heuristic
// after adding new formtas.
var knownTimestampFormats = []string{
	"2024-03-28T13:45:30.123456Z",
	"28/Mar/2024:13:45:30",
	"Sun, 28 Mar 2024 13:45:30",
	"2024-03-28 13:45:30",
	"2024-03-28 13:45:30,123",
	"02 Jan 06 15:04 MST",
	"2024-03-28T14:33:53.743350Z",
	"2024-03-28T15:19:38.578639+00:00",
	"2024-03-28 15:44:53",
	"2024-08-20'T'13:20:10*633+0000",
	"2024 Mar 03 05:12:41.211 PDT",
	"Jan 21 18:20:11 +0000 2024",
	"19/Apr/2024:06:36:15",
	"Dec 2, 2024 2:39:58 AM",
	"Jun 09 2024 15:28:14",
	"Apr 20 00:00:35 2010",
	"Sep 28 19:00:00 +0000",
	"Mar 16 08:12:04",
	"Jul 1 09:00:55",
	"2024-10-14T22:11:20+0000",
	"2024-07-01T14:59:55.711'+0000'",
	"2024-07-01T14:59:55.711Z",
	"2024-08-19 12:17:55-0400",
	"2024-06-26 02:31:29,573",
	"2024/04/12*19:37:50",
	"2024 Apr 13 22:08:13.211*PDT",
	"2024 Mar 10 01:44:20.392",
	"2024-03-10 14:30:12,655+0000",
	"2024-02-27 15:35:20.311",
	"2024-07-22'T'16:28:55.444",
	"2024-11-22'T'10:10:15.455",
	"2024-02-11'T'18:31:44",
	"2024-10-30*02:47:33:899",
	"2024-07-04*13:23:55",
	"24-02-11 16:47:35,985 +0000",
	"24-06-26 02:31:29,573",
	"24-04-19 12:00:17",
	"06/01/24 04:11:05",
	"08/10/24*13:33:56",
	"11/24/2024*05:13:11",
	"05/09/2024*08:22:14*612",
	"04/23/24 04:34:22 +0000",
	"2024/04/25 14:57:42",
	"11:42:35.173",
	"11:42:35,173",
	"23/Apr 11:42:35,173",
	"23/Apr/2024:11:42:35",
	"23/Apr/2024 11:42:35",
	"23-Apr-2024 11:42:35",
	"23-Apr-2024 11:42:35.883",
	"23 Apr 2024 11:42:35",
	"23 Apr 2024 10:32:35*311",
	"8/5/2024 3:31:18 AM:234",
	"9/28/2024 2:23:15 PM",
	"2023-03.28T14-33:53-7430Z",
	"2017-05-16_13:53:08",
}

// staticTokenGraph is never mutated after construction so this is safe to share between all instances of TimestampDetector.
var staticTokenGraph = makeStaticTokenGraph()

// minimumTokenLength is the minimum number of tokens needed to evaluate a timestamp probability.
// This is not configurable because it has a large impact of the relative accuracy of the heuristic.
// For example, a string 12:30:2017 is tokenized to 5 tokens DD:DD:DDDD which can easily be confused
// with other non timestamp string. Enforcing more tokens to determine a likely timetamp decreases
// the likelihood of a false positive. 8 was chosen by iterative testing using the tests in timestamp_detector_test.go.
var minimumTokenLength = 8

func makeStaticTokenGraph() *TokenGraph {
	tokenizer := NewTokenizer(100) // 100 is arbitrary, anything larger than the longest knownTimestampFormat is fine.
	inputData := make([][]tokens.Token, len(knownTimestampFormats))
	for i, format := range knownTimestampFormats {
		tokens, _ := tokenizer.tokenize([]byte(format))
		inputData[i] = tokens
	}
	return NewTokenGraph(minimumTokenLength, inputData)
}

// TimestampDetector is a heuristic to detect timestamps.
type TimestampDetector struct {
	tokenGraph     *TokenGraph
	matchThreshold float64
}

// NewTimestampDetector returns a new Timestamp detection heuristic.
func NewTimestampDetector(matchThreshold float64) *TimestampDetector {
	return &TimestampDetector{
		tokenGraph:     staticTokenGraph,
		matchThreshold: matchThreshold,
	}
}

// Process checks if a message is likely to be a timestamp.
func (t *TimestampDetector) Process(context *messageContext) bool {
	if context.tokens == nil {
		log.Error("Tokens are required to detect timestamps")
		return true
	}

	if t.tokenGraph.MatchProbability(context.tokens).probability > t.matchThreshold {
		context.label = startGroup
	}

	return true
}
