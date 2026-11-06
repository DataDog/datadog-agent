// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package amd

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestParseSMIEvent(t *testing.T) {
	for line, want := range map[string]Event{
		// As read from an MI300X VF after a faulting kernel.
		"1 44535:python3": {Type: EventVMFault, Message: "44535:python3"},
		"2 3:0":           {Type: EventThermalThrottle, Message: "3:0"},
		"3 5 1":           {Type: EventGPUPreReset, Message: "5 1"},
		"4 5":             {Type: EventGPUPostReset, Message: "5"},
	} {
		got, ok := parseSMIEvent(line)
		assert.True(t, ok, line)
		assert.Equal(t, want, got, line)
	}
	for _, line := range []string{"c 44535 python3", "d 44535 python3", "zz", ""} {
		_, ok := parseSMIEvent(line)
		assert.False(t, ok, "%q is not a counted event", line)
	}
}

// fakeKFD stands in for /dev/kfd: each subscription is a socket pair whose
// test side receives the event mask and sends events.
type fakeKFD struct {
	mu      sync.Mutex
	writers map[uint64]*os.File
	err     error
}

func newFakeSubscriber(t *testing.T) (*EventSubscriber, *fakeKFD) {
	kfdPath := filepath.Join(t.TempDir(), "kfd")
	require.NoError(t, os.WriteFile(kfdPath, nil, 0o600))
	fake := &fakeKFD{writers: map[uint64]*os.File{}}
	s := NewEventSubscriber(kfdPath)
	s.openSMI = func(_ *os.File, gpuID uint64) (*os.File, error) {
		fake.mu.Lock()
		defer fake.mu.Unlock()
		if fake.err != nil {
			return nil, fake.err
		}
		fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
		require.NoError(t, err)
		require.NoError(t, unix.SetNonblock(fds[0], true))
		test := os.NewFile(uintptr(fds[1]), "test")
		t.Cleanup(func() { test.Close() })
		fake.writers[gpuID] = test
		return os.NewFile(uintptr(fds[0]), "smi"), nil
	}
	t.Cleanup(s.Close)
	return s, fake
}

func (f *fakeKFD) send(t *testing.T, gpuID uint64, data string) {
	f.mu.Lock()
	w := f.writers[gpuID]
	f.mu.Unlock()
	require.NotNil(t, w, "no subscription for GPU %d", gpuID)
	_, err := w.Write([]byte(data))
	require.NoError(t, err)
}

func flushUntil(t *testing.T, s *EventSubscriber, n int) []Event {
	var events []Event
	require.Eventually(t, func() bool {
		got, _ := s.Flush()
		events = append(events, got...)
		return len(events) >= n
	}, 5*time.Second, 10*time.Millisecond)
	return events
}

func TestEventSubscriberMapsEventsToDevices(t *testing.T) {
	s, fake := newFakeSubscriber(t)
	// A compute-partitioned GPU has a KFD GPU ID per partition.
	devices := []*Device{
		{UUID: "gpu-a", kfdGPUIDs: []uint64{100, 101}},
		{UUID: "gpu-b", kfdGPUIDs: []uint64{200}},
	}
	require.NoError(t, s.Sync(devices))

	fake.send(t, 101, "1 44535:python3\n")
	fake.send(t, 200, "3 5 1\n4 5\n")
	events := flushUntil(t, s, 3)

	byDevice := map[string][]EventType{}
	for _, e := range events {
		byDevice[e.DeviceUUID] = append(byDevice[e.DeviceUUID], e.Type)
	}
	assert.Equal(t, map[string][]EventType{
		"gpu-a": {EventVMFault},
		"gpu-b": {EventGPUPreReset, EventGPUPostReset},
	}, byDevice)
}

func TestEventSubscriberMergesRepeatedEvents(t *testing.T) {
	s := NewEventSubscriber(filepath.Join(t.TempDir(), "kfd"))
	defer s.Close()
	start := time.Now()
	fault := Event{DeviceUUID: "gpu-a", Type: EventVMFault, Message: "44535:python3"}
	// One faulting kernel on an MI300X: one message per XCD that hit the fault.
	for i := range 8 {
		fault.Time = start.Add(time.Duration(i) * time.Millisecond)
		s.add(fault)
	}
	other := Event{DeviceUUID: "gpu-a", Type: EventVMFault, Message: "51000:other", Time: start}
	s.add(other)
	// The same fault again after the merge window is a new event.
	fault.Time = start.Add(eventMergeWindow + time.Millisecond)
	s.add(fault)

	events, _ := s.Flush()
	require.Len(t, events, 3)
	assert.Equal(t, "44535:python3", events[0].Message)
	assert.Equal(t, "51000:other", events[1].Message)
	assert.Equal(t, "44535:python3", events[2].Message)
}

