// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package processor

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestEncodeInner(t *testing.T) {
	tests := []struct {
		name  string
		input jsonPayload
	}{
		{
			name:  "empty payload",
			input: jsonPayload{},
		},
		{
			name: "minimal fields",
			input: jsonPayload{
				Message: "test message",
			},
		},
		{
			name: "all fields populated",
			input: jsonPayload{
				Message:   "message",
				Status:    "INFO",
				Timestamp: 100,
				Hostname:  "host",
				Service:   "service",
				Source:    "source",
				Tags:      "foo:bar,baz:bing",
			},
		},
		{
			name: "emjois in fields",
			input: jsonPayload{
				Message:   "message 🌔",
				Status:    "INFO",
				Timestamp: 100,
				Hostname:  "host🌗",
				Service:   "service🌗",
				Source:    "source🌗",
				Tags:      "foo:bar,baz:bing,🌗:🌔",
			},
		},
		{
			name: "fields with whitespace",
			input: jsonPayload{
				Message:   "    spaced-out message    ",
				Status:    "   ",
				Timestamp: 100,
				Hostname:  "host with spaces",
				Service:   " service ",
				Source:    " source ",
				Tags:      "  foo:bar , baz : bing ",
			},
		},
		{
			name: "escape sequences in message",
			input: jsonPayload{
				Message:   "Line1\nLine2\n\tIndented \"quoted\" text",
				Status:    "WARN",
				Timestamp: 100,
				Hostname:  "host",
				Service:   "service",
				Source:    "source",
				Tags:      "foo:bar,baz:bing",
			},
		},
		{
			name: "negative timestamp",
			input: jsonPayload{
				Message:   "message",
				Status:    "DEBUG",
				Timestamp: -12345,
				Hostname:  "host",
				Service:   "service",
				Source:    "source",
				Tags:      "foo:bar,baz:bing",
			},
		},
		{
			name: "multiline message",
			input: jsonPayload{
				Message: `Line1
Line2
Line3`,
				Status:    "info",
				Timestamp: 100,
				Hostname:  "host",
				Service:   "service",
				Source:    "source",
				Tags:      "foo:bar,baz:bing",
			},
		},
		{
			name: "http message",
			input: jsonPayload{
				Message:   "<p>message</p>",
				Status:    "info",
				Timestamp: 100,
				Hostname:  "host",
				Service:   "service",
				Source:    "source",
				Tags:      "foo:bar,baz:bing",
			},
		},
		{
			name: "diacritics",
			input: jsonPayload{
				Message:   "message",
				Status:    "Îńfø",
				Timestamp: 100,
				Hostname:  "host",
				Service:   "service",
				Source:    "source",
				Tags:      "foo:bar,baz:bing",
			},
		},
	}

	// Assert that encodeInner is the inverse of json.Unmarshal.
	for _, tt := range tests {
		tt := tt // pin
		t.Run(tt.name, func(t *testing.T) {
			encodedBytes, err := encodeInner(tt.input)
			if err != nil {
				t.Errorf("encodeInner() error = %v, input = %+v", err, tt.input)
				return
			}

			var decodedPayload jsonPayload
			if err := json.Unmarshal(encodedBytes, &decodedPayload); err != nil {
				t.Errorf("json.Unmarshal() error = %v, during round-trip: %s", err, string(encodedBytes))
				return
			}

			if !reflect.DeepEqual(tt.input, decodedPayload) {
				t.Errorf("Expected: %+v\nGot: %+v", tt.input, decodedPayload)
			}
		})
	}
}
