// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"context"
	"sync"
)

// runMu keeps pool cleanup and collection from using the check at the same time.
// mu protects the cancellation signal, so the hooks never wait for database I/O.
type checkLifecycle struct {
	runMu   sync.Mutex
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	running bool
	retired bool
	done    chan struct{}
}

func newCheckLifecycle() *checkLifecycle {
	ctx, cancel := context.WithCancel(context.Background())
	return &checkLifecycle{ctx: ctx, cancel: cancel, done: make(chan struct{})}
}

func (c *Check) operationContext() context.Context {
	// Some SQL helper tests construct an unconfigured Check. Scheduled checks
	// always initialize lifecycle in Configure or newCheck before use.
	if c.lifecycle == nil {
		return context.Background()
	}
	c.lifecycle.mu.Lock()
	defer c.lifecycle.mu.Unlock()
	if !c.lifecycle.running && !c.lifecycle.retired {
		return context.Background()
	}
	return c.lifecycle.ctx
}

// Run executes one cycle. Only the run owner accesses pools while it is active.
func (c *Check) Run() (err error) {
	l := c.lifecycle
	l.runMu.Lock()
	defer l.runMu.Unlock()
	l.mu.Lock()
	if l.retired {
		l.mu.Unlock()
		return context.Canceled
	}
	l.cancel()
	ctx, cancel := context.WithCancel(context.Background())
	l.ctx, l.cancel, l.running = ctx, cancel, true
	l.mu.Unlock()
	defer func() {
		l.mu.Lock()
		ctxErr := ctx.Err()
		l.running = false
		l.mu.Unlock()
		if ctxErr != nil {
			err = ctxErr
			c.closePools()
		}
		cancel()
	}()
	return c.run(ctx)
}

// Stop interrupts the active cycle. A later scheduled cycle may run again.
func (c *Check) Stop() {
	if c.lifecycle == nil {
		return
	}
	l := c.lifecycle
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.running {
		l.cancel()
	}
}

// Cancel retires an unscheduled check, including when it is idle. Cleanup is
// serialized with Run, but database logoff must not block the collector hook.
func (c *Check) Cancel() {
	if c.lifecycle == nil {
		return
	}
	l := c.lifecycle
	l.mu.Lock()
	if l.retired {
		l.mu.Unlock()
		return
	}
	l.retired = true
	l.cancel()
	l.mu.Unlock()
	go func() {
		l.runMu.Lock()
		defer l.runMu.Unlock()
		defer close(l.done)
		c.closePools()
	}()
}

// Teardown is synchronous cleanup for callers that are not running collection.
// Stop and Cancel signal cancellation before scheduling cleanup instead.
func (c *Check) Teardown() {
	if c.lifecycle != nil {
		c.lifecycle.runMu.Lock()
		defer c.lifecycle.runMu.Unlock()
	}
	c.closePools()
}

func (c *Check) closePools() {
	closeDatabase(c, c.db)
	c.db = nil
	closeDatabase(c, c.dbCustomQueries)
	c.dbCustomQueries = nil
}
