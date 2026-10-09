// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`) at commit
// 4961722de9009afdbbb711fc0adf14a8f6ff9277 (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the darwin build requires cgo, unsupported platforms get a
// stub so this tree still compiles, and test files carry an explicit platform
// gate. The transport logic, framing, and ring layout are unchanged.

// Package fitcore is the Go transport template of the Fast IPC Toolkit.
//
// It implements setup, POSIX shared-memory ownership, a bounded SPSC message
// ring, and native address waits, and is byte-compatible with the Rust
// fit-core template: peers built from either package interoperate over the
// same setup handshake and ring layout. The package has no application
// message codecs, type constants, or scheduling; those belong in a local
// protocol package derived from the application's protocol documents.
//
// The supported profile is little-endian amd64/arm64 Linux and macOS 14.4 or
// newer, with native 32-bit atomics. Linux builds are pure Go; macOS builds
// bind the public libSystem shm_open/os_sync_wait_on_address symbols through
// cgo and require CGO_ENABLED=1. The consumer listens on and owns the setup
// endpoint and the shared-memory object; the setup connection closes after
// Hello → Offer → Ready → Start. There is no liveness monitor or automatic
// recovery.
//
// Usage:
//
//	producer, err := fitcore.ConnectProducer(cfg, protocol.Descriptor)
//	result, err := producer.SendBatch([]fitcore.Record{{Kind: 1, Payload: b}})
//	consumer, err := fitcore.OpenConsumer(cfg, protocol.Descriptor)
//	kind, payload, err := consumer.Receive()
//
// Context-taking variants cancel setup and idle receives without changing
// shared queue state.
package fitcore

import (
	"context"
	"errors"
	"fmt"
)

// ErrCancelled reports that a local operation was cancelled through its
// context. A cancelled receive reclaims no queue bytes; a cancelled setup
// releases every owned resource. The triggering context error is wrapped.
var ErrCancelled = errors.New("FIT operation cancelled")

func cancelled(cause error) error {
	if cause == nil {
		return ErrCancelled
	}
	return fmt.Errorf("%w: %w", ErrCancelled, cause)
}

func invalid(message string) error {
	return errors.New(message)
}

// Producer is one producer handle. It cannot receive records.
type Producer struct {
	id       uint64
	shared   *mapping
	protocol ProtocolDescriptor
}

// ConnectProducer connects to a consumer once and establishes a producer
// queue session. The consumer must already be listening.
func ConnectProducer(config ProducerConfig, protocol ProtocolDescriptor) (*Producer, error) {
	return connectProducer(nil, config, protocol)
}

// ConnectProducerContext connects while allowing the local supervisor to
// cancel setup through the context.
func ConnectProducerContext(ctx context.Context, config ProducerConfig, protocol ProtocolDescriptor) (*Producer, error) {
	return connectProducer(ctx, config, protocol)
}

// SessionID reports the established session identifier.
func (p *Producer) SessionID() uint64 { return p.id }

// SendBatch publishes the fitting prefix of records and reports notification
// failures after publication.
func (p *Producer) SendBatch(records []Record) (SendResult, error) {
	return p.shared.sendBatch(records, p.protocol, wakeWord)
}

// Close releases the shared mapping. Accepted-but-unread records are owned by
// the consumer; closing the producer does not discard them.
func (p *Producer) Close() error {
	if p.shared == nil {
		return nil
	}
	err := p.shared.close()
	p.shared = nil
	return err
}

// Consumer is one consumer handle. It cannot send records.
type Consumer struct {
	id       uint64
	shared   *mapping
	protocol ProtocolDescriptor
}

// OpenConsumer listens for one producer and initializes its shared queue.
func OpenConsumer(config ConsumerConfig, protocol ProtocolDescriptor) (*Consumer, error) {
	return openConsumer(nil, config, protocol)
}

// OpenConsumerContext opens while allowing the local supervisor to cancel
// setup through the context.
func OpenConsumerContext(ctx context.Context, config ConsumerConfig, protocol ProtocolDescriptor) (*Consumer, error) {
	return openConsumer(ctx, config, protocol)
}

// SessionID reports the established session identifier.
func (c *Consumer) SessionID() uint64 { return c.id }

// Receive waits on the shared write index when the queue is empty, then
// returns the next record's type and an owned copy of its payload. An idle
// receive stays asleep without polling.
func (c *Consumer) Receive() (kind uint32, payload []byte, err error) {
	k, data, cancelled, err := c.shared.receiveInner(c.protocol, nil)
	if cancelled {
		return 0, nil, invalid("uncancelled receive stopped")
	}
	return k, data, err
}

// ReceiveContext receives one record, or returns an error wrapping
// ErrCancelled after local cancellation without reclaiming a queued record.
// A record whose copy already started may still be delivered.
func (c *Consumer) ReceiveContext(ctx context.Context) (kind uint32, payload []byte, err error) {
	if ctx == nil {
		return c.Receive()
	}
	if cause := ctx.Err(); cause != nil {
		return 0, nil, cancelled(cause)
	}
	reg := newWaitRegistry()
	stop := watchContext(ctx, reg)
	defer stop()
	k, data, wasCancelled, err := c.shared.receiveInner(c.protocol, reg)
	if wasCancelled {
		return 0, nil, cancelled(ctx.Err())
	}
	return k, data, err
}

// Close releases the shared mapping. The shared-memory name is not re-unlinked:
// the consumer unlinked it during setup.
func (c *Consumer) Close() error {
	if c.shared == nil {
		return nil
	}
	err := c.shared.close()
	c.shared = nil
	return err
}
