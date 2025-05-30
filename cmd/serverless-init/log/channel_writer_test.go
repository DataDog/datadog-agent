// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"reflect"
	"strings"
	"testing"

	logConfig "github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

func TestChannelWriter_Write(t *testing.T) {
	ch := make(chan *logConfig.ChannelMessage, 10)
	cw := NewChannelWriter(ch, false)

	// Test writing without a newline
	cw.Write([]byte("test"))
	if len(ch) != 0 {
		t.Fatalf("Expected channel to be empty, but it wasn't")
	}

	// Test writing with a single newline
	cw.Write([]byte("test\n"))
	if len(ch) != 1 {
		t.Fatalf("Expected channel to have 1 message, but it has %d", len(ch))
	}
	msg := <-ch
	if string(msg.Content) != "testtest" {
		t.Fatalf("Expected message content 'testtest' but got '%s'", msg.Content)
	}

	// Test writing multiple newlines
	multilineMessage := "line1\nline2\nline3\n"
	expected := strings.TrimSpace(multilineMessage)
	cw.Write([]byte(multilineMessage))
	if len(ch) != 1 {
		t.Fatalf("Expected channel to have 1 message, but it has %d", len(ch))
	}
	msg = <-ch
	if string(msg.Content) != expected {
		t.Fatalf("Expected message content '%s' but got '%s'", expected, msg.Content)
	}

	// Test sending data without flushing with a newline
	cw.Write([]byte("partial"))
	if len(ch) != 0 {
		t.Fatalf("Expected channel to be empty after sending partial data, but it wasn't")
	}
	// Complete the message with a newline and check it's sent
	cw.Write([]byte(" data\n"))
	if len(ch) != 1 {
		t.Fatalf("Expected channel to have 1 message after completing the message, but it has %d", len(ch))
	}
	msg = <-ch
	if string(msg.Content) != "partial data" {
		t.Fatalf("Expected message content 'partial data', but got '%s'", msg.Content)
	}
}

func TestSplitJsonMessages(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"empty", "", []string{}},
		{"whitespace only", "   \n\t", []string{}},
		{"plain text", "hello world", []string{"hello world"}},
		{"plain text newline separated", "Some Error\n  at line 30\n  at line 60\n", []string{"Some Error\n  at line 30\n  at line 60"}},
		{"single JSON", `{"msg":"A"}`, []string{`{"msg":"A"}`}},
		{"single JSON with whitespace", "  {\"msg\":\"A\"}\n", []string{`{"msg":"A"}`}},
		{"two JSON, newline separated", "  {\"msg\":\"A\"}\n{\"msg\":\"B\"}\n", []string{`{"msg":"A"}`, `{"msg":"B"}`}},
		{"two JSON, back-to-back", `{"msg":"A"}{"msg":"B"}`, []string{`{"msg":"A"}`, `{"msg":"B"}`}},
		{"escaped brace in string", `{"msg":"brace { inside"}{"msg":"B"}`, []string{`{"msg":"brace { inside"}`, `{"msg":"B"}`}},
		{"nested JSON", `{"outer":{"inner":1}}{"other":2}`, []string{`{"outer":{"inner":1}}`, `{"other":2}`}},
		{"malformed JSON", `{"msg":"A"`, []string{`{"msg":"A"`}},
		{"JSON plus tail", `{"msg":"A"} trailing`, []string{`{"msg":"A"}`, `trailing`}},
		{"plaintext then JSON", "plain\n{\"msg\":\"A\"}", []string{"plain", "{\"msg\":\"A\"}"}},
		{"complex case", "A\nB\nC\n{\"msg\": \"D\"}\n{\"msg\": \"E\"}\nF\nG", []string{"A\nB\nC", "{\"msg\": \"D\"}", "{\"msg\": \"E\"}", "F\nG"}},
		{"plaintext with invalid json", "A\nB{\"msg\": \"B\"C\nD", []string{"A\nB", "{\"msg\": \"B\"C\nD"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := splitJSONBytes([]byte(tc.input))

			// convert [][]byte into []string for comparison
			actual := make([]string, len(parts))
			for i, part := range parts {
				actual[i] = string(part)
			}

			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("splitJsonMessages(%q) returned %v; expected %v", tc.input, strings.Join(actual, ","), strings.Join(tc.expected, ","))
			}
		})
	}
}
