// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build antithesis_demo

// Antithesis DORMANT deadlock demonstration for no-services-store-deadlock. Run:
//
//	go test -tags "antithesis_demo test" -run TestServicesStore_DormantDeadlockWithSlowSubscriber \
//	    ./pkg/logs/service/ -v -timeout 10s
//
// Demonstrates the latent deadlock in Services.AddService: the mutex is held
// across an unbuffered channel send to every registered subscriber. If a
// subscriber goroutine stops consuming (paused, slow, or blocked), AddService
// blocks indefinitely while holding s.mu — stalling every concurrent caller.
//
// VERDICT: DORMANT — no production code calls GetAllAddedServices or
// GetAddedServicesForType, so the subscriber slice is always empty and no
// blocking send is ever attempted. The code path is unreachable in production.
//
// This test CREATES a subscriber (mimicking what a future launcher would do)
// and then deliberately stalls it. It asserts that AddService blocks and that a
// concurrent AddService call cannot acquire the lock within the timeout.
// EXPECTED TO DEMONSTRATE THE LATENT DEADLOCK (test will PASS, proving blockage).

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestServicesStore_DormantDeadlockWithSlowSubscriber proves the latent deadlock:
// a slow subscriber causes AddService to hold s.mu while blocked on a channel
// send, preventing any concurrent AddService call from making progress.
func TestServicesStore_DormantDeadlockWithSlowSubscriber(t *testing.T) {
	svc := NewServices()

	// Register a subscriber via GetAllAddedServices (the API that would deadlock).
	// No production caller exists today — this is what would trigger the bug
	// if a new launcher subscribed.
	subCh := svc.GetAllAddedServices()

	// doneCh is closed when the first AddService completes.
	doneCh := make(chan struct{})

	// Launch the first AddService call. Because subCh is unbuffered and the
	// subscriber (below) will be paused, this goroutine will block inside
	// AddService while holding s.mu.
	go func() {
		svc.AddService(&Service{Type: "docker", Identifier: "container-1"})
		close(doneCh)
	}()

	// Give the goroutine a moment to enter AddService and block on the send.
	time.Sleep(20 * time.Millisecond)

	// Now attempt a second AddService call. It must acquire s.mu to proceed,
	// but the first goroutine is holding it blocked on `ch <- service`.
	// This second call should NOT complete within the timeout.
	secondDone := make(chan struct{})
	go func() {
		svc.AddService(&Service{Type: "docker", Identifier: "container-2"})
		close(secondDone)
	}()

	// Neither the first nor second AddService should complete within 200ms —
	// both are blocked because no one is draining subCh.
	select {
	case <-secondDone:
		// If we get here quickly it means the deadlock was NOT triggered, which
		// would be surprising — but not impossible if the subscriber send was
		// buffered. Fail to draw attention.
		t.Error("second AddService completed despite slow subscriber — deadlock not demonstrated")
	case <-time.After(200 * time.Millisecond):
		// Expected: second goroutine is blocked. Deadlock demonstrated.
		t.Log("DORMANT DEADLOCK DEMONSTRATED: AddService blocked holding s.mu; " +
			"second AddService cannot acquire lock — no subscriber draining subCh. " +
			"This is latent: no production caller currently subscribes.")
	}

	// Drain the subscriber channel to unblock both goroutines and let the test
	// finish cleanly.
	go func() {
		<-subCh
		<-subCh
	}()

	// Both goroutines should now complete.
	select {
	case <-doneCh:
	case <-time.After(2 * time.Second):
		t.Fatal("first AddService goroutine did not complete after unblocking subscriber")
	}
	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("second AddService goroutine did not complete after unblocking subscriber")
	}

	assert.Len(t, svc.services, 2, "both services should be registered")
}
