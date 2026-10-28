// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"net"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

func TestConnectionTrackerStopsWithoutConnections(t *testing.T) {
	synctest.Test(t, func(_ *testing.T) {
		tracker := NewConnectionTracker("test", 0)
		tracker.Start()
		tracker.Stop()
		tracker.Stop()
		synctest.Wait()
	})
}

func TestConnectionTrackerPendingOperationsAtStop(t *testing.T) {
	for _, operation := range []string{"track", "close"} {
		t.Run(operation, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				tracker := NewConnectionTracker("test", 0)
				a, b := net.Pipe()
				defer b.Close()
				done := make(chan struct{})
				go func() {
					if operation == "track" {
						tracker.Track(a)
						tracker.Close(a)
					} else {
						tracker.Close(a)
					}
					close(done)
				}()
				synctest.Wait()
				stopped := make(chan struct{})
				go func() { tracker.Stop(); close(stopped) }()
				synctest.Wait()
				tracker.Start()
				<-stopped
				<-done
				if _, err := b.Write([]byte("closed")); err == nil {
					t.Fatal("late connection was not closed")
				}
			})
		})
	}
}

func TestConnectionTrackerBasic(_ *testing.T) {
	tracker := NewConnectionTracker("test", 1*time.Second)
	tracker.Start()
	a, b := net.Pipe()
	tracker.Track(a)
	tracker.Track(b)

	_ = a.Close()
	tracker.Close(a)

	var wg sync.WaitGroup
	wg.Add(1)

	go func() { tracker.Stop(); wg.Done() }()

	// Stop() will complete once the connection is closed.
	_ = b.Close()
	tracker.Close(b)

	wg.Wait() // Wait for stop to complete.

}

func TestConnectionTrackerRaceToStop(_ *testing.T) {

	tracker := NewConnectionTracker("test", 1*time.Second)
	tracker.Start()

	var stopped, started sync.WaitGroup
	stopped.Add(1)
	started.Add(1)

	// Start tracking a lot of extra connections to make sure we can stop.

	go func() {
		var conns []net.Conn
		for i := 0; i < 10000; i++ {
			a, b := net.Pipe()
			tracker.Track(a)
			tracker.Track(b)
			if i == 100 {
				started.Done()
			}
			conns = append(conns, a, b)
		}
		// Close all connections.
		for _, c := range conns {
			// Block until closed
			_, _ = c.Read(make([]byte, 1))
			_ = c.Close()
			tracker.Close(c)
		}
	}()

	started.Wait()

	// Stop() will complete once all tracked connections are closed.
	go func() { tracker.Stop(); stopped.Done() }()

	stopped.Wait()
}
