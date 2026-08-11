// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package preprocessor

import "testing"

// BenchmarkMatchProbability covers the shapes MatchProbability sees in the log pipeline: a
// line rejected before it is scored, a line that is scored, and the longest input the
// default tokenizer_max_input_bytes allows.
func BenchmarkMatchProbability(b *testing.B) {
	cases := []struct {
		name  string
		input string
	}{
		{"NoTimestamp", "[INFO] App started successfully"},
		{"Timestamp", "2024-05-15 17:04:12,369 - root - DEBUG - handled the request"},
		{"TimestampFollowedByAddress", "2026-08-11 10:34:49 W3SVC1 10.1.48.10 GET /svc/v13/core/Consent 443 - 200"},
		{"MaxInputBytes", "1234567890 1234567890 1234567890 1234567890 1234567890 12345"},
	}

	tokenizer := NewTokenizer(60)
	graph := makeStaticTokenGraph()

	for _, c := range cases {
		tokens, _ := tokenizer.Tokenize([]byte(c.input))
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				graph.MatchProbability(tokens)
			}
		})
	}
}
