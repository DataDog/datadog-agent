// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
)

// The state changes gatherer reads the watched values of every GPU every second, in
// parallel to the check. These benchmarks measure one read of all the GPUs of the host,
// alone and while the fields collectors run: they share the process-wide lock around NVML
// field value queries.

func physicalDevicesForBenchmark(b *testing.B) (safenvml.DeviceCache, []safenvml.Device) {
	b.Helper()
	cache := safenvml.NewDeviceCache()
	devices, err := cache.AllPhysicalDevices()
	if err != nil || len(devices) == 0 {
		b.Skipf("no GPU available: %v", err)
	}
	b.Logf("%d GPUs, %s, %d NVLinks on the first", len(devices), devices[0].GetDeviceInfo().Name, devices[0].GetDeviceInfo().NVLinkLinkCount)
	return cache, devices
}

func statePoll(cache safenvml.DeviceCache) func() {
	poll := nvidia.NewStateChangesPollForTest(cache, &nvidia.CollectorDependencies{})
	poll() // baseline
	return poll
}

// fieldsCollectors returns the collectors that query field values, the ones that compete
// with the state reads for the field values lock.
func fieldsCollectors(b *testing.B, devices []safenvml.Device) []nvidia.Collector {
	b.Helper()
	var collectors []nvidia.Collector
	for _, c := range nvidia.BuildCollectors(devices, &nvidia.CollectorDependencies{}) {
		if name := c.Name(); name == "fields" || name == "nvlink_fields" {
			collectors = append(collectors, c)
		}
	}
	require.NotEmpty(b, collectors)
	return collectors
}

// runInBackground calls f in a loop until the returned function is called.
func runInBackground(f func()) (stop func()) {
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				f()
			}
		}
	}()
	return func() {
		close(done)
		wg.Wait()
	}
}

// BenchmarkStateChangesPoll is one read of all GPUs.
func BenchmarkStateChangesPoll(b *testing.B) {
	cache, _ := physicalDevicesForBenchmark(b)
	poll := statePoll(cache)
	for b.Loop() {
		poll()
	}
}

// BenchmarkStateChangesPollUnderFieldsLoad is one read of all GPUs while the fields
// collectors of all GPUs collect continuously, much more often than the check does.
func BenchmarkStateChangesPollUnderFieldsLoad(b *testing.B) {
	cache, devices := physicalDevicesForBenchmark(b)
	poll := statePoll(cache)
	collectors := fieldsCollectors(b, devices)
	stop := runInBackground(func() {
		for _, c := range collectors {
			_, _ = c.Collect()
		}
	})
	defer stop()

	for b.Loop() {
		poll()
	}
}

// BenchmarkFieldsCollect is one collection of the fields collectors of all GPUs.
func BenchmarkFieldsCollect(b *testing.B) {
	_, devices := physicalDevicesForBenchmark(b)
	collectors := fieldsCollectors(b, devices)
	for b.Loop() {
		for _, c := range collectors {
			_, _ = c.Collect()
		}
	}
}

// BenchmarkFieldsCollectWhilePolling is one collection of the fields collectors of all
// GPUs while the states of all GPUs are read continuously, instead of once per second.
func BenchmarkFieldsCollectWhilePolling(b *testing.B) {
	cache, devices := physicalDevicesForBenchmark(b)
	collectors := fieldsCollectors(b, devices)
	stop := runInBackground(statePoll(cache))
	defer stop()

	for b.Loop() {
		for _, c := range collectors {
			_, _ = c.Collect()
		}
	}
}
