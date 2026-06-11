// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package replayimpl

import (
	"sync"

	"go.uber.org/atomic"

	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
)

type ingressRing struct {
	mu       sync.Mutex
	entries  []replay.IngressEnvelope
	next     int
	retained int
	dropped  *atomic.Uint64
}

func newIngressRing(capacity int) *ingressRing {
	if capacity <= 0 {
		return nil
	}
	return &ingressRing{
		entries: make([]replay.IngressEnvelope, capacity),
		dropped: atomic.NewUint64(0),
	}
}

func (r *ingressRing) append(envelope replay.IngressEnvelope) bool {
	if r == nil || len(r.entries) == 0 {
		return false
	}

	copyEnvelope := cloneIngressEnvelope(envelope)

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.retained == len(r.entries) {
		r.dropped.Inc()
	} else {
		r.retained++
	}
	r.entries[r.next] = copyEnvelope
	r.next = (r.next + 1) % len(r.entries)
	return true
}

func (r *ingressRing) snapshot(max int) []replay.IngressEnvelope {
	if r == nil || len(r.entries) == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	count := r.retained
	if max > 0 && max < count {
		count = max
	}
	if count == 0 {
		return nil
	}

	out := make([]replay.IngressEnvelope, 0, count)
	start := r.next - r.retained
	if start < 0 {
		start += len(r.entries)
	}
	if max > 0 && max < r.retained {
		start = r.next - max
		if start < 0 {
			start += len(r.entries)
		}
	}
	for i := 0; i < count; i++ {
		idx := (start + i) % len(r.entries)
		out = append(out, cloneIngressEnvelope(r.entries[idx]))
	}
	return out
}

func (r *ingressRing) stats() replay.IngressStats {
	if r == nil {
		return replay.IngressStats{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return replay.IngressStats{
		Capacity: len(r.entries),
		Retained: r.retained,
		Dropped:  r.dropped.Load(),
	}
}

func cloneIngressEnvelope(envelope replay.IngressEnvelope) replay.IngressEnvelope {
	copyEnvelope := envelope
	copyEnvelope.Payload = append([]byte(nil), envelope.Payload...)
	copyEnvelope.Ancillary = append([]byte(nil), envelope.Ancillary...)
	return copyEnvelope
}
