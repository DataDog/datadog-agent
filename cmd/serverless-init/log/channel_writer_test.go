// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
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