func TestEventSubscriberResubscribesAfterAFailedRead(t *testing.T) {
	s, fake := newFakeSubscriber(t)
	devices := []*Device{{UUID: "gpu-a", kfdGPUIDs: []uint64{100}}}
	require.NoError(t, s.Sync(devices))

	// The kernel side of the event file goes away: the reader stops.
	fake.mu.Lock()
	fake.writers[100].Close()
	fake.mu.Unlock()
	require.Eventually(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return len(s.subs) == 0
	}, 5*time.Second, 10*time.Millisecond)

	require.NoError(t, s.Sync(devices))
	fake.send(t, 100, "1 1:x\n")
	events := flushUntil(t, s, 1)
	assert.Equal(t, "gpu-a", events[0].DeviceUUID)
}

func TestEventSubscriberSyncDropsRemovedGPUs(t *testing.T) {
	s, fake := newFakeSubscriber(t)
	require.NoError(t, s.Sync([]*Device{{UUID: "gpu-a", kfdGPUIDs: []uint64{100}}, {UUID: "gpu-b", kfdGPUIDs: []uint64{200}}}))
	require.NoError(t, s.Sync([]*Device{{UUID: "gpu-b", kfdGPUIDs: []uint64{200}}}))

	s.mu.Lock()
	_, subscribed := s.subs[100]
	s.mu.Unlock()
	assert.False(t, subscribed)

	fake.send(t, 200, "1 1:x\n")
	events := flushUntil(t, s, 1)
	assert.Equal(t, "gpu-b", events[0].DeviceUUID)
}

func TestEventSubscriberReportsSubscriptionErrors(t *testing.T) {
	s, fake := newFakeSubscriber(t)
	fake.err = unix.EPERM
	err := s.Sync([]*Device{{UUID: "gpu-a", kfdGPUIDs: []uint64{100}}})
	assert.ErrorIs(t, err, unix.EPERM)

	// The next sync retries.
	fake.err = nil
	require.NoError(t, s.Sync([]*Device{{UUID: "gpu-a", kfdGPUIDs: []uint64{100}}}))
}

func TestEventSubscriberMissingKFD(t *testing.T) {
	s := NewEventSubscriber(filepath.Join(t.TempDir(), "missing"))
	defer s.Close()
	assert.ErrorIs(t, s.Sync([]*Device{{UUID: "gpu-a", kfdGPUIDs: []uint64{100}}}), os.ErrNotExist)
	assert.NoError(t, s.Sync(nil), "nothing to subscribe to")
}

func TestEventSubscriberBoundsPendingEvents(t *testing.T) {
	s, _ := newFakeSubscriber(t)
	now := time.Now()
	for i := range maxPendingEvents + 5 {
		s.add(Event{DeviceUUID: "gpu-a", Type: EventVMFault, Message: string(rune('a'+i%26)) + time.Duration(i).String(), Time: now})
	}
	events, dropped := s.Flush()
	assert.Len(t, events, maxPendingEvents)
	assert.Equal(t, 5, dropped)
}

func TestEventSubscriberCloseStopsReaders(t *testing.T) {
	s, _ := newFakeSubscriber(t)
	require.NoError(t, s.Sync([]*Device{{UUID: "gpu-a", kfdGPUIDs: []uint64{100, 101}}}))
	done := make(chan struct{})
	go func() { s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not stop the readers")
	}
	assert.Error(t, s.Sync(nil), "a closed subscriber does not resubscribe")
}

func TestSMIEventMask(t *testing.T) {
	// The mask written to the event file enables events 1 to 4.
	var mask uint64
	for _, e := range []EventType{EventVMFault, EventThermalThrottle, EventGPUPreReset, EventGPUPostReset} {
		mask |= 1 << (e - 1)
	}
	assert.Equal(t, uint64(0xf), mask)
	assert.Equal(t, []byte{0xf, 0, 0, 0, 0, 0, 0, 0}, binary.LittleEndian.AppendUint64(nil, mask))
	assert.Equal(t, uintptr(0xc0084b1f), amdkfdIOCSMIEvents)
}
