// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build foldspace

package foldspace

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestNativeCoreRoundTrip drives one record all the way through the real
// library: admission, sealing, the stream the library asks for, the batch bytes
// it hands back, and the acknowledgement that makes the record durable.
//
// It covers the parts of the bridge that a compile cannot: pinned record
// marshalling, the caller-owned wake array, effect and notification iteration
// with their separate frees, and the copy out of a batch lease.
//
// The config must be complete, because the library validates it and refuses a
// zero reconnect backoff.
func TestNativeCoreRoundTrip(t *testing.T) {
	core, err := NewNativeCore(Config{
		Endpoints:              []Endpoint{{Address: "127.0.0.1:1", Class: Reliable}},
		MaxInflightPayloads:    16,
		BatchCapacity:          10,
		MaxPayloadBytes:        1024,
		ReconnectBackoffBase:   time.Second,
		ReconnectBackoffFactor: 2,
		ReconnectBackoffCap:    30 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(core.Close)

	require.Equal(t, 1, core.SenderCount())
	require.Equal(t, Reliable, core.SenderClass(0))

	start := core.Start()
	t.Logf("start: wake=%v hasCapacity=%v", start.Wake, start.HasCapacity)
	require.True(t, core.HasCapacity())

	admission, progress := core.PushLog(Record{
		Body:            []byte("hello from the bridge"),
		TimestampMillis: time.Now().UnixMilli(),
		Service:         "svc",
		Status:          "info",
		Source:          "src",
		Hostname:        "host",
		Tags:            []string{"a:1", "b:2"},
		ProcessingTags:  []string{"c:3"},
	}, uint64(time.Now().UnixNano()), 42)
	require.Equal(t, Accepted, admission)
	t.Logf("push: wake=%v hasCapacity=%v notifications=%v", progress.Wake, progress.HasCapacity, progress.NotificationsReady)

	// Flushing seals the batch, which is the only thing that produces effects.
	flushAdmission, _ := core.Flush()
	require.Equal(t, Accepted, flushAdmission)

	// The library asks for a stream before it will hand over bytes, so the
	// first poll yields only OpenStream.
	opening := core.PollSender(0)
	require.Len(t, opening, 1)
	require.Equal(t, OpenStream, opening[0].Kind)
	stream := opening[0].Stream

	// Reporting the stream open is what releases the sealed batch.
	core.HandleStreamOpened(0, stream)
	sending := core.PollSender(0)
	require.NotEmpty(t, sending, "an open stream should release the sealed batch")

	var sent *Effect
	for i := range sending {
		t.Logf("effect kind=%v sender=%d stream=%d batch=%d",
			sending[i].Kind, sending[i].Sender, sending[i].Stream, sending[i].BatchID)
		if sending[i].Kind == SendBatch {
			sent = &sending[i]
		}
	}
	require.NotNil(t, sent, "the sealed batch should be offered for sending")
	require.NotEmpty(t, sent.Batch.Bytes(), "a sealed batch carries bytes")
	t.Logf("batch %d carries %d bytes", sent.BatchID, len(sent.Batch.Bytes()))
	sent.Batch.Release()

	// Acknowledging it makes the record durable, which is reported as a
	// notification naming the metadata id offered above.
	ack := core.HandleAck(0, stream, sent.BatchID, AckOK)
	require.True(t, ack.NotificationsReady, "a durable payload should be announced")
	notifications := core.PollNotifications()
	require.NotEmpty(t, notifications)
	require.Equal(t, PayloadDurable, notifications[0].Kind)
	require.Equal(t, []uint64{42}, notifications[0].MetadataIDs)

	// Clock advance is the difference between readings, so it only becomes
	// non-zero once a second, later timestamp has been offered.
	require.Zero(t, core.TakeClockAdvance())
}
