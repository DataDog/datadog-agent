// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/safenvml/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestDeviceEventsGatherer_RegisterBeforeStart(t *testing.T) {
	device := setupMockDevice(t)

	gatherer := NewDeviceEventsGatherer(nil)
	assert.Error(t, gatherer.RegisterDevice(device))
}

func TestDeviceEventsGatherer_RegisterWithUnsupportedEvents(t *testing.T) {
	device := setupMockDevice(t, testutil.WithCustomHook(func(device *testutil.MockDevice) {
		device.GetSupportedEventTypesFunc = func() (uint64, nvml.Return) {
			return 0, nvml.SUCCESS
		}
	}))

	gatherer := NewDeviceEventsGatherer(nil)
	assert.Error(t, gatherer.RegisterDevice(device))
}

func TestDeviceEventsGatherer_GetWithUnregistered(t *testing.T) {
	nvmltestutil.SetupMockNVML(t)

	gatherer := NewDeviceEventsGatherer(nil)
	require.NoError(t, gatherer.Start())
	t.Cleanup(func() { require.NoError(t, gatherer.Stop()) })

	assert.Empty(t, gatherer.GetRegisteredDeviceUUIDs())

	events, err := gatherer.getEvents("some-uuid")
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestDeviceEventsGatherer_RefreshGetSequence(t *testing.T) {
	WithDeviceEventsSetWaitTimeoutForTest(t, time.Millisecond)

	// by controlling this, we can influence the gathered device events
	gatheredDeviceEvents := make(chan nvml.EventData, 10)
	t.Cleanup(func() { close(gatheredDeviceEvents) })

	// Setup the mock device and library to return events at our command.
	nvmlMock := nvmltestutil.SetupMockNVML(t,
		testutil.WithSymbolsMock(map[string]struct{}{"nvmlDeviceGetUUID": {}}),
		testutil.WithMockAllFunctions(),
		testutil.WithEventSetCreate(func() (nvml.EventSet, nvml.Return) {
			return &mock.EventSet{
				FreeFunc: func() nvml.Return { return nvml.SUCCESS },
				WaitFunc: func(v uint32) (nvml.EventData, nvml.Return) {
					if len(gatheredDeviceEvents) == 0 {
						time.Sleep(time.Duration(v) * time.Millisecond)
						return nvml.EventData{}, nvml.ERROR_TIMEOUT
					}
					return <-gatheredDeviceEvents, nvml.SUCCESS
				},
			}, nvml.SUCCESS
		}),
	)
	device := nvmltestutil.PhysicalDevice(t, nvmlMock, 0)

	// create gatherer after lib initialization so that it picks up the mock
	gatherer := NewDeviceEventsGatherer(nil)
	require.NoError(t, gatherer.Start())
	t.Cleanup(func() { require.NoError(t, gatherer.Stop()) })

	// register device in gatherer
	uuid := device.GetDeviceInfo().UUID
	require.NoError(t, gatherer.RegisterDevice(device))
	require.Len(t, gatherer.GetRegisteredDeviceUUIDs(), 1)
	require.Equal(t, uuid, gatherer.GetRegisteredDeviceUUIDs()[0])

	// no events should be available initially
	events, err := gatherer.getEvents(uuid)
	require.NoError(t, err)
	assert.Empty(t, events)

	// create an event to be gathered, then make sure it is not available until we refresh
	sampleDeviceEvent := nvml.EventData{
		Device:    nvmlMock.Device(0),
		EventType: nvml.EventTypeXidCriticalError,
		EventData: 31, // sample xid error for invalid mem access
	}
	sentAt := time.Now()
	gatheredDeviceEvents <- sampleDeviceEvent
	time.Sleep(time.Duration(float64(eventSetWaitTimeout) * 1.2)) // wait for timeout (with some tolerance)
	events, err = gatherer.getEvents(uuid)
	require.NoError(t, err)
	assert.Empty(t, events)

	// after refreshing, the event should be present
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		require.NoError(c, gatherer.Refresh(time.Now()))
		events, err = gatherer.getEvents(uuid)
		require.NoError(c, err)
		require.Len(c, events, 1)
	}, 200*time.Millisecond, 2*time.Millisecond)
	assert.Equal(t, safenvml.DeviceEventData{
		DeviceUUID: uuid,
		EventType:  sampleDeviceEvent.EventType,
		EventData:  sampleDeviceEvent.EventData,
	}, events[0].DeviceEventData)
	assert.False(t, events[0].ObservedAt.Before(sentAt))

	// make sure the latest events cache is consistent up until the next refresh
	for i := 0; i < 10; i++ {
		events, err = gatherer.getEvents(uuid)
		require.NoError(t, err)
		require.Len(t, events, 1)
	}

	// after refresh, latest events cache should be empty (no new events gathered)
	require.NoError(t, gatherer.Refresh(time.Now()))
	events, err = gatherer.getEvents(uuid)
	require.NoError(t, err)
	require.Empty(t, events)
}

