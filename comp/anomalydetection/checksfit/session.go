// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checksfit

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/checksfit/fitcore"
)

// SendOutcome reports a typed send. QueueRejection takes precedence: when it is
// not fitcore.RejectionNone, every offered record was encoded but the queue
// stopped accepting after Accepted, and EncodingError is nil. Otherwise
// EncodingError is the first encoding failure, reported after the encoded
// prefix was published. NotificationError is independent: accepted records stay
// published when a wake fails, so they must not be retried.
type SendOutcome struct {
	Accepted          int
	QueueRejection    fitcore.Rejection
	EncodingError     error
	NotificationError error
}

// Producer is a DDCHECKS typed producer handle.
type Producer struct {
	inner *fitcore.Producer
}

// Connect connects to a consumer using the DDCHECKS descriptor.
func Connect(config fitcore.ProducerConfig) (*Producer, error) {
	inner, err := fitcore.ConnectProducer(config, Descriptor)
	if err != nil {
		return nil, err
	}
	return &Producer{inner: inner}, nil
}

// ConnectContext connects while allowing the local supervisor to cancel setup
// through the context.
func ConnectContext(ctx context.Context, config fitcore.ProducerConfig) (*Producer, error) {
	inner, err := fitcore.ConnectProducerContext(ctx, config, Descriptor)
	if err != nil {
		return nil, err
	}
	return &Producer{inner: inner}, nil
}

// SessionID reports the established session identifier.
func (p *Producer) SessionID() uint64 { return p.inner.SessionID() }

// Close releases the shared mapping. Accepted-but-unread records stay owned by
// the consumer.
func (p *Producer) Close() error { return p.inner.Close() }

// SendMetrics encodes the metrics in order, then publishes at most the encoded
// prefix in one ring batch. The first encoding failure is reported after any
// preceding fitting prefix was published.
func (p *Producer) SendMetrics(metrics []Metric) (SendOutcome, error) {
	records := make([]fitcore.Record, 0, len(metrics))
	var encodeErr error
	for _, metric := range metrics {
		payload, err := metric.Encode()
		if err != nil {
			encodeErr = err
			break
		}
		records = append(records, fitcore.Record{Kind: TypeMetric, Payload: payload})
	}
	result, err := p.inner.SendBatch(records)
	if err != nil {
		return SendOutcome{}, err
	}
	outcome := SendOutcome{
		Accepted:          result.Accepted,
		QueueRejection:    result.Rejection,
		NotificationError: result.NotificationError,
	}
	if outcome.QueueRejection == fitcore.RejectionNone {
		outcome.EncodingError = encodeErr
	}
	return outcome, nil
}

// Consumer is a DDCHECKS typed consumer handle.
type Consumer struct {
	inner *fitcore.Consumer
}

// Open listens for one producer using the DDCHECKS descriptor.
func Open(config fitcore.ConsumerConfig) (*Consumer, error) {
	inner, err := fitcore.OpenConsumer(config, Descriptor)
	if err != nil {
		return nil, err
	}
	return &Consumer{inner: inner}, nil
}

// OpenContext opens while allowing the local supervisor to cancel setup through
// the context.
func OpenContext(ctx context.Context, config fitcore.ConsumerConfig) (*Consumer, error) {
	inner, err := fitcore.OpenConsumerContext(ctx, config, Descriptor)
	if err != nil {
		return nil, err
	}
	return &Consumer{inner: inner}, nil
}

// SessionID reports the established session identifier.
func (c *Consumer) SessionID() uint64 { return c.inner.SessionID() }

// Close releases the shared mapping.
func (c *Consumer) Close() error { return c.inner.Close() }

// Receive waits for one record and decodes it as a metric.
func (c *Consumer) Receive() (Metric, error) {
	kind, payload, err := c.inner.Receive()
	if err != nil {
		return Metric{}, err
	}
	return decodeMetric(kind, payload)
}

// ReceiveContext receives one record, or returns an error wrapping
// fitcore.ErrCancelled after local cancellation without consuming a queued
// record.
func (c *Consumer) ReceiveContext(ctx context.Context) (Metric, error) {
	kind, payload, err := c.inner.ReceiveContext(ctx)
	if err != nil {
		return Metric{}, err
	}
	return decodeMetric(kind, payload)
}

func decodeMetric(kind uint32, payload []byte) (Metric, error) {
	if kind != TypeMetric {
		return Metric{}, fmt.Errorf("unexpected DDCHECKS record type %d, want metric %d", kind, TypeMetric)
	}
	return DecodeMetric(payload)
}
