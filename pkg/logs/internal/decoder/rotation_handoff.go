// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"sync"
	"time"
)

// RotationHandoff transfers the partially aggregated content held by the
// decoder of a rotated-away file to the decoder that takes over the new file,
// so a log group straddling the rotation is completed rather than split.
//
// Ownership of the content is explicit and is only ever held by one side: the
// sender parks it, the receiver claims it. Every path that does not end in a
// claim hands the content back to the sender, which then emits it exactly as it
// would have without a handoff. The handoff can only improve on that baseline,
// never lose content.
type RotationHandoff struct {
	mu sync.Mutex
	// state is the parked content, owned by the handoff between park and claim.
	state *PendingState
	// parked is closed once the sender has parked (the content may be nil, which
	// tells the receiver there is nothing to wait for).
	parked chan struct{}
	// hasParked mirrors the closed state of parked under the mutex.
	hasParked bool
	// receiving is set once the receiver is in its wait loop. From that point on
	// it is guaranteed to claim, which is what makes parking safe for a sender
	// that is about to shut down.
	receiving bool
	// settled is set by whichever side resolves the transfer first.
	settled bool
	// claimed records that the receiver took ownership.
	claimed  bool
	deadline time.Time
}

// NewRotationHandoff returns a handoff that expires window from now.
func NewRotationHandoff(window time.Duration) *RotationHandoff {
	return &RotationHandoff{
		parked:   make(chan struct{}),
		deadline: time.Now().Add(window),
	}
}

// Deadline is when the receiver stops holding the new file's content back and
// the sender takes any unclaimed content back to emit it itself.
func (h *RotationHandoff) Deadline() time.Time {
	return h.deadline
}

// Parked is closed once the sender has parked its content.
func (h *RotationHandoff) Parked() <-chan struct{} {
	return h.parked
}

// Cancel settles the handoff without a transfer. Used when the replacement
// tailer fails to start: the sender keeps (or takes back) its content.
func (h *RotationHandoff) Cancel() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.settled = true
}

// beginReceive records that the receiver is waiting. It reports false when the
// handoff is already settled, in which case there is nothing to wait for.
func (h *RotationHandoff) beginReceive() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.settled {
		return false
	}
	h.receiving = true
	return true
}

// park offers content to the receiver. It reports whether ownership was
// transferred; when it returns false the sender still owns state and must emit
// it itself.
//
// final must be set by a sender that will not be able to take the content back
// later (it is shutting down). Parking is then only allowed once the receiver
// is known to be waiting, since only then is a claim guaranteed.
func (h *RotationHandoff) park(state *PendingState, final bool) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.settled || h.hasParked {
		return false
	}
	if final && !h.receiving {
		return false
	}
	h.state = state
	h.hasParked = true
	close(h.parked)
	return true
}

// claim takes ownership of whatever the sender parked and settles the handoff.
// It reports false when there was nothing parked, which also settles the
// handoff so a later park is refused and the sender keeps its content.
func (h *RotationHandoff) claim() (*PendingState, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.settled || !h.hasParked {
		h.settled = true
		return nil, false
	}
	h.settled = true
	h.claimed = true
	state := h.state
	h.state = nil
	return state, true
}

// reclaim gives parked content back to the sender when the receiver never took
// it. It reports false when the receiver already claimed it.
func (h *RotationHandoff) reclaim() (*PendingState, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.claimed {
		return nil, false
	}
	h.settled = true
	state := h.state
	h.state = nil
	return state, true
}
