// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package eventbuf

import (
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

// SharedMessage refcounts a single underlying Message so it can be attached
// to multiple Readys. NotePanicUnwoundRange uses this to fan one synthetic
// recovery event out across N user-probe finalizations.
//
// Lifecycle:
//   - NewSharedMessage(underlying) returns a SharedMessage holding one
//     base reference.
//   - For each fanout target, Acquire() takes an additional reference and
//     returns a handle that satisfies the Message interface.
//   - ReleaseBase() drops the base reference. Call exactly once when no
//     further Acquires will be made.
//   - Each handle's Release() drops its reference.
//
// The underlying Message is released when the last reference (base or
// handle) is dropped. All Acquires must happen before ReleaseBase;
// individual handle Releases may interleave freely with ReleaseBase
// and with each other.
type SharedMessage struct {
	underlying Message
	refs       atomic.Int32
}

// NewSharedMessage wraps underlying. The returned SharedMessage holds one
// base reference that must be dropped via ReleaseBase exactly once.
func NewSharedMessage(underlying Message) *SharedMessage {
	s := &SharedMessage{underlying: underlying}
	s.refs.Store(1)
	return s
}

// Acquire returns a handle holding one reference on the underlying message.
// Each handle is its own Message; calling Release on it decrements the
// shared refcount.
func (s *SharedMessage) Acquire() Message {
	s.refs.Add(1)
	return sharedMessageHandle{shared: s}
}

// ReleaseBase drops the base reference taken by NewSharedMessage. Call
// exactly once. The underlying Message is released by whichever caller
// performs the final decrement (this one, or the last handle's Release).
func (s *SharedMessage) ReleaseBase() {
	s.release()
}

func (s *SharedMessage) release() {
	if s.refs.Add(-1) != 0 {
		return
	}
	s.underlying.Release()
}

// sharedMessageHandle is one reference into a SharedMessage. Each handle
// implements Message in its own right; the buffer doesn't need to know it's
// a shared payload.
type sharedMessageHandle struct {
	shared *SharedMessage
}

// Event returns the shared underlying event bytes. The buffer reads this
// to determine fragment length; all handles return the same slice.
func (h sharedMessageHandle) Event() output.Event {
	return h.shared.underlying.Event()
}

// Release decrements the shared refcount; the underlying message is
// released when the last handle releases.
func (h sharedMessageHandle) Release() {
	h.shared.release()
}
