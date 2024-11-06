// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"sync"
	"time"
)

// RegisterCacheEntry used to track the value
type RegisterCacheEntry struct {
	Pos   int
	Value interface{}
}

// Context describes the context used during a rule evaluation
type Context struct {
	Event Event

	// cache available across all the evaluations
	StringCache map[string][]string
	IntCache    map[string][]int
	BoolCache   map[string][]bool

	// iterator register cache. used to cache entry within a single rule evaluation
	RegisterCache map[RegisterID]*RegisterCacheEntry

	// rule register
	Registers map[RegisterID]int

	now time.Time
}

// Now return and cache the `now` timestamp
func (c *Context) Now() time.Time {
	if c.now.IsZero() {
		c.now = time.Now()
	}
	return c.now
}

// SetEvent set the given event to the context
func (c *Context) SetEvent(evt Event) {
	c.Event = evt
}

// Reset the context
func (c *Context) Reset() {
	c.Event = nil
	c.now = time.Time{}

	clear(c.StringCache)
	clear(c.IntCache)
	clear(c.BoolCache)
	clear(c.Registers)
	clear(c.RegisterCache)
}

// NewContext return a new Context
func NewContext(evt Event) *Context {
	return &Context{
		Event:         evt,
		StringCache:   make(map[string][]string),
		IntCache:      make(map[string][]int),
		BoolCache:     make(map[string][]bool),
		Registers:     make(map[RegisterID]int),
		RegisterCache: make(map[RegisterID]*RegisterCacheEntry),
	}
}

// ContextPool defines a pool of context
type ContextPool struct {
	pool sync.Pool
}

// Get returns a context with the given event
func (c *ContextPool) Get(evt Event) *Context {
	ctx := c.pool.Get().(*Context)
	ctx.SetEvent(evt)
	return ctx
}

// Put returns the context to the pool
func (c *ContextPool) Put(ctx *Context) {
	ctx.Reset()
	c.pool.Put(ctx)
}

// NewContextPool returns a new context pool
func NewContextPool() *ContextPool {
	return &ContextPool{
		pool: sync.Pool{
			New: func() interface{} { return NewContext(nil) },
		},
	}
}
