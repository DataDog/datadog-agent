// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

// Fanout duplicates each Payload to a fixed set of destination Senders,
// implementing PayloadSink. Each Sender has its own stream, inflight tracker,
// and snapshot; the shared *Payload must be treated as immutable after Submit.
// Submit blocks on each destination's queue in turn, so the encoder runs at the
// pace of the slowest destination. Lifecycle (Start/Stop) is driven by the
// owning StatefulOutput, not through the sink.
type Fanout struct {
	dests []*Sender
}

// NewFanout constructs a Fanout over the given destination Senders.
func NewFanout(dests []*Sender) *Fanout {
	return &Fanout{dests: dests}
}

// Submit duplicates the payload to every destination, blocking per destination.
// Returns a non-nil error only on shutdown.
func (f *Fanout) Submit(p *Payload) error {
	for _, d := range f.dests {
		if err := d.Submit(p); err != nil {
			return err
		}
	}
	return nil
}
