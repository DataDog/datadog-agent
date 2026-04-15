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

func TestHasContent(t *testing.T) {
	t.Run("unstructured with content", func(t *testing.T) {
		msg := NewMessage([]byte("hello"), nil, "", 0)
		assert.True(t, msg.HasContent())
	})

	t.Run("unstructured empty", func(t *testing.T) {
		msg := NewMessage([]byte{}, nil, "", 0)
		assert.False(t, msg.HasContent())
	})

	t.Run("structured with message", func(t *testing.T) {
		sc := &BasicStructuredContent{Data: map[string]interface{}{"message": "hello"}}
		msg := NewStructuredMessage(sc, nil, "", 0)
		assert.True(t, msg.HasContent())
	})

	t.Run("structured with empty message", func(t *testing.T) {
		sc := &BasicStructuredContent{Data: map[string]interface{}{"message": "", "siem": map[string]interface{}{"format": "CEF"}}}
		msg := NewStructuredMessage(sc, nil, "", 0)
		assert.True(t, msg.HasContent())
	})

	t.Run("structured nil content", func(t *testing.T) {
		msg := &Message{MessageContent: MessageContent{State: StateStructured, structuredContent: nil}}
		assert.False(t, msg.HasContent())
	})

	t.Run("rendered with content", func(t *testing.T) {
		msg := NewMessage([]byte("hello"), nil, "", 0)
		msg.SetRendered([]byte(`{"message":"hello"}`))
		assert.True(t, msg.HasContent())
	})

	t.Run("rendered empty", func(t *testing.T) {
		msg := NewMessage(nil, nil, "", 0)
		msg.SetRendered([]byte{})
		assert.False(t, msg.HasContent())
	})
}

func TestGetStructuredAttribute(t *testing.T) {
	t.Run("top-level string", func(t *testing.T) {
		sc := &BasicStructuredContent{Data: map[string]interface{}{"message": "hello"}}
		msg := NewStructuredMessage(sc, nil, "", 0)
		val, ok := msg.GetStructuredAttribute("message")
		assert.True(t, ok)
		assert.Equal(t, "hello", val)
	})

	t.Run("nested dot path", func(t *testing.T) {
		sc := &BasicStructuredContent{Data: map[string]interface{}{
			"siem": map[string]interface{}{
				"device_vendor": "Security",
			},
		}}
		msg := NewStructuredMessage(sc, nil, "", 0)
		val, ok := msg.GetStructuredAttribute("siem.device_vendor")
		assert.True(t, ok)
		assert.Equal(t, "Security", val)
	})

	t.Run("deeply nested", func(t *testing.T) {
		sc := &BasicStructuredContent{Data: map[string]interface{}{
			"a": map[string]interface{}{
				"b": map[string]interface{}{
					"c": "deep",
				},
			},
		}}
		msg := NewStructuredMessage(sc, nil, "", 0)
		val, ok := msg.GetStructuredAttribute("a.b.c")
		assert.True(t, ok)
		assert.Equal(t, "deep", val)
	})

	t.Run("numeric leaf", func(t *testing.T) {
		sc := &BasicStructuredContent{Data: map[string]interface{}{
			"count": float64(42),
		}}
		msg := NewStructuredMessage(sc, nil, "", 0)
		val, ok := msg.GetStructuredAttribute("count")
		assert.True(t, ok)
		assert.Equal(t, "42", val)
	})

	t.Run("missing key returns false", func(t *testing.T) {
		sc := &BasicStructuredContent{Data: map[string]interface{}{"message": "hello"}}
		msg := NewStructuredMessage(sc, nil, "", 0)
		_, ok := msg.GetStructuredAttribute("nonexistent")
		assert.False(t, ok)
	})

	t.Run("missing nested key returns false", func(t *testing.T) {
		sc := &BasicStructuredContent{Data: map[string]interface{}{
			"siem": map[string]interface{}{},
		}}
		msg := NewStructuredMessage(sc, nil, "", 0)
		_, ok := msg.GetStructuredAttribute("siem.device_vendor")
		assert.False(t, ok)
	})

	t.Run("unstructured message returns false", func(t *testing.T) {
		msg := NewMessage([]byte("hello"), nil, "", 0)
		_, ok := msg.GetStructuredAttribute("message")
		assert.False(t, ok)
	})

	t.Run("map leaf returns false", func(t *testing.T) {
		sc := &BasicStructuredContent{Data: map[string]interface{}{
			"siem": map[string]interface{}{
				"nested": map[string]interface{}{"a": "b"},
			},
		}}
		msg := NewStructuredMessage(sc, nil, "", 0)
		_, ok := msg.GetStructuredAttribute("siem.nested")
		assert.False(t, ok)
	})
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

// ---------------------------------------------------------------------------
// EnsureRendered
// ---------------------------------------------------------------------------

// fullEncoderSC implements both StructuredContent and FullEncoder for testing.
type fullEncoderSC struct {
	BasicStructuredContent
}

func (f *fullEncoderSC) EncodeFull(_ string, _ int64, _, _, _, _ string) ([]byte, error) {
	return []byte(`{"message":"mock"}`), nil
}

func TestEnsureRendered(t *testing.T) {
	t.Run("unstructured promotes state", func(t *testing.T) {
		msg := NewMessage([]byte("raw"), nil, "", 0)
		assert.Equal(t, StateUnstructured, msg.State)

		require.NoError(t, msg.EnsureRendered())
		assert.Equal(t, StateRendered, msg.State)
		assert.Equal(t, "raw", string(msg.GetContent()))
	})

	t.Run("already rendered is no-op", func(t *testing.T) {
		msg := NewMessage([]byte("data"), nil, "", 0)
		msg.SetRendered([]byte("rendered"))

		require.NoError(t, msg.EnsureRendered())
		assert.Equal(t, StateRendered, msg.State)
		assert.Equal(t, "rendered", string(msg.GetContent()))
	})

	t.Run("already encoded is no-op", func(t *testing.T) {
		msg := NewMessage([]byte("data"), nil, "", 0)
		msg.SetEncoded([]byte("encoded"))

		require.NoError(t, msg.EnsureRendered())
		assert.Equal(t, StateEncoded, msg.State)
		assert.Equal(t, "encoded", string(msg.GetContent()))
	})

	t.Run("structured without FullEncoder renders", func(t *testing.T) {
		sc := &BasicStructuredContent{Data: map[string]interface{}{"message": "hello"}}
		msg := NewStructuredMessage(sc, nil, "", 0)
		assert.Equal(t, StateStructured, msg.State)

		require.NoError(t, msg.EnsureRendered())
		assert.Equal(t, StateRendered, msg.State)
		assert.Contains(t, string(msg.GetContent()), `"message":"hello"`)
	})

	t.Run("structured with FullEncoder skips render", func(t *testing.T) {
		sc := &fullEncoderSC{BasicStructuredContent{Data: map[string]interface{}{"message": "hello"}}}
		msg := NewStructuredMessage(sc, nil, "", 0)
		assert.Equal(t, StateStructured, msg.State)

		require.NoError(t, msg.EnsureRendered())
		assert.Equal(t, StateStructured, msg.State, "state should stay StateStructured for FullEncoder")
	})
}
