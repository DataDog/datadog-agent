// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package transaction

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// DeliveryTracker accounts for final delivery of explicitly tracked requests.
// It does not change retry behavior. Install it before starting a forwarder.
// Call Wait only after producers have finished registering the work to drain.
type DeliveryTracker struct {
	mu         sync.Mutex
	pending    int
	expected   int
	accepted   int
	firstError error
	changed    chan struct{}
}

func NewDeliveryTracker() *DeliveryTracker { return &DeliveryTracker{changed: make(chan struct{})} }

// Register returns an idempotent final-completion callback.
func (t *DeliveryTracker) Register() func(error) {
	t.mu.Lock()
	t.pending++
	t.expected++
	t.mu.Unlock()
	var once sync.Once
	return func(err error) {
		once.Do(func() {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.pending--
			if err != nil {
				if t.firstError == nil {
					t.firstError = err
				}
			} else {
				t.accepted++
			}
			close(t.changed)
			t.changed = make(chan struct{})
		})
	}
}

// Track preserves existing response handling and observes its final result.
func (t *DeliveryTracker) Track(tx *HTTPTransaction) {
	done := t.Register()
	previous := tx.CompletionHandler
	var once sync.Once
	tx.CompletionHandler = func(tx *HTTPTransaction, status int, body []byte, err error) {
		once.Do(func() {
			if err != nil {
				done(errors.New("delivery failed"))
			} else if status < 200 || status >= 300 {
				done(fmt.Errorf("delivery rejected with HTTP %d", status))
			} else {
				done(nil)
			}
			if previous != nil {
				previous(tx, status, body, err)
			}
		})
	}
	tx.DeliveryFailure = func(err error) { tx.CompletionHandler(tx, 0, nil, err) }
}

// ReportDeliveryFailure notifies explicitly tracked requests when a queue drops
// them. Ordinary transactions and non-HTTP transaction types are unaffected.
func ReportDeliveryFailure(tx Transaction, err error) {
	if httpTx, ok := tx.(*HTTPTransaction); ok && httpTx.DeliveryFailure != nil {
		httpTx.DeliveryFailure(err)
	}
}

func (t *DeliveryTracker) Wait(ctx context.Context) error {
	for {
		t.mu.Lock()
		pending, err, changed := t.pending, t.firstError, t.changed
		t.mu.Unlock()
		if err != nil {
			return err
		}
		if pending == 0 {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return fmt.Errorf("%d payloads did not complete delivery: %w", pending, ctx.Err())
		}
	}
}

func (t *DeliveryTracker) Counts() (expected, accepted int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.expected, t.accepted
}
