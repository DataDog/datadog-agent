// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`) at commit
// 4961722de9009afdbbb711fc0adf14a8f6ff9277 (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the darwin build requires cgo, unsupported platforms get a
// stub so this tree still compiles, and test files carry an explicit platform
// gate. The transport logic, framing, and ring layout are unchanged.

package fitcore

import (
	"context"
	"errors"
	"sync"
	"time"
)

// errSecondWaiter reports a second concurrent registration on one registry;
// one receive call supports one active shared-memory wait.
var errSecondWaiter = errors.New("cancellation token is already used by a receiver")

// waitRegistry coordinates one cancellable receive call. It mirrors the Rust
// CancellationToken semantics: a registered wait address is woken on
// cancellation, and a wake that races the entry into the native wait is
// retried until the receiver unregisters. One registry supports at most one
// active shared-memory wait, the same way one token supports one receiver.
type waitRegistry struct {
	mu        sync.Mutex
	cancelled bool
	waiting   *uint32
	waitDone  chan struct{}
}

func newWaitRegistry() *waitRegistry {
	return &waitRegistry{}
}

// check reports whether the operation was already cancelled.
func (r *waitRegistry) check() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancelled
}

// cancel marks the registry cancelled and wakes any registered wait word.
// The wake runs under the registry mutex, mirroring the Rust token: once the
// receiver has unregistered (also under the mutex), no later wake can
// reference its word, so a concurrently closing handle cannot unmap the
// shared region beneath the wake. A wake just before the receiver enters
// its native wait could be missed, so it is retried until the receiver
// unregisters. It returns the first wake failure; the registry stays
// cancelled and cancel can be retried.
func (r *waitRegistry) cancel() error {
	r.mu.Lock()
	r.cancelled = true
	for {
		word, done := r.waiting, r.waitDone
		if word == nil {
			r.mu.Unlock()
			return nil
		}
		// Holding the mutex across the wake keeps the word registered for the
		// whole call; waitWord never takes this mutex, so the blocked receiver
		// cannot deadlock against this critical section.
		err := wakeWord(word)
		r.mu.Unlock()
		if err != nil {
			return err
		}
		// Give the woken receiver a chance to unregister before re-checking;
		// release the mutex while waiting so unregistering can proceed, and a
		// fresh registration replaces the old one.
		select {
		case <-done:
		case <-time.After(10 * time.Millisecond):
		}
		r.mu.Lock()
	}
}

// waitOn registers word, performs the native wait with expected, and
// unregisters. It returns proceed=false when the operation was cancelled
// before or during the wait, without changing shared queue state.
func (r *waitRegistry) waitOn(word *uint32, expected uint32) (proceed bool, err error) {
	return r.waitOnWith(word, expected, waitWord)
}

// waitOnWith is waitOn with an injectable wait function, so tests can hold
// the receiver between registration and its native wait, exactly like the
// Rust token's wait_on_with.
func (r *waitRegistry) waitOnWith(word *uint32, expected uint32, waitFn func(*uint32, uint32) error) (proceed bool, err error) {
	r.mu.Lock()
	if r.cancelled {
		r.mu.Unlock()
		return false, nil
	}
	if r.waiting != nil {
		r.mu.Unlock()
		return false, errSecondWaiter
	}
	done := make(chan struct{})
	r.waiting = word
	r.waitDone = done
	r.mu.Unlock()

	waitErr := waitFn(word, expected)

	r.mu.Lock()
	r.waiting = nil
	r.waitDone = nil
	close(done)
	cancelled := r.cancelled
	r.mu.Unlock()
	if cancelled {
		return false, nil
	}
	return true, waitErr
}

// watchContext cancels the registry when the context fires. The returned
// stop function must be called when the receive ends. A wake failure is
// retried the way a repeated token cancel retries, because the blocked
// native wait is the only thing keeping the receive alive.
func watchContext(ctx context.Context, reg *waitRegistry) (stop func()) {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			for {
				if err := reg.cancel(); err == nil {
					return
				}
				select {
				case <-done:
					return
				case <-time.After(10 * time.Millisecond):
				}
			}
		case <-done:
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() { close(done) })
	}
}
