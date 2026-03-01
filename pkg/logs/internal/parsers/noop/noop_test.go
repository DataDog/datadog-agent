// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package noop

import (
	"bytes"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/logs/message"

	"github.com/stretchr/testify/assert"
)

func TestNoopParserHandleMessages(t *testing.T) {
	parser := New()
	logMessage := message.NewMessage([]byte("Foo"), nil, "", 0)
	msg, err := parser.Parse(logMessage)
	assert.Nil(t, err)
	assert.False(t, logMessage.ParsingExtra.IsPartial)
	assert.Equal(t, logMessage.GetContent(), msg.GetContent())
}

func TestNoopParserSupportsPartialLine(t *testing.T) {
	parser := New()
	assert.False(t, parser.SupportsPartialLine())
}

// FuzzNoopParser tests the noop parser with arbitrary input. Even though the
// noop parser just returns its input unchanged, we still want to ensure it
// never panics and always returns the same message object it was given.
func FuzzNoopParser(f *testing.F) {
	f.Add([]byte("Foo"))
	f.Add([]byte(""))
	f.Add([]byte(" "))
	f.Add([]byte("\x00\x01\x02\x03"))
	f.Add([]byte("\xFF\xFE\xFD\xFC"))

	parser := New()

	f.Fuzz(func(t *testing.T, data []byte) {
		msg := message.NewMessage(data, nil, "", 0)
		originalContent := msg.GetContent()

		// Parser must not panic
		result, err := parser.Parse(msg)

		// Noop parser should never return an error
		if err != nil {
			t.Fatalf("Noop parser returned error: %v", err)
		}

		// Must return the same message object
		if result != msg {
			t.Fatal("Noop parser returned different message object")
		}

		// Content must be unchanged
		if !bytes.Equal(result.GetContent(), originalContent) {
			t.Fatal("Noop parser modified content")
		}

		// IsPartial should remain false (noop doesn't support partial lines)
		if result.ParsingExtra.IsPartial {
			t.Fatal("Noop parser set IsPartial to true")
		}
	})
}
