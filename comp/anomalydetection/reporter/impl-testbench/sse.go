// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package testbenchimpl provides the testbench SSE reporter implementation.
package testbenchimpl

import (
	"sync"
)

// SSEEvent is a message sent to SSE clients.
type SSEEvent struct {
	Event string // "advance", "status"
	Data  []byte // JSON payload
}

// SSEAccess is implemented by the testbench reporter.
// The testbench HTTP API casts the injected reporter to this to register browser clients.
type SSEAccess interface {
	Subscribe() (*SSEClient, func())
	LatestStatus() []byte
	Broadcast(msg SSEEvent)
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
	clients      map[*SSEClient]struct{}
	latestStatus []byte // most recent status JSON, nil until first broadcast
}

// SSEClient represents one connected SSE consumer.
// StatusNotify is a 1-buffered channel used as a wakeup signal when new status
// is available. The actual payload is read from hub.LatestStatus under lock.
type SSEClient struct {
	Events       chan SSEEvent // buffered channel for ephemeral events
	StatusNotify chan struct{} // 1-buffered; signals new status available
}

func newSSEHub() *sseHub {
	return &sseHub{clients: make(map[*SSEClient]struct{})}
}

// Subscribe registers a new client. Returns the client (for reading) and an
// unsubscribe function. If a status has been broadcast previously, the client's
// StatusNotify channel is pre-signaled so the reader picks it up immediately.
func (h *sseHub) Subscribe() (c *SSEClient, unsubscribe func()) {
	c = &SSEClient{
		Events:       make(chan SSEEvent, 64),
		StatusNotify: make(chan struct{}, 1),
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	if h.latestStatus != nil {
		select {
		case c.StatusNotify <- struct{}{}:
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

// LatestStatus returns the current status payload.
func (h *sseHub) LatestStatus() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.latestStatus
}

// Broadcast sends an event to all connected clients.
func (h *sseHub) Broadcast(msg SSEEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if msg.Event == "status" {
		h.latestStatus = msg.Data
		for c := range h.clients {
			select {
			case c.StatusNotify <- struct{}{}:
			default:
			}
		}
	} else {
		for c := range h.clients {
			select {
			case c.Events <- msg:
			default:
			}
		}
	}
}
