// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build serverless
// +build serverless

package appsec

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

func TestLimiterUnit(t *testing.T) {
	startTime := time.Now()

	t.Run("no-ticks-1", func(t *testing.T) {
		l := NewTestTicker(1, 100)
		l.start(startTime)
		defer l.stop()
		// No ticks between the requests
		require.True(t, l.Allow(), "First call to limiter.Allow() should return True")
		require.False(t, l.Allow(), "Second call to limiter.Allow() should return False")
	})

	t.Run("no-ticks-2", func(t *testing.T) {
		l := NewTestTicker(100, 100)
		l.start(startTime)
		defer l.stop()
		// No ticks between the requests
		for i := 0; i < 100; i++ {
			require.True(t, l.Allow())
		}
		require.False(t, l.Allow())
	})

	t.Run("10ms-ticks", func(t *testing.T) {
		l := NewTestTicker(1, 100)
		l.start(startTime)
		defer l.stop()
		require.True(t, l.Allow(), "First call to limiter.Allow() should return True")
		require.False(t, l.Allow(), "Second call to limiter.Allow() should return false")
		l.tick(startTime.Add(10 * time.Millisecond))
		require.True(t, l.Allow(), "Third call to limiter.Allow() after 10ms should return True")
	})

	t.Run("9ms-ticks", func(t *testing.T) {
		l := NewTestTicker(1, 100)
		l.start(startTime)
		defer l.stop()
		require.True(t, l.Allow(), "First call to limiter.Allow() should return True")
		l.tick(startTime.Add(9 * time.Millisecond))
		require.False(t, l.Allow(), "Second call to limiter.Allow() after 9ms should return False")
		l.tick(startTime.Add(10 * time.Millisecond))
		require.True(t, l.Allow(), "Third call to limiter.Allow() after 10ms should return True")
	})

	t.Run("1s-rate", func(t *testing.T) {
		l := NewTestTicker(1, 1)
		l.start(startTime)
		defer l.stop()
		require.True(t, l.Allow(), "First call to limiter.Allow() should return True with 1s per token")
		l.tick(startTime.Add(500 * time.Millisecond))
		require.False(t, l.Allow(), "Second call to limiter.Allow() should return False with 1s per Token")
		l.tick(startTime.Add(1000 * time.Millisecond))
		require.True(t, l.Allow(), "Third call to limiter.Allow() should return True with 1s per Token")
	})

	t.Run("100-requests-burst", func(t *testing.T) {
		l := NewTestTicker(100, 100)
		l.start(startTime)
		defer l.stop()
		for i := 0; i < 100; i++ {
			require.Truef(t, l.Allow(),
				"Burst call %d to limiter.Allow() should return True with 100 initial tokens", i)
			startTime = startTime.Add(50 * time.Microsecond)
			l.tick(startTime)
		}
	})

	t.Run("101-requests-burst", func(t *testing.T) {
		l := NewTestTicker(100, 100)
		l.start(startTime)
		defer l.stop()
		for i := 0; i < 100; i++ {
			require.Truef(t, l.Allow(),
				"Burst call %d to limiter.Allow() should return True with 100 initial tokens", i)
			startTime = startTime.Add(50 * time.Microsecond)
			l.tick(startTime)
		}
		require.False(t, l.Allow(),
			"Burst call 101 to limiter.Allow() should return False with 100 initial tokens")
	})

	t.Run("bucket-refill-short", func(t *testing.T) {
		l := NewTestTicker(100, 100)
		l.start(startTime)
		defer l.stop()

		for i := 0; i < 1000; i++ {
			startTime = startTime.Add(time.Millisecond)
			l.tick(startTime)
			require.Equalf(t, int64(100), l.t.tokens.Load(), "Bucket should have exactly 100 tokens")
		}
	})

	t.Run("bucket-refill-long", func(t *testing.T) {
		l := NewTestTicker(100, 100)
		l.start(startTime)
		defer l.stop()

		for i := 0; i < 1000; i++ {
			startTime = startTime.Add(3 * time.Second)
			l.tick(startTime)
		}
		require.Equalf(t, int64(100), l.t.tokens.Load(), "Bucket should have exactly 100 tokens")
	})

	t.Run("allow-after-stop", func(t *testing.T) {
		l := NewTestTicker(3, 3)
		l.start(startTime)
		require.True(t, l.Allow())
		l.stop()
		// The limiter keeps allowing until there's no more tokens
		require.True(t, l.Allow())
		require.True(t, l.Allow())
		require.False(t, l.Allow())
	})

	t.Run("allow-before-start", func(t *testing.T) {
		l := NewTestTicker(2, 100)
		// The limiter keeps allowing until there's no more tokens
		require.True(t, l.Allow())
		require.True(t, l.Allow())
		require.False(t, l.Allow())
		l.start(startTime)
		// The limiter has used all its tokens and the bucket is not getting refilled yet
		require.False(t, l.Allow())
		l.tick(startTime.Add(10 * time.Millisecond))
		// The limiter has started refilling its tokens
		require.True(t, l.Allow())
		l.stop()
	})
}

