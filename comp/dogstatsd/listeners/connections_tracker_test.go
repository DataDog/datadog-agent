// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package listeners

import (
	"net"
	"sync"
	"testing"
	"time"
)

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
