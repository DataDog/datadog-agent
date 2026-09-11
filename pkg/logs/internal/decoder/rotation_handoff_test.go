// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/preprocessor"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// newMultilineDecoderForHandoffTest builds a decoder whose LineHandler buffers a
// multiline group, which is the content a rotation handoff carries.
func newMultilineDecoderForHandoffTest(t *testing.T) Decoder {
	t.Helper()
	configmock.New(t)
	source := sources.NewLogSource("", &config.LogsConfig{
		ProcessingRules: []*config.ProcessingRule{{
			Type:  config.MultiLine,
			Name:  "date_prefix",
			Regex: regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`),
		}},
	})
	return NewDecoderFromSource(sources.NewReplaceableSource(source), status.NewInfoRegistry())
}

// openGroup feeds a group-start line plus a continuation, leaving the handler
// with a buffered group that has not been emitted.
func openGroup(t *testing.T, d Decoder) {
	t.Helper()
	d.InputChan() <- NewInput([]byte("2026-01-01 foo 1\nfoo 2\n"))
}

func drain(d Decoder, timeout time.Duration) []string {
	var got []string
	deadline := time.After(timeout)
	for {
		select {
		case msg, open := <-d.OutputChan():
			if !open {
				return got
			}
			got = append(got, string(msg.GetContent()))
		case <-deadline:
			return got
		}
	}
}

// TestRotationHandoffDeadlineReturnsContentToSender covers the case where no
// replacement decoder ever shows up: the sender must take its content back and
// emit it, never drop it.
func TestRotationHandoffDeadlineReturnsContentToSender(t *testing.T) {
	d := newMultilineDecoderForHandoffTest(t)
	handoff := NewRotationHandoff(200 * time.Millisecond)
	d.SetRotationHandoffTarget(handoff)
	d.Start()

	openGroup(t, d)
	// Stands in for the tailer reaching the end of the rotated-away file.
	d.CompleteRotationHandoff()

	// Nothing claims the offer, so after the deadline plus the regular
	// aggregation timeout the group is emitted as-is.
	select {
	case msg := <-d.OutputChan():
		assert.Equal(t, "2026-01-01 foo 1\\nfoo 2", string(msg.GetContent()))
	case <-time.After(5 * time.Second):
		t.Fatal("the unclaimed group was never emitted")
	}

	d.Stop()
}

// TestRotationHandoffCancelReturnsContentToSender covers a replacement tailer
// that fails to start: the cancelled offer must not swallow the content.
func TestRotationHandoffCancelReturnsContentToSender(t *testing.T) {
	d := newMultilineDecoderForHandoffTest(t)
	handoff := NewRotationHandoff(10 * time.Second)
	d.SetRotationHandoffTarget(handoff)
	d.Start()

	openGroup(t, d)
	d.CompleteRotationHandoff()
	// The replacement tailer failed to start.
	handoff.Cancel()

	// Stopping must still emit the group rather than leave it in the handoff.
	go d.Stop()
	got := drain(d, 5*time.Second)
	assert.Contains(t, got, "2026-01-01 foo 1\\nfoo 2")
}

// TestRotationHandoffStopWithoutReceiverFlushes covers the sender shutting down
// while an offer is outstanding and no receiver is waiting.
func TestRotationHandoffStopWithoutReceiverFlushes(t *testing.T) {
	d := newMultilineDecoderForHandoffTest(t)
	handoff := NewRotationHandoff(10 * time.Second)
	d.SetRotationHandoffTarget(handoff)
	d.Start()

	openGroup(t, d)
	d.CompleteRotationHandoff()

	go d.Stop()
	got := drain(d, 5*time.Second)
	assert.Contains(t, got, "2026-01-01 foo 1\\nfoo 2")
}

// TestRotationHandoffTransfersToReceiver is the happy path across two decoders:
// the group opened on the first is completed by the first line the second sees.
func TestRotationHandoffTransfersToReceiver(t *testing.T) {
	sender := newMultilineDecoderForHandoffTest(t)
	receiver := newMultilineDecoderForHandoffTest(t)

	handoff := NewRotationHandoff(5 * time.Second)
	receiver.AwaitRotationHandoff(handoff)
	receiver.Start()
	sender.SetRotationHandoffTarget(handoff)
	sender.Start()

	openGroup(t, sender)
	sender.CompleteRotationHandoff()

	// The continuation is the first thing in the new file.
	receiver.InputChan() <- NewInput([]byte("foo 3\n2026-01-01 bar 1\n"))

	select {
	case msg := <-receiver.OutputChan():
		assert.Equal(t, "2026-01-01 foo 1\\nfoo 2\\nfoo 3", string(msg.GetContent()))
	case <-time.After(5 * time.Second):
		t.Fatal("the reassembled group was never emitted")
	}

	// The sender handed its content over, so it has nothing left of its own.
	go sender.Stop()
	assert.Empty(t, drain(sender, time.Second))

	receiver.Stop()
}

// TestRotationHandoffOwnershipIsExclusive pins the ownership rules the "never
// drop content" guarantee rests on.
func TestRotationHandoffOwnershipIsExclusive(t *testing.T) {
	state := &PendingState{
		HandlerContent: []preprocessor.PendingContent{{Content: []byte("buffered")}},
	}

	t.Run("claim after park transfers ownership", func(t *testing.T) {
		h := NewRotationHandoff(time.Second)
		require.True(t, h.beginReceive())
		require.True(t, h.park(state, false))
		got, ok := h.claim()
		assert.True(t, ok)
		assert.Same(t, state, got)
		// The sender must not get it back once claimed.
		_, ok = h.reclaim()
		assert.False(t, ok)
	})

	t.Run("reclaim after an unclaimed park returns to the sender", func(t *testing.T) {
		h := NewRotationHandoff(time.Second)
		require.True(t, h.park(state, false))
		got, ok := h.reclaim()
		assert.True(t, ok)
		assert.Same(t, state, got)
		// And the receiver can no longer take it.
		_, ok = h.claim()
		assert.False(t, ok)
	})

	t.Run("a failed claim blocks a later park", func(t *testing.T) {
		h := NewRotationHandoff(time.Second)
		_, ok := h.claim()
		assert.False(t, ok)
		assert.False(t, h.park(state, false), "the sender must keep content the receiver gave up on")
	})

	t.Run("a shutting-down sender only parks for a waiting receiver", func(t *testing.T) {
		h := NewRotationHandoff(time.Second)
		assert.False(t, h.park(state, true), "no receiver is waiting, so the sender must flush")

		h = NewRotationHandoff(time.Second)
		require.True(t, h.beginReceive())
		assert.True(t, h.park(state, true))
	})

	t.Run("cancel blocks any transfer", func(t *testing.T) {
		h := NewRotationHandoff(time.Second)
		h.Cancel()
		assert.False(t, h.beginReceive())
		assert.False(t, h.park(state, false))
	})
}
