// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import "sync"

var clientPool *clientBufferPool

const defaultClientBufferSize = 1024

// clientBuffer amortizes the allocations of objects generated when a client
// calls `GetConnections`.
type clientBuffer struct {
	clientID string
	*ConnectionBuffer
	// TODO: consider recycling objects for HTTP and DNS data as well
}

type clientBufferPool struct {
	mux            sync.Mutex
	bufferByClient map[string]*clientBuffer
}

func (p *clientBufferPool) Get(clientID string) *clientBuffer {
	p.mux.Lock()
	defer p.mux.Unlock()

	buffer := p.bufferByClient[clientID]
	if buffer != nil {
		p.bufferByClient[clientID] = nil
		return buffer
	}

	return &clientBuffer{
		clientID:         clientID,
		ConnectionBuffer: NewConnectionBuffer(defaultClientBufferSize, 256),
	}
}

func (p *clientBufferPool) Put(b *clientBuffer) {
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

	clientPool.Put(b)
	c.buffer = nil
}

func init() {
	clientPool = &clientBufferPool{
		bufferByClient: make(map[string]*clientBuffer),
	}
}
