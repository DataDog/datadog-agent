// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package buf provides `BufferedChan` that is more efficient than `chan []interface{}`.
package buf

import (
	"context"
	"sync"
)

// BufferedChan behaves like a `chan []interface{}` (See thread safety for restrictions), but is
// most efficient as it uses internally a channel of []interface{}. This reduces the number of reads
// and writes to the channel. Instead of having one write and one read for each value for a regular channel,
// there are one write and one read for each `bufferSize` value.
// Thread safety:
//   - `BufferedChan.Put` cannot be called concurrently.
//   - `BufferedChan.Get` cannot be called concurrently.
//   - `BufferedChan.Put` can be called while another goroutine calls `BufferedChan.Get`.
type BufferedChan struct {
	c        chan []interface{}
	pool     *sync.Pool
	putSlice []interface{}
	getSlice []interface{}
	getIndex int
	ctx      context.Context
}

// NewBufferedChan creates a new instance of `BufferedChan`.
// `ctx` can be used to cancel all Put and Get operations.
func NewBufferedChan(ctx context.Context, chanSize int, bufferSize int) *BufferedChan {
	pool := &sync.Pool{
		New: func() interface{} {
			return make([]interface{}, 0, bufferSize)
		},
	}
	return &BufferedChan{
		c:        make(chan []interface{}, chanSize),
		pool:     pool,
		putSlice: pool.Get().([]interface{}),
		ctx:      ctx,
	}
}

// Put puts a new value into c.
// Cannot be called concurrently.
// Returns false when BufferedChan is cancelled.
func (c *BufferedChan) Put(value interface{}) bool {
	if cap(c.putSlice) <= len(c.putSlice) {
		select {
		case c.c <- c.putSlice:
		case <-c.ctx.Done():
			return false
		}
		c.putSlice = c.pool.Get().([]interface{})[:0]
	}
	c.putSlice = append(c.putSlice, value)
	return true
}

// Close flushes and closes the channel
func (c *BufferedChan) Close() {
	if len(c.putSlice) > 0 {
		c.c <- c.putSlice
	}
	close(c.c)
}

// Get gets the value and returns false when the channel is closed and all
//
//	values were read.
//
// Cannot be called concurrently.
func (c *BufferedChan) Get() (interface{}, bool) {
	if !c.WaitForValue() {
		return nil, false
	}
	value := c.getSlice[c.getIndex]
	c.getSlice[c.getIndex] = nil // do not keep a reference on the object.
	c.getIndex++
	return value, true
}

// WaitForValue waits until a value is available for Get or until Close is called or when the context is Done
// Returns true if a value is available, false otherwise
func (c *BufferedChan) WaitForValue() bool {
	if c.getIndex >= len(c.getSlice) {
		if c.getSlice != nil {
			c.pool.Put(c.getSlice[:0])
		}

		var ok bool
		select {
		case c.getSlice, ok = <-c.c:
			if !ok {
				return false
			}
		case <-c.ctx.Done():
			return false
		}
		c.getIndex = 0
		if len(c.getSlice) == 0 {
			return false
		}
	}
	return true
}
