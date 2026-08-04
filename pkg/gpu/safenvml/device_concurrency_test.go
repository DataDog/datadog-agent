// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// concurrencyTracker records how many native calls are in flight at once.
type concurrencyTracker struct {
	mu       sync.Mutex
	inFlight int
	max      int
}

func (c *concurrencyTracker) enter() {
	c.mu.Lock()
	c.inFlight++
	if c.inFlight > c.max {
		c.max = c.inFlight
	}
	c.mu.Unlock()

	// Widen the window so an unsynchronized implementation reliably overlaps.
	time.Sleep(time.Millisecond)
}

func (c *concurrencyTracker) exit() {
	c.mu.Lock()
	c.inFlight--
	c.mu.Unlock()
}

func (c *concurrencyTracker) maxInFlight() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.max
}

// trackCalls overrides the NVML entry points that the GPU check invokes
// concurrently so that each one reports when it enters and leaves the driver.
func trackCalls(tracker *concurrencyTracker) func(*nvmlmock.Device) {
	return func(m *nvmlmock.Device) {
		m.GpmSampleGetFunc = func(_ nvml.GpmSample) nvml.Return {
			tracker.enter()
			defer tracker.exit()
			return nvml.SUCCESS
		}
		m.GetFieldValuesFunc = func(_ []nvml.FieldValue) nvml.Return {
			tracker.enter()
			defer tracker.exit()
			return nvml.SUCCESS
		}
		m.GetSamplesFunc = func(_ nvml.SamplingType, _ uint64) (nvml.ValueType, []nvml.Sample, nvml.Return) {
			tracker.enter()
			defer tracker.exit()
			return nvml.VALUE_TYPE_UNSIGNED_INT, nil, nvml.SUCCESS
		}
		m.GetBAR1MemoryInfoFunc = func() (nvml.BAR1Memory, nvml.Return) {
			tracker.enter()
			defer tracker.exit()
			return nvml.BAR1Memory{}, nvml.SUCCESS
		}
	}
}

func newTestSafeDevice(t *testing.T, nvmlDevice nvml.Device) *safeDeviceImpl {
	t.Helper()

	lib, err := GetSafeNvmlLib()
	require.NoError(t, err)

	return &safeDeviceImpl{nvmlDevice: nvmlDevice, lib: lib}
}

// staticLookup reports every symbol as available. Unlike the NVML singleton it
// is never reset on test cleanup, so probe goroutines that outlive a test
// cannot race with the teardown of the shared capability map.
type staticLookup struct{}

func (staticLookup) lookup(string) error { return nil }

// TestDeviceSerializesCallsOnSameHandle is the regression test for the crash
// seen on 7.82.0: the nvlink_gpm and gpm collectors ran in parallel and issued
// two nvmlGpmSampleGet calls against the same device handle, which faulted
// inside the NVIDIA driver. No two native calls may overlap on one handle.
func TestDeviceSerializesCallsOnSameHandle(t *testing.T) {
	WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithSymbolsMock(allSymbols)))

	tracker := &concurrencyTracker{}
	device := newTestSafeDevice(t, testutil.GetDeviceMock(0, trackCalls(tracker)))

	// Mirror the collector fan-out: several different APIs, plus two concurrent
	// GPM sample reads, all against a single device.
	calls := []func(){
		func() { _ = device.GpmSampleGet(nil) },
		func() { _ = device.GpmSampleGet(nil) },
		func() { _ = device.GetFieldValues(make([]nvml.FieldValue, 1)) },
		func() { _ = device.GetFieldValues(make([]nvml.FieldValue, 1)) },
		func() { _, _, _ = device.GetSamples(nvml.TOTAL_POWER_SAMPLES, 0) },
		func() { _, _ = device.GetBAR1MemoryInfo() },
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		for _, call := range calls {
			wg.Add(1)
			go func(call func()) {
				defer wg.Done()
				call()
			}(call)
		}
	}
	wg.Wait()

	require.Equal(t, 1, tracker.maxInFlight(),
		"native NVML calls must never overlap on the same device handle")
}

// TestDeviceAllowsConcurrencyAcrossHandles ensures the lock is per handle. A
// process-wide lock would serialize every GPU and undo the parallel collector
// work, so two devices must be able to sit in the driver at the same time.
func TestDeviceAllowsConcurrencyAcrossHandles(t *testing.T) {
	WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithSymbolsMock(allSymbols)))

	// Both calls block until the other has arrived. If a single lock covered
	// both devices, neither could proceed.
	bothArrived := make(chan struct{}, 2)
	proceed := make(chan struct{})
	rendezvous := func(m *nvmlmock.Device) {
		m.GpmSampleGetFunc = func(_ nvml.GpmSample) nvml.Return {
			bothArrived <- struct{}{}
			<-proceed
			return nvml.SUCCESS
		}
	}

	deviceA := newTestSafeDevice(t, testutil.GetDeviceMock(0, rendezvous))
	deviceB := newTestSafeDevice(t, testutil.GetDeviceMock(1, rendezvous))

	var wg sync.WaitGroup
	wg.Add(2)
	for _, device := range []*safeDeviceImpl{deviceA, deviceB} {
		go func(device *safeDeviceImpl) {
			defer wg.Done()
			_ = device.GpmSampleGet(nil)
		}(device)
	}

	for i := 0; i < 2; i++ {
		select {
		case <-bothArrived:
		case <-time.After(10 * time.Second):
			close(proceed)
			t.Fatal("calls to different device handles did not run concurrently")
		}
	}

	close(proceed)
	wg.Wait()
}

// TestAllDeviceMethodsHoldTheLock guards against a new SafeDevice method being
// added that reaches the driver without taking the device lock. Every method is
// invoked while the lock is held; none of them may complete.
func TestAllDeviceMethodsHoldTheLock(t *testing.T) {
	device := &safeDeviceImpl{nvmlDevice: testutil.GetDeviceMock(0), lib: staticLookup{}}

	// Held for the rest of the test. The probe goroutines below stay blocked on
	// it and are never released: unblocking them would let them reach mock
	// functions that are not configured here, panicking outside the test body.
	device.mu.Lock()

	deviceValue := reflect.ValueOf(device)
	ifaceType := reflect.TypeOf((*SafeDevice)(nil)).Elem()

	completed := make(chan string, ifaceType.NumMethod())
	started := make(chan struct{}, ifaceType.NumMethod())

	for i := 0; i < ifaceType.NumMethod(); i++ {
		name := ifaceType.Method(i).Name
		method := deviceValue.MethodByName(name)
		require.True(t, method.IsValid(), "method %s not found on safeDeviceImpl", name)

		args := make([]reflect.Value, method.Type().NumIn())
		for j := range args {
			args[j] = reflect.New(method.Type().In(j)).Elem()
		}

		go func() {
			started <- struct{}{}
			method.Call(args)
			completed <- name
		}()
	}

	for i := 0; i < ifaceType.NumMethod(); i++ {
		<-started
	}

	select {
	case name := <-completed:
		t.Fatalf("%s returned while the device lock was held: it does not serialize its NVML call", name)
	case <-time.After(500 * time.Millisecond):
		// No method made it through the lock, which is what we want.
	}
}
