// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestDeviceEventsGatherer_RegisterBeforeStart(t *testing.T) {
	device := setupMockDevice(t, nil)

	gatherer := NewDeviceEventsGatherer()
	assert.Error(t, gatherer.RegisterDevice(device))
}

func TestDeviceEventsGatherer_RegisterWithUnsupportedEvents(t *testing.T) {
	device := setupMockDevice(t, func(device *mock.Device) *mock.Device {
		device.GetSupportedEventTypesFunc = func() (uint64, nvml.Return) {
			return 0, nvml.SUCCESS
		}
		return device
	})

	gatherer := NewDeviceEventsGatherer()
	assert.Error(t, gatherer.RegisterDevice(device))
}

func TestDeviceEventsGatherer_GetWithUnregistered(t *testing.T) {
	safenvml.WithMockNVML(t, testutil.GetBasicNvmlMock())

	gatherer := NewDeviceEventsGatherer()
	require.NoError(t, gatherer.Start())
	t.Cleanup(func() { require.NoError(t, gatherer.Stop()) })

	assert.Empty(t, gatherer.GetRegisteredDeviceUUIDs())

	events, err := gatherer.GetEvents("some-uuid")
	require.NoError(t, err)
	assert.Empty(t, events)
}

func TestDeviceEventsGatherer_RefreshGetSequence(t *testing.T) {
	// by controlling this, we can influence the gathered device events
	gatheredDeviceEvents := make(chan nvml.EventData, 10)
	t.Cleanup(func() { close(gatheredDeviceEvents) })

	// setup mock device, and the nvml lib to return events at our command
	device := setupMockDeviceWithLibOpts(t, nil,
		testutil.WithSymbolsMock(map[string]struct{}{"nvmlDeviceGetUUID": {}}),
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
		}))

	// create gatherer after lib initialization so that it picks up the mock
	gatherer := NewDeviceEventsGatherer()
	require.NoError(t, gatherer.Start())
	t.Cleanup(func() { require.NoError(t, gatherer.Stop()) })

	// register device in gatherer
	uuid := device.GetDeviceInfo().UUID
	require.NoError(t, gatherer.RegisterDevice(device))
	require.Len(t, gatherer.GetRegisteredDeviceUUIDs(), 1)
	require.Equal(t, uuid, gatherer.GetRegisteredDeviceUUIDs()[0])

	// no events should be available initially
	events, err := gatherer.GetEvents(uuid)
	require.NoError(t, err)
	assert.Empty(t, events)

	// create an event to be gathered, then make sure it is not available until we refresh
	sampleDeviceEvent := nvml.EventData{
		Device:    &mock.Device{GetUUIDFunc: func() (string, nvml.Return) { return uuid, nvml.SUCCESS }},
		EventType: nvml.EventTypeXidCriticalError,
		EventData: 31, // sample xid error for invalid mem access
	}
	gatheredDeviceEvents <- sampleDeviceEvent
	time.Sleep(time.Duration(float64(eventSetWaitTimeout) * 1.2)) // wait for timeout (with some tolerance)
	events, err = gatherer.GetEvents(uuid)
	require.NoError(t, err)
	assert.Empty(t, events)

	// after refreshing, the event should be present
	require.NoError(t, gatherer.Refresh())
	events, err = gatherer.GetEvents(uuid)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, safenvml.DeviceEventData{
		DeviceUUID: uuid,
		EventType:  sampleDeviceEvent.EventType,
		EventData:  sampleDeviceEvent.EventData,
	}, events[0])

	// make sure the latest events cache is consistent up until the next refresh
	for i := 0; i < 10; i++ {
		events, err = gatherer.GetEvents(uuid)
		require.NoError(t, err)
		require.Len(t, events, 1)
	}

	// after refresh, latest events cache should be empty (no new events gathered)
	require.NoError(t, gatherer.Refresh())
	events, err = gatherer.GetEvents(uuid)
	require.NoError(t, err)
	require.Empty(t, events)
}

