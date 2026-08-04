// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

package noisyneighbor

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
)

type fakePerfEventOpener struct {
	nextFD   int
	failMask uint64
	failCPU  int
	opened   map[int]struct{}
	closed   map[int]struct{}
}

func (f *fakePerfEventOpener) Open(event perfEventDefinition, cpu int) (int, error) {
	if event.mask == f.failMask && cpu == f.failCPU {
		return -1, errors.New("unsupported")
	}
	f.nextFD++
	if f.opened == nil {
		f.opened = make(map[int]struct{})
	}
	f.opened[f.nextFD] = struct{}{}
	return f.nextFD, nil
}

func (f *fakePerfEventOpener) Close(fd int) error {
	if f.closed == nil {
		f.closed = make(map[int]struct{})
	}
	f.closed[fd] = struct{}{}
	return nil
}

func TestOpenPerfEventsRequiresEveryOnlineCPU(t *testing.T) {
	opener := &fakePerfEventOpener{failMask: model.EventCacheMisses, failCPU: 1}
	events, mask, errorCount := openPerfEvents(model.EventCycles|model.EventCacheMisses, []uint{0, 1}, opener)
	require.Equal(t, model.EventCycles, mask)
	require.EqualValues(t, 1, errorCount)
	require.Equal(t, 2, events.fdCount())
	// The cache-miss FD opened for CPU 0 is immediately cleaned up.
	require.Len(t, opener.closed, 1)

	events.close()
	require.Len(t, opener.closed, len(opener.opened))
}

func TestOpenPerfEventsEnforcesFDCap(t *testing.T) {
	cpus := make([]uint, maxPMUCPUs+1)
	events, mask, errorCount := openPerfEvents(model.EventCycles|model.EventInstructions, cpus, &fakePerfEventOpener{})
	require.Zero(t, mask)
	require.EqualValues(t, 2, errorCount)
	require.Zero(t, events.fdCount())
}

func TestScaleCounter(t *testing.T) {
	tests := []struct {
		name                string
		value, enabled, run uint64
		expected            uint64
		ok                  bool
	}{
		{name: "not multiplexed", value: 42, enabled: 10, run: 10, expected: 42, ok: true},
		{name: "multiplexed", value: 100, enabled: 20, run: 10, expected: 200, ok: true},
		{name: "not running", value: 100, enabled: 20, run: 0, ok: false},
		{name: "overflow", value: math.MaxUint64, enabled: 2, run: 1, ok: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, ok := scaleCounter(test.value, test.enabled, test.run)
			require.Equal(t, test.ok, ok)
			require.Equal(t, test.expected, actual)
		})
	}
}

func TestAddCounterRejectsOverflowWithoutMutation(t *testing.T) {
	destination := ebpfPmuCounter{Value: math.MaxUint64 - 1, Enabled: 10, Running: 5}
	original := destination
	require.False(t, addCounter(&destination, ebpfPmuCounter{Value: 2, Enabled: 1, Running: 1}))
	require.Equal(t, original, destination)
}

func TestParseCPUList(t *testing.T) {
	cpus, err := parseCPUList("0-2,5,7-8\n")
	require.NoError(t, err)
	require.Equal(t, []uint{0, 1, 2, 5, 7, 8}, cpus)
	_, err = parseCPUList("4-2")
	require.Error(t, err)
}
