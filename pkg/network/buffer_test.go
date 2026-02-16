// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func makeConn(pid uint32) ConnectionStats {
	var c ConnectionStats
	c.Pid = pid
	return c
}

func TestConnectionBufferAssign(t *testing.T) {
	buf := NewConnectionBuffer(0, 0)
	slice := []ConnectionStats{makeConn(1), makeConn(2)}
	buf.Assign(slice)
	assert.Equal(t, 2, buf.Len())
	conns := buf.Connections()
	assert.Equal(t, uint32(1), conns[0].Pid)
	assert.Equal(t, uint32(2), conns[1].Pid)
}

func TestConnectionBufferAppend(t *testing.T) {
	buf := NewConnectionBuffer(2, 2)
	c := buf.Next()
	c.Pid = 1

	buf.Append([]ConnectionStats{makeConn(2), makeConn(3)})
	assert.Equal(t, 3, buf.Len())
	conns := buf.Connections()
	assert.Equal(t, uint32(1), conns[0].Pid)
	assert.Equal(t, uint32(2), conns[1].Pid)
	assert.Equal(t, uint32(3), conns[2].Pid)
}

func TestConnectionBufferReclaim(t *testing.T) {
	buf := NewConnectionBuffer(4, 2)
	for i := 0; i < 3; i++ {
		c := buf.Next()
		c.Pid = uint32(i)
	}
	assert.Equal(t, 3, buf.Len())

	buf.Reclaim(1)
	assert.Equal(t, 2, buf.Len())

	// reclaim more than available clamps to 0
	buf.Reclaim(100)
	assert.Equal(t, 0, buf.Len())
}

func TestConnectionBufferConnections(t *testing.T) {
	buf := NewConnectionBuffer(4, 2)
	for i := 0; i < 3; i++ {
		c := buf.Next()
		c.Pid = uint32(i + 10)
	}
	conns := buf.Connections()
	assert.Len(t, conns, 3)
	assert.Equal(t, uint32(10), conns[0].Pid)
	assert.Equal(t, uint32(11), conns[1].Pid)
	assert.Equal(t, uint32(12), conns[2].Pid)
}

func BenchmarkBuffer(b *testing.B) {
	var buffer *ConnectionBuffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer := NewConnectionBuffer(256, 256)
		for i := 0; i < 512; i++ {
			conn := buffer.Next()
			conn.Pid = uint32(i)
		}
	}
	runtime.KeepAlive(buffer)
}
