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

	// Test writing with a single newline
	cw.Write([]byte("test\n"))
	if len(ch) != 1 {
		t.Fatalf("Expected channel to have 1 message, but it has %d", len(ch))
	}
	msg := <-ch
	if string(msg.Content) != "test" {
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
		{"single JSON", `{"msg":"A"}`, []string{`{"msg":"A"}`}},
		{"single JSON with whitespace", "  {\"msg\":\"A\"}\n", []string{`{"msg":"A"}`}},
		{"two JSON, newline separated", "  {\"msg\":\"A\"}\n{\"msg\":\"B\"}\n", []string{`{"msg":"A"}`, `{"msg":"B"}`}},
		{"two JSON, back-to-back", `{"msg":"A"}{"msg":"B"}`, []string{`{"msg":"A"}`, `{"msg":"B"}`}},
		{"escaped brace in string", `{"msg":"brace { inside"}{"msg":"B"}`, []string{`{"msg":"brace { inside"}`, `{"msg":"B"}`}},
		{"nested JSON", `{"outer":{"inner":1}}{"other":2}`, []string{`{"outer":{"inner":1}}`, `{"other":2}`}},
		{"malformed JSON", `{"msg":"A"`, []string{`{"msg":"A"`}},
		{"JSON plus tail", `{"msg":"A"} trailing`, []string{`{"msg":"A"}`, `trailing`}},
		{"plaintext then JSON", "plain\n{\"msg\":\"A\"}", []string{"plain\n{\"msg\":\"A\"}"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := splitJsonMessages(tc.input)
			if !reflect.DeepEqual(actual, tc.expected) {
				t.Errorf("splitJsonMessages(%q) = %v; want %v", tc.input, actual, tc.expected)
			}
		})
	}
}
