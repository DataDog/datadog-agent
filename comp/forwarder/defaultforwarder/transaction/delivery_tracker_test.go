// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package transaction

import (
	"context"
	"errors"
	"testing"
)

func TestTrackedDeliveryRequiresEveryFinalResponse(t *testing.T) {
	tracker := NewDeliveryTracker()
	first, second := NewHTTPTransaction(), NewHTTPTransaction()
	tracker.Track(first)
	tracker.Track(second)
	first.CompletionHandler(first, 202, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tracker.Wait(ctx); err == nil {
		t.Fatal("partial delivery was reported successful")
	}
	second.CompletionHandler(second, 200, nil, nil)
	second.CompletionHandler(second, 200, nil, nil)
	if err := tracker.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	expected, accepted := tracker.Counts()
	if expected != 2 || accepted != 2 {
		t.Fatal("duplicate completion corrupted accounting")
	}
}

func TestDroppedAndRejectedDeliveryFails(t *testing.T) {
	for _, complete := range []func(*HTTPTransaction){func(tx *HTTPTransaction) { tx.CompletionHandler(tx, 403, nil, nil) }, func(tx *HTTPTransaction) { ReportDeliveryFailure(tx, errors.New("queue full")) }} {
		tracker := NewDeliveryTracker()
		tx := NewHTTPTransaction()
		tracker.Track(tx)
		complete(tx)
		if err := tracker.Wait(context.Background()); err == nil {
			t.Fatal("failed payload was reported as delivered")
		}
		_, accepted := tracker.Counts()
		if accepted != 0 {
			t.Fatal("failed payload counted as accepted")
		}
	}
}