func TestLimiter(t *testing.T) {
	t.Run("concurrency", func(t *testing.T) {
		// Tests the limiter's ability to sample the traces when subjected to a continuous flow of requests
		// Each goroutine will continuously call the rate limiter for 1 second
		for nbUsers := 1; nbUsers <= 10; nbUsers *= 10 {
			t.Run(fmt.Sprintf("continuous-requests-%d-users", nbUsers), func(t *testing.T) {
				var startBarrier, stopBarrier sync.WaitGroup
				// Create a start barrier to synchronize every goroutine's launch and
				// increase the chances of parallel accesses
				startBarrier.Add(1)
				// Create a stopBarrier to signal when all user goroutines are done.
				stopBarrier.Add(nbUsers)
				var skipped, kept atomic.Uint64
				l := NewTokenTicker(0, 100)

				for n := 0; n < nbUsers; n++ {
					go func(l Limiter, kept, skipped *atomic.Uint64) {
						startBarrier.Wait()      // Sync the starts of the goroutines
						defer stopBarrier.Done() // Signal we are done when returning

						for tStart := time.Now(); time.Since(tStart) < 1*time.Second; {
							if !l.Allow() {
								skipped.Inc()
							} else {
								kept.Inc()
							}
						}
					}(l, &kept, &skipped)
				}

				l.Start()
				defer l.Stop()
				start := time.Now()
				startBarrier.Done() // Unblock the user goroutines
				stopBarrier.Wait()  // Wait for the user goroutines to be done
				duration := time.Since(start).Seconds()
				maxExpectedKept := uint64(math.Ceil(duration) * 100)

				require.LessOrEqualf(t, kept.Load(), maxExpectedKept,
					"Expected at most %d kept tokens for a %fs duration", maxExpectedKept, duration)
			})
		}

		burstFreq := 1000 * time.Millisecond
		burstSize := 101
		startTime := time.Now()
		// Simulate sporadic bursts during up to 1 minute
		for burstAmount := 1; burstAmount <= 10; burstAmount++ {
			t.Run(fmt.Sprintf("requests-bursts-%d-iterations", burstAmount), func(t *testing.T) {
				skipped := 0
				kept := 0
				l := NewTestTicker(100, 100)
				l.start(startTime)
				defer l.stop()

				for c := 0; c < burstAmount; c++ {
					for i := 0; i < burstSize; i++ {
						if !l.Allow() {
							skipped++
						} else {
							kept++
						}
					}
					// Schedule next burst 1sec later
					startTime = startTime.Add(burstFreq)
					l.tick(startTime)
				}

				expectedSkipped := (burstSize - 100) * burstAmount
				expectedKept := 100 * burstAmount
				if burstSize < 100 {
					expectedSkipped = 0
					expectedKept = burstSize * burstAmount
				}
				require.Equalf(t, kept, expectedKept, "Expected %d burst requests to be kept", expectedKept)
				require.Equalf(t, expectedSkipped, skipped, "Expected %d burst requests to be skipped", expectedSkipped)
			})
		}
	})
}

func BenchmarkLimiter(b *testing.B) {
	for nbUsers := 1; nbUsers <= 1000; nbUsers *= 10 {
		b.Run(fmt.Sprintf("%d-users", nbUsers), func(b *testing.B) {
			var skipped, kept atomic.Uint64
			limiter := NewTokenTicker(0, 100)
			limiter.Start()
			defer limiter.Stop()
			b.ResetTimer()

			for n := 0; n < b.N; n++ {
				var startBarrier, stopBarrier sync.WaitGroup
				// Create a start barrier to synchronize every goroutine's launch and
				// increase the chances of parallel accesses
				startBarrier.Add(1)
				// Create a stopBarrier to signal when all user goroutines are done.
				stopBarrier.Add(nbUsers)

				for n := 0; n < nbUsers; n++ {
					go func(l Limiter, kept, skipped *atomic.Uint64) {
						startBarrier.Wait()      // Sync the starts of the goroutines
						defer stopBarrier.Done() // Signal we are done when returning

						for i := 0; i < 100; i++ {
							if !l.Allow() {
								skipped.Inc()
							} else {
								kept.Inc()
							}
						}
					}(limiter, &kept, &skipped)
				}
				startBarrier.Done() // Unblock the user goroutines
				stopBarrier.Wait()  // Wait for the user goroutines to be done
			}
		})
	}
}

// TestTicker is a utility struct used to send hand-crafted ticks to the rate limiter for controlled testing
// It also makes sure to give time to the bucket update goroutine by using the optional sync channel
type TestTicker struct {
	C        chan time.Time
	syncChan <-chan struct{}
	t        *TokenTicker
}

func NewTestTicker(tokens, maxTokens int64) *TestTicker {
	return &TestTicker{
		C: make(chan time.Time),
		t: NewTokenTicker(tokens, maxTokens),
	}
}

func (t *TestTicker) start(timeStamp time.Time) {
	t.syncChan = t.t.start(t.C, timeStamp, true)
}

func (t *TestTicker) stop() {
	t.t.Stop()
	close(t.C)
	// syncChan is closed by the token ticker when sure that nothing else will be sent on it
}

func (t *TestTicker) tick(timeStamp time.Time) {
	t.C <- timeStamp
	<-t.syncChan
}

func (t *TestTicker) Allow() bool {
	return t.t.Allow()
}
