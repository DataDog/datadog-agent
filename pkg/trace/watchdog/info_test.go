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

	c := CPU()
	globalCurrentInfo.cacheDelay = testDuration
	time.Sleep(testDuration)
	c = CPU()
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
	c := CPU()
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
	c = CPU()
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
	assert.True(m.AllocPerSec >= 0.0, "allocs per sec should be positive")
	assert.True(m.AllocPerSec <= 1e5, "over 100 Kb allocated per sec, way too high for a program doing nothing")
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

	t.Logf("Mem (%d bytes): %v", n, m)

	// Checking that Mem is high enough
	assert.True(m.Alloc >= uint64(n), "not enough bytes allocated")
	assert.True(int64(m.Alloc)-int64(oldM.Alloc) >= int64(n), "not enough bytes allocated since last call")
	expectedAllocPerSec := float64(n) * float64(time.Second) / (float64(testDuration))
	assert.True(m.AllocPerSec >= 0.1*expectedAllocPerSec, fmt.Sprintf("not enough bytes allocated per second, expected %f", expectedAllocPerSec))
	assert.True(m.AllocPerSec <= 1.5*expectedAllocPerSec, fmt.Sprintf("not enough bytes allocated per second, expected %f", expectedAllocPerSec))
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

func doTestNetHigh(t *testing.T, n int) {
	assert := assert.New(t)
	runtime.GC()

	servers := make([]*httptest.Server, n)
	for i := range servers {
		servers[i] = newTestNetServer(t)
	}
	time.Sleep(testDuration)
	info := Net()
	t.Logf("Net: %v", info)
	for _, v := range servers {
		v.Close()
	}

	// Checking that Net connections number is in a reasonable range
	assert.True(info.Connections >= int32(n/2), fmt.Sprintf("not enough connections %d < %d / 2", info.Connections, n))
	assert.True(info.Connections <= int32(n*3), fmt.Sprintf("not enough connections %d > %d * 3", info.Connections, n))
}

func TestNetLow(t *testing.T) {
	assert := assert.New(t)
	runtime.GC()

	time.Sleep(testDuration)
	info := Net()
	t.Logf("Net: %v", info)

	// Checking that Net connections number is low enough, this is theorically flaky,
	// unless some other random GoRoutine is running, figures should remain low
	assert.True(int32(info.Connections) <= 10, "over 10 connections open when we're doing nothing, way too high")
}

func BenchmarkCPU(b *testing.B) {
	CPU()                            // make sure globalCurrentInfo exists
	globalCurrentInfo.cacheDelay = 0 // disable cache
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = CPU()
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

func BenchmarkNet(b *testing.B) {
	Net()                            // make sure globalCurrentInfo exists
	globalCurrentInfo.cacheDelay = 0 // disable cache
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Net()
	}
}
