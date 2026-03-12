// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sync"
)

// sseEvent is a message sent to SSE clients.
type sseEvent struct {
	Event string // "status", "progress", "heartbeat"
	Data  []byte // JSON payload
}

// sseHub manages a set of SSE client channels and broadcasts events to all of them.
//
// Status events are stored separately from the buffered channel so that:
//   - New subscribers always receive the latest status immediately.
//   - A slow client's full channel cannot block the broadcaster.
//   - Status is never lost — readers always get the latest value on wakeup.
//
// Ephemeral events (progress, heartbeat) go through the buffered channel with
// non-blocking sends; slow clients skip them, which is acceptable.
type sseHub struct {
	mu           sync.Mutex
	clients      map[*sseClient]struct{}
	latestStatus []byte // most recent status JSON, nil until first broadcast
}

// sseClient represents one connected SSE consumer.
// statusNotify is a 1-buffered channel used as a wakeup signal when new status
// is available. The actual payload is read from hub.latestStatus under lock.
type sseClient struct {
	events       chan sseEvent // buffered channel for ephemeral events
	statusNotify chan struct{} // 1-buffered; signals new status available
}

func newSSEHub() *sseHub {
	return &sseHub{clients: make(map[*sseClient]struct{})}
}

// subscribe registers a new client. Returns the client (for reading) and an
// unsubscribe function. If a status has been broadcast previously, the client's
// statusNotify channel is pre-signaled so the reader picks it up immediately.
func (h *sseHub) subscribe() (c *sseClient, unsubscribe func()) {
	c = &sseClient{
		events:       make(chan sseEvent, 64),
		statusNotify: make(chan struct{}, 1),
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	if h.latestStatus != nil {
		// Signal so reader picks up initial status on first select.
		select {
		case c.statusNotify <- struct{}{}:
		default:
		}
	}
	h.mu.Unlock()
	return c, func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
	}
}

// latestStatusData returns the current status payload. Safe to call from the
// SSE handler's read loop after receiving a statusNotify signal.
func (h *sseHub) latestStatusData() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.latestStatus
}

// broadcast sends an event to all connected clients.
//
// Status events: stored in latestStatus, then each client's statusNotify is
// signaled (non-blocking). The reader goroutine picks up the payload from
// latestStatus, so delivery is guaranteed and never blocks the broadcaster.
//
// Other events: non-blocking send to the buffered channel; slow clients skip.
func (h *sseHub) broadcast(msg sseEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if msg.Event == "status" {
		h.latestStatus = msg.Data
		for c := range h.clients {
			select {
			case c.statusNotify <- struct{}{}:
			default:
				// Already signaled, reader will pick up latest.
			}
		}
	} else {
		for c := range h.clients {
			select {
			case c.events <- msg:
			default:
			}
		}
	}
}
