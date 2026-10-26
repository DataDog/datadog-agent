// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package foldspace

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// simultaneousFailureStream holds Send while Recv fails. The test waits until
// the receive error is queued before letting Send report the same failure.
// This models the two halves of a bidi stream observing one disconnect.
type simultaneousFailureStream struct {
	sendEntered chan struct{}
	releaseSend chan struct{}
}

func (s *simultaneousFailureStream) Send(context.Context, uint32, []byte) error {
	close(s.sendEntered)
	<-s.releaseSend
	return errors.New("send observed disconnect")
}

func (s *simultaneousFailureStream) Recv(ctx context.Context) (uint32, int32, error) {
	select {
	case <-s.sendEntered:
		return 0, 0, errors.New("receive observed disconnect")
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	}
}

func (*simultaneousFailureStream) Close() error { return nil }

type trackedReplacementStream struct {
	Stream
	closed atomic.Bool
}

func (s *trackedReplacementStream) Close() error {
	s.closed.Store(true)
	return s.Stream.Close()
}

type simultaneousFailureTransport struct {
	first       *simultaneousFailureStream
	base        *FakeTransport
	replacement *trackedReplacementStream
	opens       int
}

func (t *simultaneousFailureTransport) OpenStream(ctx context.Context, sender SenderID, stream StreamID) (Stream, error) {
	t.opens++
	if t.opens == 1 {
		return t.first, nil
	}
	s, err := t.base.OpenStream(ctx, sender, stream)
	if err != nil {
		return nil, err
	}
	t.replacement = &trackedReplacementStream{Stream: s}
	return t.replacement, nil
}

func TestStaleReceiveErrorDoesNotCloseReplacementStream(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		transport := &simultaneousFailureTransport{
			first: &simultaneousFailureStream{
				sendEntered: make(chan struct{}),
				releaseSend: make(chan struct{}),
			},
			base: NewFakeTransport(1),
		}
		core := NewFakeCore(FakeCoreConfig{Classes: []SenderClass{Reliable}})
		d := NewDriver(DriverOptions{
			Core:            core,
			Transport:       transport,
			Sink:            newChannelSink(),
			AckTimeout:      30 * time.Second,
			ShutdownTimeout: time.Second,
		})
		d.Start()
		defer d.Stop()

		d.Offer(testMessage("first"))
		// Send is blocked, and the receive loop has queued its error and exited.
		// No sleeps or scheduler races are needed to reproduce the stale event.
		synctest.Wait()
		close(transport.first.releaseSend)
		synctest.Wait()

		require.NotNil(t, transport.replacement)
		assert.False(t, transport.replacement.closed.Load(),
			"an old generation's queued receive error must not close the new stream")

		d.Offer(testMessage("second"))
		synctest.Wait()
		// FakeCore does not replay failed writes. This assertion specifically
		// checks that the next record can use the replacement transport.
		assert.Equal(t, [][]byte{[]byte("second")}, transport.base.Sent(0),
			"new records must be sent immediately, not wait for stream rotation")

		// A skipped SendBatch never increments the Go outstanding count, so
		// the ACK watchdog cannot rescue this bug even after its timeout.
		time.Sleep(2 * d.ackTimeout)
		synctest.Wait()
		assert.Equal(t, 2, transport.opens)
		assert.Equal(t, [][]byte{[]byte("second")}, transport.base.Sent(0))
	})
}
