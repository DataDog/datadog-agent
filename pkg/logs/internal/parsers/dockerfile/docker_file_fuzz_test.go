// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dockerfile

import (
	"encoding/json"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func FuzzParseDockerFile(f *testing.F) {
	// Seed corpus
	f.Add([]byte(`{"log":"hello world\n","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}`))
	f.Add([]byte(`{"log":"error message","stream":"stderr","time":"2019-06-06T16:35:55.930852911Z"}`))
	f.Add([]byte(`{"log":"","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}`))
	f.Add([]byte(`{"log":"\n","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}`))
	f.Add([]byte(`{"log":"\n\n","stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}`))

	// Invalid JSON that triggered the bug
	f.Add([]byte(`null`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`[]`))
	f.Add([]byte(`"string"`))
	f.Add([]byte(`123`))
	f.Add([]byte(`true`))
	f.Add([]byte(`false`))

	// Malformed JSON
	f.Add([]byte(`{`))
	f.Add([]byte(`{"log":"unclosed`))
	f.Add([]byte(``))

	// Type confusion
	f.Add([]byte(`{"log":123,"stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}`))
	f.Add([]byte(`{"log":null,"stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}`))
	f.Add([]byte(`{"log":["array"],"stream":"stdout","time":"2019-06-06T16:35:55.930852911Z"}`))

	parser := New()

	f.Fuzz(func(t *testing.T, data []byte) {
		msg := message.NewMessage(data, nil, "", 0)
		originalContent := msg.GetContent()

		// Parser should not panic
		result, err := parser.Parse(msg)

		// Critical invariants
		if result != msg {
			t.Fatalf("Parser returned different message object")
		}

		if err != nil {
			// On error: status should be Info and content should be preserved
			if result.Status != message.StatusInfo {
				t.Errorf("Failed parse should have Info status, got %s", result.Status)
			}
			if string(result.GetContent()) != string(originalContent) {
				t.Errorf("Failed parse should preserve original content")
			}
		} else {
			// On success: status should be Info, Error, or empty
			switch result.Status {
			case message.StatusInfo, message.StatusError, "":
				// valid
			default:
				t.Errorf("Unexpected status: %s", result.Status)
			}

			// Parse JSON to verify behavior matches what we parsed
			var parsed logLine
			if json.Unmarshal(originalContent, &parsed) == nil {
				// Stream mapping
				if parsed.Stream == "stderr" && result.Status != message.StatusError {
					t.Errorf("stderr stream should map to StatusError, got %s", result.Status)
				} else if parsed.Stream == "stdout" && result.Status != message.StatusInfo {
					t.Errorf("stdout stream should map to StatusInfo, got %s", result.Status)
				}

				// Timestamp should match
				if result.ParsingExtra.Timestamp != parsed.Time {
					t.Errorf("Timestamp mismatch: got %q, expected %q", result.ParsingExtra.Timestamp, parsed.Time)
				}

				// Newline and IsPartial handling
				if len(parsed.Log) > 0 && parsed.Log[len(parsed.Log)-1] == '\n' {
					// Should NOT be partial
					if result.ParsingExtra.IsPartial {
						t.Errorf("Log ending with newline should not be partial")
					}
					// Content should be stripped by exactly one newline
					expected := parsed.Log[:len(parsed.Log)-1]
					if string(result.GetContent()) != expected {
						t.Errorf("Content mismatch: got %q, expected %q", string(result.GetContent()), expected)
					}
				} else if len(parsed.Log) > 0 {
					// Should be partial
					if !result.ParsingExtra.IsPartial {
						t.Errorf("Log not ending with newline should be partial")
					}
					// Content should match exactly
					if string(result.GetContent()) != parsed.Log {
						t.Errorf("Content mismatch: got %q, expected %q", string(result.GetContent()), parsed.Log)
					}
				} else {
					// Empty log - should not be partial
					if result.ParsingExtra.IsPartial {
						t.Errorf("Empty log should not be partial")
					}
				}
			}
		}
	})
}