func TestDeviceEventsGatherer_StartShouldFailIfNvmlInitFails(t *testing.T) {
	if _, err := safenvml.GetSafeNvmlLib(); err == nil {
		t.Skip("NVML library is already initialized, this test relies on the library not being initializable")
	}

	gatherer := NewDeviceEventsGatherer(nil)
	require.Error(t, gatherer.Start())
}

func TestDeviceEventsGathererRefreshesSourcesIndependently(t *testing.T) {
	timestamp := time.Unix(100, 0)

	t.Run("keeps NVML events when system-probe refresh fails", func(t *testing.T) {
		sourceErr := errors.New("driver source failed")
		source := &fakeDriverEventsSource{err: sourceErr}
		gatherer := NewDeviceEventsGatherer(source)
		cache := &deviceEventsEventsCache{
			pendingEvents: make(chan observedDeviceEvent, 1),
		}
		cache.pendingEvents <- newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2)
		gatherer.devices["GPU-1"] = cache

		err := gatherer.Refresh(timestamp.Add(time.Second))

		require.ErrorIs(t, err, sourceErr)
		require.Equal(t, 1, source.refreshCalls)
		require.Equal(t, []xidEvent{
			newNVMLXIDEvent(newObservedXIDEvent("GPU-1", 31, timestamp, 1, 2)),
		}, gatherer.getXIDEvents("GPU-1"))
	})

	t.Run("collects system-probe events without NVML events", func(t *testing.T) {
		source := &fakeDriverEventsSource{
			events: []model.DriverEvent{
				newDriverXIDEvent("GPU-1", 31, timestamp, "kernel message"),
				newDriverXIDEvent("GPU-2", 31, timestamp, "other kernel message"),
			},
		}
		gatherer := NewDeviceEventsGatherer(source)

		require.NoError(t, gatherer.Refresh(timestamp.Add(time.Second)))

		require.Equal(t, 1, source.refreshCalls)
		require.Equal(t, []xidEvent{
			newDriverOnlyXIDEvent(newDriverXIDEvent("GPU-1", 31, timestamp, "kernel message")),
		}, gatherer.getXIDEvents("GPU-1"))
		require.Equal(t, []xidEvent{
			newDriverOnlyXIDEvent(newDriverXIDEvent("GPU-2", 31, timestamp, "other kernel message")),
		}, gatherer.getXIDEvents("GPU-2"))
	})
}

type fakeDriverEventsSource struct {
	events       []model.DriverEvent
	err          error
	refreshCalls int
}

func (s *fakeDriverEventsSource) Refresh() error {
	s.refreshCalls++
	return s.err
}

func (s *fakeDriverEventsSource) Get() []model.DriverEvent {
	return s.events
}

func TestDeviceEventsCollectorSupportsDriverEventsWithoutNVMLEvents(t *testing.T) {
	device := setupMockDevice(t, testutil.WithCustomHook(func(device *testutil.MockDevice) {
		device.GetSupportedEventTypesFunc = func() (uint64, nvml.Return) {
			return 0, nvml.SUCCESS
		}
	}))
	uuid := device.GetDeviceInfo().UUID
	gatherer := NewDeviceEventsGatherer(nil)
	setGathererEvents(gatherer, uuid, []xidEvent{{
		DeviceUUID: uuid,
		XIDCode:    31,
		Timestamp:  time.Unix(123, 0),
	}})

	collector, err := newDeviceEventsCollector(device, &CollectorDependencies{
		DeviceEventsGatherer: gatherer,
		Config: gpuconfig.Config{
			Enabled:             true,
			DriverEventsEnabled: true,
		},
	})
	require.NoError(t, err)

	// The system-probe source does not require NVML registration, so the
	// collector must emit its XID even when NVML events are unsupported.
	samples, err := collector.Collect()
	require.NoError(t, err)
	assert.Empty(t, gatherer.GetRegisteredDeviceUUIDs())
	assert.Len(t, eventSamples(samples), 1)

	// Without system-probe events, this device has no usable event source.
	_, err = newDeviceEventsCollector(device, &CollectorDependencies{DeviceEventsGatherer: gatherer})
	require.ErrorIs(t, err, errUnsupportedDevice)
}