func TestDeviceEventsGatherer_StartShouldFailIfNvmlInitFails(t *testing.T) {
	if _, err := safenvml.GetSafeNvmlLib(); err == nil {
		t.Skip("NVML library is already initialized, this test relies on the library not being initializable")
	}

	gatherer := NewDeviceEventsGatherer()
	require.Error(t, gatherer.Start())
}

type mockDeviceEventsCollectorCache struct {
	uuids  []string
	events []safenvml.DeviceEventData
	err    error
}

func (m *mockDeviceEventsCollectorCache) RegisterDevice(device safenvml.Device) error {
	m.uuids = append(m.uuids, device.GetDeviceInfo().UUID)
	return nil
}

func (m *mockDeviceEventsCollectorCache) SupportsDevice(_ safenvml.Device) (bool, error) {
	return true, nil
}

func (m *mockDeviceEventsCollectorCache) GetEvents(_ string) ([]safenvml.DeviceEventData, error) {
	return m.events, m.err
}

func TestDeviceEventsCollector(t *testing.T) {
	cache := mockDeviceEventsCollectorCache{}
	device := setupMockDevice(t, nil)
	uuid := device.GetDeviceInfo().UUID

	collector, err := newDeviceEventsCollectorWithCache(device, &cache)
	require.NoError(t, err)
	require.NotNil(t, collector)

	// initially, no device should be registered before the first metrics collection
	require.Equal(t, uuid, collector.DeviceUUID())
	require.Equal(t, deviceEvents, collector.Name())
	require.Empty(t, cache.uuids)

	// no metrics until no event is received, but device should now be registered
	mm, err := collector.Collect()
	require.NoError(t, err)
	require.Empty(t, mm)
	require.Len(t, cache.uuids, 1)
	require.Equal(t, cache.uuids[0], uuid)

	// make sure non-xid events are ignored
	cache.events = []safenvml.DeviceEventData{
		{
			DeviceUUID: uuid,
			EventType:  nvml.EventMigConfigChange,
		},
	}
	mm, err = collector.Collect()
	require.NoError(t, err)
	require.Empty(t, mm)

	// add event to cache and check that metrics are properly computed.
	// since we don't change events between subsequent calls, we check
	// that the counter is increased
	xidErrorsMetricName := "errors.xid.total"
	cache.events = []safenvml.DeviceEventData{
		{
			DeviceUUID: uuid,
			EventType:  nvml.EventTypeXidCriticalError,
			EventData:  31,
		},
	}
	mm, err = collector.Collect()
	require.NoError(t, err)
	require.Len(t, mm, 1)
	assert.Equal(t, Metric{
		Name:     xidErrorsMetricName,
		Value:    1,
		Type:     metrics.GaugeType,
		Priority: Medium,
		Tags:     []string{"type:31", "origin:hardware"},
	}, mm[0])

	mm, err = collector.Collect()
	require.NoError(t, err)
	require.Len(t, mm, 1)
	assert.Equal(t, float64(2), mm[0].Value)

	// make sure different xid errors produce distinct metric contexts
	cache.events = []safenvml.DeviceEventData{
		{
			DeviceUUID: uuid,
			EventType:  nvml.EventTypeXidCriticalError,
			EventData:  12,
		},
	}
	mm2, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, mm2, 2)
	assert.ElementsMatch(t, []Metric{
		{
			Name:     xidErrorsMetricName,
			Value:    1,
			Type:     metrics.GaugeType,
			Priority: Medium,
			Tags:     []string{"type:12", "origin:driver"},
		},
		{
			Name:     xidErrorsMetricName,
			Value:    2,
			Type:     metrics.GaugeType,
			Priority: Medium,
			Tags:     []string{"type:31", "origin:hardware"},
		},
	}, mm2)

	// make sure there's no update in case we have no cached events
	cache.events = nil
	mm3, err := collector.Collect()
	require.NoError(t, err)
	require.ElementsMatch(t, mm2, mm3)
}
