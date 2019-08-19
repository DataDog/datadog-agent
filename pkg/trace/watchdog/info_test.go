// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package watchdog

import (
	"fmt"
	"net/http"
	"net/http/httptest"
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

	c := CPU(time.Now())
	globalCurrentInfo.cacheDelay = testDuration
	time.Sleep(testDuration)
	c = CPU(time.Now())
	t.Logf("CPU (sleep): %v", c)

	// checking that CPU is low enough, this is theorically flaky,
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

	// Checking that CPU is high enough, a very simple ++ loop should be
	// enough to stimulate one core and make it over 50%. One of the goals
	// of this test is to check that values are not wrong by a factor 100, such
	// as mismatching percentages and [0...1]  values.
	assert.Condition(func() bool { return c.UserAvg >= 0.5 }, fmt.Sprintf("cpu avg is too low, got %f", c.UserAvg))
	assert.Condition(func() bool { return c.UserAvg <= float64(n+1) }, fmt.Sprintf("cpu avg is too high, target is %d, got %f", n, c.UserAvg))
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

type testNetHandler struct {
	t *testing.T
}

func (h *testNetHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	r.Body.Close()
	h.t.Logf("request")
}

func newTestNetServer(t *testing.T) *httptest.Server {
	assert := assert.New(t)
	server := httptest.NewServer(&testNetHandler{t: t})
	assert.NotNil(server)
	t.Logf("server on %v", server.URL)
	return server
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
