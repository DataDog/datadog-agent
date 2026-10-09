// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package foldspace

import (
	"context"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// Transport opens a stream for one sender generation.
type Transport interface {
	OpenStream(ctx context.Context, sender SenderID, stream StreamID) (Stream, error)
}

// Stream is one bidirectional batch/ack connection.
type Stream interface {
	// Send must return once ctx is done, even while blocked on flow control.
	// The stream is unusable afterwards.
	Send(ctx context.Context, batchID uint32, data []byte) error
	Recv(ctx context.Context) (batchID uint32, status int32, err error)
	Close() error
}

// FakeTransport records sealed bytes per sender and acks OK in send order.
type FakeTransport struct {
	mu        sync.Mutex
	sent      [][][]byte
	openErr   []error
	ackOK     bool
	blockRecv bool
	// stallStreams is how many streams, in open order, block every Send until
	// its ctx ends.
	stallStreams int
	opens        int
	// blockOpen makes every OpenStream wait until its ctx ends, as a dial to an
	// intake that never answers does.
	blockOpen bool
}

// NewFakeTransport returns a transport with n senders that acks OK.
func NewFakeTransport(n int) *FakeTransport {
	sent := make([][][]byte, n)
	return &FakeTransport{sent: sent, ackOK: true}
}

// Sent returns a copy of the sealed bytes written to sender.
func (t *FakeTransport) Sent(sender SenderID) [][]byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([][]byte(nil), t.sent[sender]...)
}

// SetOpenError makes the next OpenStream for sender fail.
func (t *FakeTransport) SetOpenError(sender SenderID, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.openErr == nil {
		t.openErr = make([]error, len(t.sent))
	}
	t.openErr[sender] = err
}

// OpenStream returns a FakeStream that records writes and acks OK.
func (t *FakeTransport) OpenStream(ctx context.Context, sender SenderID, _ StreamID) (Stream, error) {
	t.mu.Lock()
	if t.blockOpen {
		t.mu.Unlock()
		<-ctx.Done()
		return nil, ctx.Err()
	}
	defer t.mu.Unlock()
	if t.openErr != nil && t.openErr[sender] != nil {
		err := t.openErr[sender]
		t.openErr[sender] = nil
		return nil, err
	}
	t.opens++
	return &FakeStream{
		transport: t,
		sender:    sender,
		acks:      make(chan ack, 64),
		blockRecv: t.blockRecv,
		stallSend: t.opens <= t.stallStreams,
	}, nil
}

// Opens returns how many streams have been opened across all senders.
func (t *FakeTransport) Opens() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.opens
}

type ack struct {
	id     uint32
	status int32
}

// FakeStream is one FakeTransport connection.
type FakeStream struct {
	transport *FakeTransport
	sender    SenderID
	acks      chan ack
	blockRecv bool
	stallSend bool
}

// Send records the bytes and queues an OK ack. When stallSend is set it waits
// until ctx ends, as a send blocked on flow control does.
func (s *FakeStream) Send(ctx context.Context, batchID uint32, data []byte) error {
	if s.stallSend {
		<-ctx.Done()
		return ctx.Err()
	}
	s.transport.mu.Lock()
	s.transport.sent[s.sender] = append(s.transport.sent[s.sender], append([]byte(nil), data...))
	s.transport.mu.Unlock()
	status := int32(0)
	if s.transport.ackOK {
		status = AckOK
	}
	s.acks <- ack{id: batchID, status: status}
	return nil
}

// Recv returns the next queued ack. When blockRecv is set it waits until ctx ends.
func (s *FakeStream) Recv(ctx context.Context) (uint32, int32, error) {
	if s.blockRecv {
		<-ctx.Done()
		return 0, 0, ctx.Err()
	}
	select {
	case <-ctx.Done():
		return 0, 0, ctx.Err()
	case a := <-s.acks:
		return a.id, a.status, nil
	}
}

// Close is a no-op.
func (s *FakeStream) Close() error { return nil }

// recordFromMessage maps a processed message onto a foldspace Record.
func recordFromMessage(msg *message.Message) Record {
	r := Record{
		Body:            append([]byte(nil), msg.GetContent()...),
		TimestampMillis: msg.GetTimestampUnixMilli(),
		Status:          msg.Status,
		Hostname:        msg.GetHostname(),
		MRFAllowed:      msg.ParsingExtra.IsMRFAllow,
	}
	if msg.Origin != nil {
		r.Service = msg.Origin.Service()
		r.Source = msg.Origin.Source()
		r.Tags = append([]string(nil), msg.Origin.Tags()...)
	}
	if len(msg.ParsingExtra.Tags) > 0 {
		r.ProcessingTags = append([]string(nil), msg.ParsingExtra.Tags...)
	}
	return r
}
