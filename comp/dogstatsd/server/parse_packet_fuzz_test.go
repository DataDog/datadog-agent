// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"testing"
)

// FuzzNextMessage fuzzes the packet → message splitting logic that extracts
// individual DogStatsD messages from raw UDP/UDS packets. A real packet may
// contain multiple newline-separated messages of mixed types. This is the
// first function to process raw untrusted bytes from the network.
func FuzzNextMessage(f *testing.F) {
	// Single messages
	f.Add([]byte("custom_counter:1|c\n"), true)
	f.Add([]byte("custom_counter:1|c"), false) // no trailing newline

	// Multi-message packets (the real untested case)
	f.Add([]byte("metric.a:1|c\nmetric.b:2|g\n"), true)
	f.Add([]byte("metric:1|c\n_e{5,4}:title|text\n_sc|name|0\n"), true)
	f.Add([]byte("metric:1|c\n_e{5,4}:title|text\n_sc|name|0"), false)

	// Edge cases in line splitting
	f.Add([]byte("metric:1|c\r\n"), true)             // CRLF
	f.Add([]byte("\n\n\n"), true)                     // empty lines
	f.Add([]byte("metric:1|c\n\nmetric:2|g\n"), true) // empty line between messages
	f.Add([]byte(""), true)                           // empty packet
	f.Add([]byte("\n"), true)                         // just a newline
	f.Add([]byte("a\nb\nc\nd\ne\nf\ng\n"), true)      // many short messages

	// EOL termination behavior
	f.Add([]byte("metric:1|c"), true) // no newline + eolTermination → should be dropped

	f.Fuzz(func(t *testing.T, packet []byte, eolTermination bool) {
		// Work on a copy since nextMessage mutates the slice header
		contents := make([]byte, len(packet))
		copy(contents, packet)

		totalConsumed := 0
		messageCount := 0
		const maxMessages = 100000 // safety bound

		for messageCount < maxMessages {
			msg := nextMessage(&contents, eolTermination)
			if msg == nil {
				break
			}
			messageCount++
			totalConsumed += len(msg)
		}

		// nextMessage must eventually return nil (terminate).
		if messageCount >= maxMessages {
			t.Fatal("nextMessage did not terminate within bounds")
		}

		// Total consumed bytes must not exceed input length.
		if totalConsumed > len(packet) {
			t.Fatalf("consumed %d bytes from %d byte packet", totalConsumed, len(packet))
		}
	})
}

// FuzzPacketToMessages fuzzes the full packet → parse pipeline: splitting a raw
// packet into messages, detecting their types, and parsing each one. This tests
// the composition of nextMessage + findMessageType + parseMetricSample/parseEvent/
// parseServiceCheck — the actual code path for every UDP/UDS packet received.
func FuzzPacketToMessages(f *testing.F) {
	// Multi-message packets with mixed types
	f.Add([]byte("metric.a:1|c|#tag1:val1\nmetric.b:2.5|g|@0.5\n"))
	f.Add([]byte("_e{5,4}:title|text|t:warning|#tag1\n_sc|check.name|0|#tag2\n"))
	f.Add([]byte("metric:1|c\n_e{5,4}:title|text\n_sc|name|0\nmetric:2|g|#t:v\n"))

	// Extended protocol fields
	f.Add([]byte("metric:1|c|#tag|T1657100430|c:ci-abc123|e:it-true,cn-ctr,pu-uid\n"))

	// Edge cases
	f.Add([]byte(""))
	f.Add([]byte("\n"))
	f.Add([]byte("not_a_valid_message\n"))
	f.Add([]byte("just_text_no_pipe"))
	f.Add([]byte("::|\n"))      // minimal structure
	f.Add([]byte("_e{0,0}:\n")) // empty event
	f.Add([]byte("_sc||0\n"))   // empty service check name

	// Large packet with many messages
	large := make([]byte, 0, 4096)
	for i := 0; i < 100; i++ {
		large = append(large, []byte("m:1|c\n")...)
	}
	f.Add(large)

	f.Fuzz(func(t *testing.T, packet []byte) {
		// Parser must be created per-iteration because parseEvent/parseServiceCheck
		// emit log warnings, and the mock logger is bound to testing.TB.
		deps := newServerDeps(t)
		stringInternerTelemetry := newSiTelemetry(false, deps.Telemetry)
		parser := newParser(deps.Config, newFloat64ListPool(deps.Telemetry), 1, deps.WMeta, stringInternerTelemetry)

		contents := make([]byte, len(packet))
		copy(contents, packet)

		const maxMessages = 100000
		for i := 0; i < maxMessages; i++ {
			msg := nextMessage(&contents, false)
			if msg == nil {
				break
			}
			if len(msg) == 0 {
				continue
			}

			// Dispatch by type — exactly as parsePackets does
			switch findMessageType(msg) {
			case serviceCheckType:
				_, _ = parser.parseServiceCheck(msg)
			case eventType:
				_, _ = parser.parseEvent(msg)
			case metricSampleType:
				_, _ = parser.parseMetricSample(msg)
			}
		}
	})
}
