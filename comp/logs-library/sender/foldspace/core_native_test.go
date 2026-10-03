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
// zero reconnect backoff, and requires snapshot_batch_id below
// first_payload_batch_id (the zero value for both is not below itself).
func TestNativeCoreRoundTrip(t *testing.T) {
	core, err := NewNativeCore(Config{
		Endpoints:              []Endpoint{{Address: "127.0.0.1:1", Class: Reliable}},
		MaxInflightPayloads:    16,
		BatchCapacity:          10,
		MaxPayloadBytes:        1024,
		ReconnectBackoffBase:   time.Second,
		ReconnectBackoffFactor: 2,
		ReconnectBackoffCap:    30 * time.Second,
		FirstPayloadBatchID:    1,
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
	}, uint64(time.Now().UnixNano()), 42, AllSenders(1))
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

	// Reporting the stream open is what releases the sealed batch. A payload
	// that references rule state (tags, in this record) can be preceded by a
	// reserved snapshot batch carrying those definitions, so more than one
	// SendBatch effect can appear here. Acks on this library are matched
	// positionally against the outstanding FIFO rather than by batch_id value
	// (a payload needing no rule definitions can even leave that snapshot
	// batch trailing a numerically later payload batch), so every batch this
	// stream sent must be acked in the order PollSender returned it, not
	// filtered down to "the" payload batch.
	core.HandleStreamOpened(0, stream)
	sending := core.PollSender(0)
	require.NotEmpty(t, sending, "an open stream should release the sealed batch")

	var ack Progress
	sawSendBatch := false
	for i := range sending {
		e := sending[i]
		t.Logf("effect kind=%v sender=%d stream=%d batch=%d", e.Kind, e.Sender, e.Stream, e.BatchID)
		if e.Kind != SendBatch {
			continue
		}
		sawSendBatch = true
		require.NotEmpty(t, e.Batch.Bytes(), "a sealed batch carries bytes")
		t.Logf("batch %d carries %d bytes", e.BatchID, len(e.Batch.Bytes()))
		e.Batch.Release()
		ack = core.HandleAck(0, stream, e.BatchID, AckOK)
	}
	require.True(t, sawSendBatch, "the sealed batch should be offered for sending")

	// Acknowledging every batch in order is what makes the record durable,
	// reported as a notification naming the metadata id offered above. A
	// telemetry notification for the reserved snapshot batch's own send can
	// share the poll with it, so the durable resolution is found by kind
	// rather than assumed to be the first entry.
	require.True(t, ack.NotificationsReady, "a durable payload should be announced")
	notifications := core.PollNotifications()
	require.NotEmpty(t, notifications)
	var durable *Notification
	for i := range notifications {
		t.Logf("notification kind=%v sender=%d hasSender=%v records=%d bytes=%d ids=%v",
			notifications[i].Kind, notifications[i].Sender, notifications[i].HasSender,
			notifications[i].Records, notifications[i].Bytes, notifications[i].MetadataIDs)
		if notifications[i].Kind == PayloadDurable {
			durable = &notifications[i]
		}
	}
	require.NotNil(t, durable, "a durable payload should be announced")
	require.Equal(t, []uint64{42}, durable.MetadataIDs)

	// Clock advance is the difference between readings, so it only becomes
	// non-zero once a second, later timestamp has been offered.
	require.Zero(t, core.TakeClockAdvance())
}
