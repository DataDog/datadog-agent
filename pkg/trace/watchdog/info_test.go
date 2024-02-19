// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package watchdog

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	testDuration = time.Second
)

func TestCPULow(t *testing.T) {
	assert := assert.New(t)
	runtime.GC()
	info := NewCurrentInfo()

	_, _ = info.CPU(time.Now())
	info.cacheDelay = testDuration
	time.Sleep(testDuration)
	c, _ := info.CPU(time.Now())
	t.Logf("CPU (sleep): %v", c)

	// checking that CPU is low enough, this is theoretically flaky,
	// but eating 50% of CPU for a time.Sleep is still not likely to happen often
	assert.LessOrEqualf(c.UserAvg, 0.5, "cpu avg should be below 0.5, got %f", c.UserAvg)
}

func TestCPUHigh(t *testing.T) {
	tests := []struct {
		n        int
		runShort bool // whether to run the test if testing.Short() is true
	}{
		{1, true},
		{10, false},
		{100, false},
	}
	info := NewCurrentInfo()
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%d_goroutines", tc.n), func(t *testing.T) {
			if !tc.runShort && testing.Short() {
				t.Skip("skipping test in short mode")
			}
			assert := assert.New(t)
			runtime.GC()

			done := make(chan struct{}, 1)
			info.CPU(time.Now())
			info.cacheDelay = testDuration
			for i := 0; i < tc.n; i++ {
				go func() {
					j := 0
					for {
						select {
						case <-done:
							return
						default:
							j++
						}
					}
				}()
			}
			time.Sleep(testDuration)
			c, _ := info.CPU(time.Now())
			for i := 0; i < tc.n; i++ {
				done <- struct{}{}
			}
			t.Logf("CPU (%d goroutines): %v", tc.n, c)

			// Checking that CPU is not "too high", the above loops create CPU usage, given that `1` means a single core at full
			// utilization we want to verify that we did not accidentally mix integer percentage values and whole numbers
			// (e.g. 15% should be `0.15` NOT `15`)
			assert.LessOrEqualf(c.UserAvg, float64(tc.n+1), "cpu avg is too high, should never exceed %d, got %f", tc.n, c.UserAvg)
		})
	}
}

func TestMemLow(t *testing.T) {
	assert := assert.New(t)
	runtime.GC()

	info := NewCurrentInfo()
	oldM := info.Mem()
	info.cacheDelay = testDuration
	time.Sleep(testDuration)
	m := info.Mem()
	t.Logf("Mem (sleep): %v", m)

	// Checking that Mem is low enough, this is theorically flaky,
	// unless some other random GoRoutine is running, figures should remain low
	assert.LessOrEqualf(m.Alloc-oldM.Alloc, uint64(1e4), "over 10 Kb allocated since last call, way to high for almost no operation")
	assert.LessOrEqualf(m.Alloc, uint64(1e8), "over 100 Mb allocated (%d bytes), way to high for almost no operation", m.Alloc)
}

func doTestMemHigh(t *testing.T, n int) {
	assert := assert.New(t)
	runtime.GC()

	done := make(chan struct{}, 1)
	data := make(chan []byte, 1)
	info := NewCurrentInfo()
	oldM := info.Mem()
	info.cacheDelay = testDuration
	go func() {
		a := make([]byte, n)
		a[0] = 1
		a[n-1] = 1
		data <- a
		<-done
	}()
	time.Sleep(testDuration)
	m := info.Mem()
	done <- struct{}{}

	t.Logf("Mem (%d bytes): %v %v", n, oldM, m)

	// Checking that Mem is high enough
	assert.GreaterOrEqualf(m.Alloc, uint64(n), "not enough bytes allocated")
	assert.GreaterOrEqualf(int64(m.Alloc)-int64(oldM.Alloc), int64(n), "not enough bytes allocated since last call")
	<-data
}

func TestMemHigh(t *testing.T) {
	doTestMemHigh(t, 1e5)
	if testing.Short() {
		return
	}
	doTestMemHigh(t, 1e7)
}

func BenchmarkCPU(b *testing.B) {
	info := NewCurrentInfo()
	info.cacheDelay = 0 // disable cache
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = info.CPU(time.Now())
	}
}

func BenchmarkMem(b *testing.B) {
	info := NewCurrentInfo()
	info.cacheDelay = 0 // disable cache
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = info.Mem()
	}
}
