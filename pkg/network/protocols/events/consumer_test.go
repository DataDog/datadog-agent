// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package events

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func TestConsumer(t *testing.T) {
	kversion, err := kernel.HostVersion()
	require.NoError(t, err)
	if minVersion := kernel.VersionCode(4, 14, 0); kversion < minVersion {
		t.Skipf("package not supported by kernels < %s", minVersion)
	}

	const numEvents = 100
	c := config.New()
	program, err := NewEBPFProgram(c)
	require.NoError(t, err)

	var mux sync.Mutex
	result := make(map[uint64]int)
	callback := func(events []uint64) {
		mux.Lock()
		defer mux.Unlock()
		for _, n := range events {
			result[n] = +1
		}
	}

	consumer, err := NewConsumer("test", program.Manager, callback)
	require.NoError(t, err)
	consumer.Start()

	err = program.Start()
	require.NoError(t, err)

	// generate test events
	generator := newEventGenerator(program, t)
	for i := 0; i < numEvents; i++ {
		generator.Generate(uint64(i))
	}
	generator.Stop()
	time.Sleep(100 * time.Millisecond)

	// this ensures that any incomplete batch left in eBPF is fully processed
	consumer.Sync()
	program.Stop(manager.CleanAll)
	consumer.Stop()

	// ensure that we have received each event exactly once
	for i := 0; i < numEvents; i++ {
		actual := result[uint64(i)]
		assert.Equalf(t, 1, actual, "eventID=%d should have 1 occurrence. got %d", i, actual)
	}
}

func TestInvalidBatchCountMetric(t *testing.T) {
	kversion, err := kernel.HostVersion()
	require.NoError(t, err)
	if minVersion := kernel.VersionCode(4, 14, 0); kversion < minVersion {
		t.Skipf("package not supported by kernels < %s", minVersion)
	}

	c := config.New()
	program, err := NewEBPFProgram(c)
	require.NoError(t, err)
	t.Cleanup(func() { program.Stop(manager.CleanAll) })

	consumer, err := NewConsumer("test", program.Manager, func([]uint64) {})
	require.NoError(t, err)

	// We are creating a raw sample with a data length of 4, which is smaller than sizeOfBatch
	// and would be considered an invalid batch.
	RecordSample(c, consumer, []byte("test"))

	consumer.Start()
	t.Cleanup(func() { consumer.Stop() })
	require.Eventually(t, func() bool {
		// Wait for the consumer to process the invalid batch.
		return consumer.invalidBatchCount.Get() == 1
	}, 5*time.Second, 100*time.Millisecond)
}

func TestNegativeLengthEventCountReal(t *testing.T) {
	kversion, err := kernel.HostVersion()
	require.NoError(t, err)
	if minVersion := kernel.VersionCode(4, 14, 0); kversion < minVersion {
		t.Skipf("package not supported by kernels < %s", minVersion)
	}

	c := config.New()
	program, err := NewEBPFProgram(c)
	require.NoError(t, err)
	t.Cleanup(func() { program.Stop(manager.CleanAll) })

	mu := sync.Mutex{}
	results := map[uint64]int{}
	consumer, err := NewConsumer("test", program.Manager, func(v []uint64) {
		mu.Lock()
		defer mu.Unlock()
		for _, n := range v {
			results[n] = +1
		}
	})
	require.NoError(t, err)
	consumer.Start()
	t.Cleanup(func() { consumer.Stop() })

	err = program.Start()
	require.NoError(t, err)

	// Create an event generator
	generator := newEventGenerator(program, t)
	defer generator.Stop()

	// Generate events in a tight loop while simultaneously calling Sync()
	// This creates a race condition between normal batch processing and forced sync
	// which can lead to the offset manager getting into an inconsistent state
	const cycles = int(10)
	const iterations = int(1000)
	for i := 0; i < iterations; i++ {
		// Generate events
		for j := 0; j < cycles; j++ {
			generator.Generate(uint64(i*cycles + j))
		}

		// Call Sync() while events are still being processed
		// This can cause the offset manager to get out of sync
		consumer.Sync()

		// Small delay to allow some events to be processed
		time.Sleep(time.Millisecond)
	}

	// Final sync to ensure all events are processed
	consumer.Sync()

	assert.Eventually(t, func() bool {
		// Wait for the consumer to process the invalid batch.
		return consumer.eventsCount.Get() == int64(cycles*iterations)
	}, 5*time.Second, 100*time.Millisecond, "expected %d events, got %d", cycles*iterations, consumer.eventsCount.Get())

	if t.Failed() {
		for k, v := range results {
			if v != 1 {
				t.Logf("eventID=%d should have 1 occurrence. got %d", k, v)
			}
		}
	}

	require.Zero(t, consumer.invalidBatchCount.Get(), "expected 0 invalid batch count, got %d", consumer.invalidBatchCount.Get())
}

type eventGenerator struct {
	// map used for coordinating test with eBPF program space
	testMap *ebpf.Map

	// file used for triggering write(2) syscalls
	testFile *os.File
}

func newEventGenerator(program *ddebpf.Manager, t *testing.T) *eventGenerator {
	m, _, _ := program.GetMap("test")
	require.NotNilf(t, m, "couldn't find test map")

	f, err := os.Create(filepath.Join(t.TempDir(), "foobar"))
	require.NoError(t, err)

	return &eventGenerator{
		testMap:  m,
		testFile: f,
	}
}

func (e *eventGenerator) Generate(eventID uint64) error {
	type testCtx struct {
		pid     uint32
		fd      uint64
		eventID uint64
	}

	var (
		fd  = uint64(e.testFile.Fd())
		key = uint32(0)
		val = testCtx{pid: uint32(os.Getpid()), fd: fd, eventID: eventID}
	)

	// this is done so the eBPF program will echo back an event that is equal to
	// eventID once the write syscall is triggered below
	err := e.testMap.Put(unsafe.Pointer(&key), unsafe.Pointer(&val))
	if err != nil {
		return err
	}

	e.testFile.Write([]byte("whatever"))
	return nil
}

func (e *eventGenerator) Stop() {
	e.testFile.Close()
}
