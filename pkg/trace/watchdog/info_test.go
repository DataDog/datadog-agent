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

	_ = CPU(time.Now())
	globalCurrentInfo.cacheDelay = testDuration
	time.Sleep(testDuration)
	c := CPU(time.Now())
	t.Logf("CPU (sleep): %v", c)

	// checking that CPU is low enough, this is theoretically flaky,
	// but eating 50% of CPU for a time.Sleep is still not likely to happen often
	assert.Condition(func() bool { return c.UserAvg >= 0.0 }, fmt.Sprintf("cpu avg should be positive, got %f", c.UserAvg))
	assert.Condition(func() bool { return c.UserAvg <= 0.5 }, fmt.Sprintf("cpu avg should be below 0.5, got %f", c.UserAvg))
}

func doTestCPUHigh(t *testing.T, n int) {
	assert := assert.New(t)
	runtime.GC()

	done := make(chan struct{}, 1)
	c := CPU(time.Now())
	globalCurrentInfo.cacheDelay = testDuration
	for i := 0; i < n; i++ {
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
	c = CPU(time.Now())
	for i := 0; i < n; i++ {
		done <- struct{}{}
	}
	t.Logf("CPU (%d goroutines): %v", n, c)

	// Checking that CPU is not "too high", the above loops create CPU usage, given that `1` means a single core at full
	// utilization we want to verify that we did not accidentally mix integer percentage values and whole numbers
	// (e.g. 15% should be `0.15` NOT `15`)
	assert.Condition(func() bool { return c.UserAvg <= float64(n+1) }, fmt.Sprintf("cpu avg is too high, should never exceed %d, got %f", n, c.UserAvg))
}

func TestCPUHigh(t *testing.T) {
	doTestCPUHigh(t, 1)
	if testing.Short() {
		return
	}
	doTestCPUHigh(t, 10)
	doTestCPUHigh(t, 100)
}

func TestMemLow(t *testing.T) {
	assert := assert.New(t)
	runtime.GC()

	oldM := Mem()
	globalCurrentInfo.cacheDelay = testDuration
	time.Sleep(testDuration)
	m := Mem()
	t.Logf("Mem (sleep): %v", m)

	// Checking that Mem is low enough, this is theorically flaky,
	// unless some other random GoRoutine is running, figures should remain low
	assert.True(int64(m.Alloc)-int64(oldM.Alloc) <= 1e4, "over 10 Kb allocated since last call, way to high for almost no operation")
	assert.True(m.Alloc <= 1e8, "over 100 Mb allocated, way to high for almost no operation")
}

func doTestMemHigh(t *testing.T, n int) {
	assert := assert.New(t)
	runtime.GC()

	done := make(chan struct{}, 1)
	data := make(chan []byte, 1)
	oldM := Mem()
	globalCurrentInfo.cacheDelay = testDuration
	go func() {
		a := make([]byte, n)
		a[0] = 1
		a[n-1] = 1
		data <- a
		select {
		case <-done:
		}
	}()
	time.Sleep(testDuration)
	m := Mem()
	done <- struct{}{}

	t.Logf("Mem (%d bytes): %v %v", n, oldM, m)

	// Checking that Mem is high enough
	assert.True(m.Alloc >= uint64(n), "not enough bytes allocated")
	assert.True(int64(m.Alloc)-int64(oldM.Alloc) >= int64(n), "not enough bytes allocated since last call")
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
	CPU(time.Now())                  // make sure globalCurrentInfo exists
	globalCurrentInfo.cacheDelay = 0 // disable cache
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = CPU(time.Now())
	}
}

func BenchmarkMem(b *testing.B) {
	Mem()                            // make sure globalCurrentInfo exists
	globalCurrentInfo.cacheDelay = 0 // disable cache
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Mem()
	}
}
