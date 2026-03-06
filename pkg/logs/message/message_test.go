// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessage(t *testing.T) {

	message := NewMessage([]byte("hello"), nil, "", 0)
	assert.Equal(t, "hello", string(message.GetContent()))

	message.SetContent([]byte("world"))
	assert.Equal(t, "world", string(message.GetContent()))
	assert.Equal(t, StatusInfo, message.GetStatus())
}

func TestNewPayload(t *testing.T) {
	messages := []*Message{
		NewMessage([]byte("hello"), nil, "", 0),
		NewMessage([]byte("world"), nil, "", 0),
		NewMessage([]byte("test"), nil, "", 0),
	}
	messageMetas := make([]*MessageMetadata, len(messages))
	for i, msg := range messages {
		messageMetas[i] = &msg.MessageMetadata
	}
	encoded := []byte("encoded content")
	encoding := "gzip"
	unencodedSize := 100

	payload := NewPayload(messageMetas, encoded, encoding, unencodedSize)

	// Test basic payload properties
	assert.Equal(t, 3, len(payload.MessageMetas))
	assert.Equal(t, encoded, payload.Encoded)
	assert.Equal(t, encoding, payload.Encoding)
	assert.Equal(t, unencodedSize, payload.UnencodedSize)

	// Test Count method
	assert.Equal(t, int64(3), payload.Count())

	// Test Size method (each message is 5, 5, and 4 bytes respectively)
	assert.Equal(t, int64(14), payload.Size())
}
func TestPayloadPreservesMessageOrder(t *testing.T) {
	messages := []*Message{
		NewMessage([]byte("1"), nil, "", 1),    // datalen = 1
		NewMessage([]byte("22"), nil, "", 2),   // datalen = 2
		NewMessage([]byte("333"), nil, "", 3),  // datalen = 3
		NewMessage([]byte("4444"), nil, "", 4), // datalen = 4
	}
	messageMetas := make([]*MessageMetadata, len(messages))
	for i, msg := range messages {
		messageMetas[i] = &msg.MessageMetadata
	}

	payload := NewPayload(messageMetas, []byte(""), "", 0)

	expectedLengths := []int{1, 2, 3, 4}
	assert.Equal(t, len(expectedLengths), len(payload.MessageMetas), "Should have same number of message metas")

	for i, msg := range messages {
		assert.Equal(t, msg.RawDataLen, payload.MessageMetas[i].RawDataLen, "Message at index %d should have RawDataLen of %d", i, msg.GetContent())
		assert.Equal(t, msg.IngestionTimestamp, payload.MessageMetas[i].IngestionTimestamp, "Message at index %d should have ingestion timestamp %d", i, msg.IngestionTimestamp)
	}
}

func TestPayloadAllowsMessageContentGC(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	// Create a function to track when our test content gets GC'd
	wasGCd := false
	trackGC := func(_ *[]byte) {
		wasGCd = true
		wg.Done()
	}

	var payload *Payload

	// Create scope to allow message to be GC'd
	func() {
		// Create message with content we want to track
		content := make([]byte, 1000000) // Large enough to be noticeable for GC
		message := NewMessage(content, nil, "", 2)

		// Set up finalizer to track when content is GC'd
		runtime.SetFinalizer(&message.content, trackGC)

		// Create payload from message
		meta := message.MessageMetadata // Copy metadata instead of taking reference
		payload = NewPayload([]*MessageMetadata{&meta}, []byte("encoded"), "", 0)
		message = nil

		// Ensure payload captured metadata
		require.Equal(t, 1, len(payload.MessageMetas))
		require.Equal(t, int64(2), payload.MessageMetas[0].IngestionTimestamp)
	}()

	// Clear any references and force GC
	runtime.GC()

	// Wait for finalizer with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.True(t, wasGCd, "Message content should have been garbage collected")
	case <-time.After(time.Second):
		t.Fatal("Message content was not garbage collected")
	}

	// Verify payload metadata still intact
	assert.Equal(t, 1, len(payload.MessageMetas))
	assert.Equal(t, int64(2), payload.MessageMetas[0].IngestionTimestamp)
}

func TestMessageContentStates(t *testing.T) {
	t.Run("unstructured state", func(t *testing.T) {
		msg := NewMessage([]byte("test content"), nil, StatusInfo, time.Now().UnixNano())
		assert.Equal(t, StateUnstructured, msg.State)
		assert.Equal(t, "test content", string(msg.GetContent()))
	})

	t.Run("rendered state", func(t *testing.T) {
		msg := NewMessage([]byte("original"), nil, StatusInfo, time.Now().UnixNano())
		msg.SetRendered([]byte("rendered content"))
		assert.Equal(t, StateRendered, msg.State)
		assert.Equal(t, "rendered content", string(msg.GetContent()))
	})

	t.Run("encoded state", func(t *testing.T) {
		msg := NewMessage([]byte("original"), nil, StatusInfo, time.Now().UnixNano())
		msg.SetEncoded([]byte("encoded content"))
		assert.Equal(t, StateEncoded, msg.State)
		assert.Equal(t, "encoded content", string(msg.GetContent()))
	})
}

func TestMessageRender(t *testing.T) {
	t.Run("unstructured message", func(t *testing.T) {
		msg := NewMessage([]byte("test"), nil, StatusInfo, 0)
		rendered, err := msg.Render()
		assert.NoError(t, err)
		assert.Equal(t, []byte("test"), rendered)
	})

	t.Run("rendered message", func(t *testing.T) {
		msg := NewMessage([]byte("original"), nil, StatusInfo, 0)
		msg.SetRendered([]byte("rendered"))
		rendered, err := msg.Render()
		assert.NoError(t, err)
		assert.Equal(t, []byte("rendered"), rendered)
	})

	t.Run("encoded message returns error", func(t *testing.T) {
		msg := NewMessage([]byte("original"), nil, StatusInfo, 0)
		msg.SetEncoded([]byte("encoded"))
		_, err := msg.Render()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "encoded message")
	})
}

