// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import "sync"

// ClientPool holds a ConnectionBuffer object per client
var ClientPool *clientBufferPool

const defaultClientBufferSize = 1024

// ClientBuffer amortizes the allocations of objects generated when a client
// calls `GetConnections`.
type ClientBuffer struct {
	clientID string
	*ConnectionBuffer
	// TODO: consider recycling objects for HTTP and DNS data as well
}

type clientBufferPool struct {
	mux            sync.Mutex
	bufferByClient map[string]*ClientBuffer
}

func (p *clientBufferPool) Get(clientID string) *ClientBuffer {
	p.mux.Lock()
	defer p.mux.Unlock()

	buffer := p.bufferByClient[clientID]
	if buffer != nil {
		p.bufferByClient[clientID] = nil
		return buffer
	}

	return &ClientBuffer{
		clientID:         clientID,
		ConnectionBuffer: NewConnectionBuffer(defaultClientBufferSize, 256),
	}
}

func (p *clientBufferPool) Put(b *ClientBuffer) {
	p.mux.Lock()
	defer p.mux.Unlock()

	b.Reset()
	p.bufferByClient[b.clientID] = b
}

func (p *clientBufferPool) RemoveExpiredClient(clientID string) {
	p.mux.Lock()
	defer p.mux.Unlock()
	delete(p.bufferByClient, clientID)
}

// Reclaim memory from the `Connections` underlying buffer
func Reclaim(c *Connections) {
	b := c.buffer
	if b == nil {
		return
	}

	ClientPool.Put(b)
	c.buffer = nil
}

func init() {
	ClientPool = &clientBufferPool{
		bufferByClient: make(map[string]*ClientBuffer),
	}
}
