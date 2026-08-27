// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf && nvml && test

package gpu

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	gputestutil "github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/ktime"
)

func TestCreateDriverEvent(t *testing.T) {
	subscriber, _ := newTestDriverEventSubscriber(t)

	for _, tc := range []struct {
		message string
		xidCode uint64
	}{
		{"NVRM: Xid (PCI:0000:00:1e): 31, pid=1, name=app, channel 0x1. MMU Fault", 31},
		{"NVRM: Xid (PCI:0000:00:1e): 13, Graphics Exception: channel 0x1", 13},
		{"NVRM: Xid (PCI:0000:00:1e): 120, GSP task exception", 120},
		{"NVRM: Xid (PCI:0000:00:1e): 154, GPU recovery action changed", 154},
		{"nvrm:  xid ( pci:0000:00:1e.0 ) : 43, channel 0x1", 43},
	} {
		record := kernel.KmsgRecord{Timestamp: 1234, Message: tc.message}
		expectedTimestamp := subscriber.timeResolver.ResolveMonotonicTimestamp(record.Timestamp * uint64(time.Microsecond))

		event, err := subscriber.createDriverEvent(record)

		require.NoError(t, err, tc.message)
		require.WithinDuration(t, expectedTimestamp, event.Timestamp, time.Millisecond, tc.message)
		require.Equal(t, model.DriverEvent{
			DeviceUUID: gputestutil.DefaultGpuUUID,
			Timestamp:  event.Timestamp,
			Type:       model.DriverEventTypeNvidiaXid,
			XidCode:    tc.xidCode,
		}, event, tc.message)
	}
}

func TestCreateDriverEventReportsResolutionError(t *testing.T) {
	subscriber, _ := newTestDriverEventSubscriber(t)

	_, err := subscriber.createDriverEvent(kernel.KmsgRecord{Message: "NVRM: Xid (PCI:0000:97:00): 31"})

	require.ErrorContains(t, err, "resolve device UUID for PCI bus ID 0000:97:00.0")
}

func TestCreateDriverEventRejectsMalformedMessages(t *testing.T) {
	subscriber, _ := newTestDriverEventSubscriber(t)

	for _, message := range []string{
		"NVRM: Xid (PCI:0000:65:00): invalid",
		"NVRM: Xid (PCI:0000:65:00)",
		"NVRM: Xid: 31",
	} {
		_, err := subscriber.createDriverEvent(kernel.KmsgRecord{Message: message})
		require.Error(t, err, message)
	}
}

func TestDriverEventQueueDropsWhenFull(t *testing.T) {
	subscriber, telemetryMock := newTestDriverEventSubscriber(t)
	first := model.DriverEvent{DeviceUUID: "GPU-first"}
	second := model.DriverEvent{DeviceUUID: "GPU-second"}

	subscriber.enqueue(first)
	subscriber.enqueue(second)

	events, err := subscriber.GetAndFlush()
	require.NoError(t, err)
	require.Equal(t, []model.DriverEvent{first}, events)

	droppedMetrics, err := telemetryMock.GetCountMetric("gpu__driver_events", "dropped")
	require.NoError(t, err)
	require.Len(t, droppedMetrics, 1)
	require.Equal(t, float64(1), droppedMetrics[0].Value())

	events, err = subscriber.GetAndFlush()
	require.NoError(t, err)
	require.Empty(t, events)
}

func TestDriverEventSubscriberStop(t *testing.T) {
	subscriber, _ := newTestDriverEventSubscriber(t)
	reader := newFakeDriverEventReader()
	subscriber.reader = reader
	subscriber.done = make(chan struct{})
	go subscriber.run()
	queuedEvent := model.DriverEvent{DeviceUUID: "GPU-queued"}
	subscriber.enqueue(queuedEvent)

	subscriber.Stop()
	subscriber.Stop()

	events, err := subscriber.GetAndFlush()
	require.Equal(t, []model.DriverEvent{queuedEvent}, events)
	require.ErrorIs(t, err, errDriverEventSubscriberStopped)
}

func TestDriverEventSubscriberStopsAfterReaderError(t *testing.T) {
	subscriber, _ := newTestDriverEventSubscriber(t)
	reader := newFakeDriverEventReader()
	subscriber.reader = reader
	subscriber.done = make(chan struct{})
	go subscriber.run()

	reader.errors <- errors.New("read failed")
	<-subscriber.done

	events, err := subscriber.GetAndFlush()
	require.Empty(t, events)
	require.ErrorIs(t, err, errDriverEventSubscriberStopped)
}

func newTestDriverEventSubscriber(t *testing.T) (*DriverEventSubscriber, telemetry.Mock) {
	ddnvml.WithMockNVML(t, gputestutil.GetBasicNvmlMockWithOptions(
		gputestutil.WithMIGDisabled(),
		gputestutil.WithDeviceCount(1),
	))

	deviceCache := ddnvml.NewDeviceCache()
	require.NoError(t, deviceCache.Refresh())

	timeResolver, err := ktime.NewResolver()
	require.NoError(t, err)

	telemetryMock := gputestutil.GetTelemetryMock(t)
	telemetry := &driverEventTelemetry{}
	telemetry.init(telemetryMock)

	return &DriverEventSubscriber{
		telemetry:    telemetry,
		timeResolver: timeResolver,
		events:       make(chan model.DriverEvent, 1),
		deviceCache:  deviceCache,
	}, telemetryMock
}

type fakeDriverEventReader struct {
	records  chan kernel.KmsgRecord
	errors   chan error
	stopOnce sync.Once
}

func newFakeDriverEventReader() *fakeDriverEventReader {
	return &fakeDriverEventReader{
		records: make(chan kernel.KmsgRecord),
		errors:  make(chan error, 1),
	}
}

func (r *fakeDriverEventReader) Records() <-chan kernel.KmsgRecord {
	return r.records
}

func (r *fakeDriverEventReader) Errors() <-chan error {
	return r.errors
}

func (r *fakeDriverEventReader) Stop() {
	r.stopOnce.Do(func() {
		close(r.errors)
		close(r.records)
	})
}