func TestBasicStructuredContent(t *testing.T) {
	t.Run("render", func(t *testing.T) {
		content := &BasicStructuredContent{
			Data: map[string]interface{}{
				"message": "test message",
				"level":   "info",
			},
		}
		rendered, err := content.Render()
		assert.NoError(t, err)
		assert.Contains(t, string(rendered), "test message")
		assert.Contains(t, string(rendered), "info")
	})

	t.Run("get content", func(t *testing.T) {
		content := &BasicStructuredContent{
			Data: map[string]interface{}{
				"message": "hello world",
			},
		}
		assert.Equal(t, []byte("hello world"), content.GetContent())
	})

	t.Run("get content missing message", func(t *testing.T) {
		content := &BasicStructuredContent{
			Data: map[string]interface{}{
				"other": "value",
			},
		}
		assert.Equal(t, []byte{}, content.GetContent())
	})

	t.Run("set content", func(t *testing.T) {
		content := &BasicStructuredContent{
			Data: map[string]interface{}{},
		}
		content.SetContent([]byte("new content"))
		assert.Equal(t, "new content", content.Data["message"])
	})
}

func TestStructuredMessage(t *testing.T) {
	content := &BasicStructuredContent{
		Data: map[string]interface{}{
			"message": "structured message",
		},
	}
	origin := NewOrigin(nil)
	msg := NewStructuredMessage(content, origin, StatusInfo, time.Now().UnixNano())

	assert.Equal(t, StateStructured, msg.State)
	assert.Equal(t, []byte("structured message"), msg.GetContent())

	// Test SetContent on structured message
	msg.SetContent([]byte("modified"))
	assert.Equal(t, "modified", content.Data["message"])

	// Test Render on structured message
	rendered, err := msg.Render()
	assert.NoError(t, err)
	assert.Contains(t, string(rendered), "modified")
}

func TestNewStructuredMessageWithParsingExtra(t *testing.T) {
	content := &BasicStructuredContent{
		Data: map[string]interface{}{
			"message": "test",
		},
	}
	msg := NewStructuredMessageWithParsingExtra(content, nil, StatusInfo, 0, true)
	assert.True(t, msg.ParsingExtra.IsTruncated)
}

func TestMessageMetadataMethods(t *testing.T) {
	t.Run("GetStatus with empty status", func(t *testing.T) {
		meta := &MessageMetadata{Status: ""}
		assert.Equal(t, StatusInfo, meta.GetStatus())
	})

	t.Run("GetStatus with set status", func(t *testing.T) {
		meta := &MessageMetadata{Status: StatusError}
		assert.Equal(t, StatusError, meta.GetStatus())
	})

	t.Run("GetLatency", func(t *testing.T) {
		ingestionTime := time.Now().UnixNano()
		meta := &MessageMetadata{IngestionTimestamp: ingestionTime}
		latency := meta.GetLatency()
		assert.True(t, latency >= 0)
	})

	t.Run("Count", func(t *testing.T) {
		meta := &MessageMetadata{}
		assert.Equal(t, int64(1), meta.Count())
	})

	t.Run("Size", func(t *testing.T) {
		meta := &MessageMetadata{RawDataLen: 100}
		assert.Equal(t, int64(100), meta.Size())
	})
}

func TestTagFunctions(t *testing.T) {
	t.Run("TruncatedReasonTag", func(t *testing.T) {
		tag := TruncatedReasonTag("single_line")
		assert.Equal(t, "truncated:single_line", tag)
	})

	t.Run("MultiLineSourceTag", func(t *testing.T) {
		tag := MultiLineSourceTag("docker")
		assert.Equal(t, "multiline:docker", tag)
	})

	t.Run("LogSourceTag", func(t *testing.T) {
		tag := LogSourceTag("stdout")
		assert.Equal(t, "logsource:stdout", tag)
	})
}

func TestNewMessageWithParsingExtra(t *testing.T) {
	parsingExtra := ParsingExtra{
		Timestamp:   "2024-01-01T00:00:00Z",
		IsPartial:   true,
		IsTruncated: true,
		IsMultiLine: true,
	}
	msg := NewMessageWithParsingExtra([]byte("test"), nil, StatusInfo, 0, parsingExtra)
	assert.Equal(t, parsingExtra.Timestamp, msg.ParsingExtra.Timestamp)
	assert.True(t, msg.ParsingExtra.IsPartial)
	assert.True(t, msg.ParsingExtra.IsTruncated)
	assert.True(t, msg.ParsingExtra.IsMultiLine)
}

func TestPayloadIsMRF(t *testing.T) {
	t.Run("empty payload", func(t *testing.T) {
		payload := NewPayload([]*MessageMetadata{}, []byte{}, "", 0)
		assert.False(t, payload.IsMRF())
	})

	t.Run("non-MRF payload", func(t *testing.T) {
		meta := &MessageMetadata{ParsingExtra: ParsingExtra{IsMRFAllow: false}}
		payload := NewPayload([]*MessageMetadata{meta}, []byte{}, "", 0)
		assert.False(t, payload.IsMRF())
	})

	t.Run("MRF payload", func(t *testing.T) {
		meta := &MessageMetadata{ParsingExtra: ParsingExtra{IsMRFAllow: true}}
		payload := NewPayload([]*MessageMetadata{meta}, []byte{}, "", 0)
		assert.True(t, payload.IsMRF())
	})
}
