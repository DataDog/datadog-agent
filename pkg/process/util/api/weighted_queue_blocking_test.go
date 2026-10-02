// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package api

import (
	"context"
	"testing"
	"time"
)

func TestBlockingQueuePreservesEveryItem(t *testing.T) {
	q := NewWeightedQueue(1, 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 100; i++ {
			if err := q.AddBlocking(ctx, newItem("item", 1)); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for i := 0; i < 100; i++ {
		if _, ok := q.Poll(); !ok {
			t.Fatal("queue truncated before all items arrived")
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestBlockingQueueCancellationAndOversize(t *testing.T) {
	q := NewWeightedQueue(1, 1)
	q.Add(newItem("first", 1))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := q.AddBlocking(ctx, newItem("second", 1)); err == nil {
		t.Fatal("ignored cancellation")
	}
	item, ok := q.Poll()
	if !ok || item.Type() != "first" {
		t.Fatal("evicted an existing payload")
	}
	if err := q.AddBlocking(context.Background(), newItem("oversized", 2)); err == nil {
		t.Fatal("accepted an item that cannot fit")
	}
	q.Stop()
	if err := q.AddBlocking(context.Background(), newItem("stopped", 1)); err == nil {
		t.Fatal("accepted a payload after shutdown")
	}
}