func TestDeviceEventsCollectorSupportsDriverEventsAfterNVMLCapabilityQueryFailure(t *testing.T) {
	device := setupMockDevice(t, testutil.WithCustomHook(func(device *testutil.MockDevice) {
		device.GetSupportedEventTypesFunc = func() (uint64, nvml.Return) {
			return 0, nvml.ERROR_UNKNOWN
		}
	}))
	uuid := device.GetDeviceInfo().UUID
	gatherer := NewDeviceEventsGatherer(nil)
	setGathererEvents(gatherer, uuid, []xidEvent{{
		DeviceUUID: uuid,
		XIDCode:    31,
		Timestamp:  time.Unix(123, 0),
	}})

	collector, err := newDeviceEventsCollector(device, &CollectorDependencies{
		DeviceEventsGatherer: gatherer,
		Config: gpuconfig.Config{
			Enabled:             true,
			DriverEventsEnabled: true,
		},
	})
	require.NoError(t, err)

	samples, err := collector.Collect()
	require.NoError(t, err)
	assert.Empty(t, gatherer.GetRegisteredDeviceUUIDs())
	assert.Len(t, eventSamples(samples), 1)
}

func TestDeviceEventsCollectorContinuesAfterNVMLRegistrationFailureWhenDriverEventsEnabled(t *testing.T) {
	device := setupMockDevice(t)
	uuid := device.GetDeviceInfo().UUID
	gatherer := NewDeviceEventsGatherer(nil)
	setGathererEvents(gatherer, uuid, []xidEvent{{
		DeviceUUID: uuid,
		XIDCode:    31,
		Timestamp:  time.Unix(123, 0),
	}})
	collector, err := newDeviceEventsCollector(device, &CollectorDependencies{
		DeviceEventsGatherer: gatherer,
		Config: gpuconfig.Config{
			Enabled:             true,
			DriverEventsEnabled: true,
		},
	})
	require.NoError(t, err)

	samples, err := collector.Collect()
	require.NoError(t, err)
	require.Empty(t, gatherer.GetRegisteredDeviceUUIDs())
	require.Len(t, eventSamples(samples), 1)
}

