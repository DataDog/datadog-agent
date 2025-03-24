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
	encoded := []byte("encoded content")
	encoding := "gzip"
	unencodedSize := 100

	payload := NewPayload(messages, encoded, encoding, unencodedSize)

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
	payload := NewPayload(messages, []byte(""), "", 0)

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
		payload = NewPayload([]*Message{message}, []byte("encoded"), "", 0)

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
