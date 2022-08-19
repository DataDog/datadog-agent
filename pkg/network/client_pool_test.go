// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientBufferPool(t *testing.T) {
	pool := &clientBufferPool{
		bufferByClient: make(map[string]*clientBuffer),
	}

	buffer := pool.Get("client_id")

	// Add twice the elements the buffer originally supports
	assert.Equal(t, 0, buffer.connsBuf.Len())
	assert.Equal(t, defaultClientBufferSize, buffer.connsBuf.Capacity())
	for i := 0; i < 2*defaultClientBufferSize; i++ {
		buffer.connsBuf.Next().Pid = uint32(i)
	}
	assert.Equal(t, 2*defaultClientBufferSize, buffer.connsBuf.Len())
	increasedCapacity := buffer.connsBuf.Capacity()

	// Now we return the buffer and retrieve it again
	pool.Put(buffer)
	buffer = pool.Get("client_id")

	// Buffer length has to be 0, as buffers are cleared when returned to the pool
	assert.Equal(t, 0, buffer.connsBuf.Len())
	// Capacity should be retained
	assert.Equal(t, increasedCapacity, buffer.connsBuf.Capacity())
}