func TestDeviceEventsCollector(t *testing.T) {
	device := setupMockDevice(t, testutil.WithCustomHook(func(device *testutil.MockDevice) {
		device.RegisterEventsFunc = func(uint64, nvml.EventSet) nvml.Return {
			return nvml.SUCCESS
		}
	}))
	uuid := device.GetDeviceInfo().UUID
	gatherer := NewDeviceEventsGatherer(nil)
	gatherer.evtSet = &mock.EventSet{}

	collector, err := newDeviceEventsCollector(device, &CollectorDependencies{DeviceEventsGatherer: gatherer})
	require.NoError(t, err)
	require.NotNil(t, collector)

	// initially, no device should be registered before the first metrics collection
	require.Equal(t, uuid, collector.Device().GetDeviceInfo().UUID)
	require.Equal(t, deviceEvents, collector.Name())
	require.Empty(t, gatherer.GetRegisteredDeviceUUIDs())

	// no metrics until no event is received, but device should now be registered
	mm, err := collector.Collect()
	require.NoError(t, err)
	require.Empty(t, mm)
	require.Equal(t, []string{uuid}, gatherer.GetRegisteredDeviceUUIDs())

	// add event to cache and check that metrics are properly computed.
	// GetEvents is idempotent until Refresh, so subsequent Collect calls with
	// the same cached events increase the lifetime gauge and re-emit the
	// interval count as the number of events in that Collect call.
	xid31Tags := []string{"type:31", "origin:hardware"}
	xid12Tags := []string{"type:12", "origin:driver"}
	xid31Total := func(value float64) *Metric {
		return NewMetric(xidErrorsTotalMetricName, value, metrics.GaugeType, Medium, xid31Tags, nil)
	}
	xid31Count := func(value float64) *Metric {
		return NewMetric(xidErrorsCountMetricName, value, metrics.CountType, Medium, xid31Tags, nil)
	}
	xid12Total := func(value float64) *Metric {
		return NewMetric(xidErrorsTotalMetricName, value, metrics.GaugeType, Medium, xid12Tags, nil)
	}
	xid12Count := func(value float64) *Metric {
		return NewMetric(xidErrorsCountMetricName, value, metrics.CountType, Medium, xid12Tags, nil)
	}
	observedEvent := newObservedXIDEvent(uuid, 31, time.Unix(123, 0), 0, 0)
	setGathererEvents(gatherer, uuid, []xidEvent{
		{
			DeviceUUID: uuid,
			XIDCode:    31,
			Timestamp:  time.Unix(123, 0),
			NVMLEvent:  &observedEvent,
			DriverEvent: &model.DriverEvent{
				DeviceUUID: uuid,
				Timestamp:  time.Unix(123, 0),
				Type:       model.DriverEventTypeNvidiaXid,
				NvidiaXid: &model.NvidiaXid{
					XidCode: 31,
					Message: "NVRM: Xid test message",
				},
			},
		},
	})
	mm, err = collector.Collect()
	require.NoError(t, err)
	assert.ElementsMatch(t, []*Metric{xid31Total(1), xid31Count(1)}, metricSamples(mm))
	events := eventSamples(mm)
	require.Len(t, events, 1)
	assert.Equal(t, "XID 31 error on "+uuid, events[0].event.Title)
	assert.Equal(t, "NVRM: Xid test message", events[0].event.Text)
	assert.Equal(t, time.Unix(123, 0), events[0].occurredAt)
	assert.Contains(t, events[0].tags, "event_source:nvml")
	assert.Contains(t, events[0].tags, "event_source:kmsg")

	mm, err = collector.Collect()
	require.NoError(t, err)
	assert.ElementsMatch(t, []*Metric{xid31Total(2), xid31Count(1)}, metricSamples(mm))

	// make sure different xid errors produce distinct metric contexts
	setGathererEvents(gatherer, uuid, []xidEvent{
		{
			DeviceUUID: uuid,
			XIDCode:    12,
		},
	})
	mm2, err := collector.Collect()
	require.NoError(t, err)
	assert.ElementsMatch(t, []*Metric{
		xid12Total(1),
		xid12Count(1),
		xid31Total(2),
		xid31Count(0),
	}, metricSamples(mm2))

	// no new events: lifetime gauges stay, interval counts are emitted as zero
	setGathererEvents(gatherer, uuid, nil)
	mm3, err := collector.Collect()
	require.NoError(t, err)
	assert.ElementsMatch(t, []*Metric{
		xid12Total(1),
		xid12Count(0),
		xid31Total(2),
		xid31Count(0),
	}, metricSamples(mm3))

	// multiple events of the same xid in one interval increase the count by that amount
	setGathererEvents(gatherer, uuid, []xidEvent{
		{
			DeviceUUID: uuid,
			XIDCode:    31,
		},
		{
			DeviceUUID: uuid,
			XIDCode:    31,
		},
	})
	mm4, err := collector.Collect()
	require.NoError(t, err)
	assert.ElementsMatch(t, []*Metric{
		xid31Total(4),
		xid31Count(2),
		xid12Total(1),
		xid12Count(0),
	}, metricSamples(mm4))
}

func metricSamples(samples []Sample) []*Metric {
	var metrics []*Metric
	for _, sample := range samples {
		if metric, ok := sample.(*Metric); ok {
			metrics = append(metrics, metric)
		}
	}
	return metrics
}

func eventSamples(samples []Sample) []*Event {
	var events []*Event
	for _, sample := range samples {
		if event, ok := sample.(*Event); ok {
			events = append(events, event)
		}
	}
	return events
}

func setGathererEvents(gatherer *DeviceEventsGatherer, deviceUUID string, events []xidEvent) {
	merger := newXIDEventMerger(xidEventMergeWindow)
	merger.latest = events
	gatherer.xidMergers[deviceUUID] = merger
}
