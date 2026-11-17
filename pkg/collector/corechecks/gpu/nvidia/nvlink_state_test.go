// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"slices"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

// nvlinkStateDevice wraps a mock device so that the NVLink states and read
// errors can be changed between collections.
type nvlinkStateDevice struct {
	ddnvml.Device
	states []nvml.EnableState
	errors map[int]error
}

func (d *nvlinkStateDevice) GetNvLinkState(link int) (nvml.EnableState, error) {
	if err := d.errors[link]; err != nil {
		return 0, err
	}
	return d.states[link], nil
}

// nvlinkStateTestDevice returns a collector over a device with the given
// initial link states, and the wrapper used to change them.
func nvlinkStateTestDevice(t *testing.T, states []nvml.EnableState) (*nvlinkStateDevice, *nvlinkStateCollector, time.Time) {
	t.Helper()
	device := &nvlinkStateDevice{
		Device: setupMockDevice(t,
			testutil.WithCapabilities(testutil.Capabilities{NvLinkGenerationSupported: 1}),
			testutil.WithNVLinkStates(states, nil),
		),
		states: slices.Clone(states),
		errors: map[int]error{},
	}
	now := time.Unix(1000, 0)
	collector, err := newNVLinkStateCollectorWithClock(device, func() time.Time { return now })
	require.NoError(t, err)
	return device, collector, now
}

func requireNVLinkEvents(t *testing.T, samples []Sample) []*Event {
	t.Helper()
	events := make([]*Event, 0, len(samples))
	for _, sample := range samples {
		evt, ok := sample.(*Event)
		require.True(t, ok, "expected only events, got %T", sample)
		events = append(events, evt)
	}
	return events
}

func TestNVLinkStateCollectorBaselineEmitsNothing(t *testing.T) {
	_, collector, _ := nvlinkStateTestDevice(t, []nvml.EnableState{nvml.FEATURE_ENABLED, nvml.FEATURE_DISABLED})
	require.Equal(t, nvlinkState, collector.Name())

	// The inactive link at startup is the baseline, not a transition.
	samples, err := collector.Collect()
	require.NoError(t, err)
	require.Empty(t, samples)
}

func TestNVLinkStateCollectorEmitsTransitions(t *testing.T) {
	device, collector, now := nvlinkStateTestDevice(t, []nvml.EnableState{nvml.FEATURE_ENABLED, nvml.FEATURE_ENABLED, nvml.FEATURE_DISABLED})
	uuid := collector.Device().GetDeviceInfo().UUID

	// Link 0 goes down and link 2 comes back up.
	device.states[0] = nvml.FEATURE_DISABLED
	device.states[2] = nvml.FEATURE_ENABLED

	samples, err := collector.Collect()
	require.NoError(t, err)
	events := requireNVLinkEvents(t, samples)
	require.Len(t, events, 2)

	down := events[0]
	assert.Equal(t, "NVLink port 1 on "+uuid+" went down", down.event.Title)
	assert.Equal(t, "NVLink port 1 (NVML link 0) changed from active to inactive.", down.event.Text)
	assert.Equal(t, event.AlertTypeError, down.event.AlertType)
	assert.Equal(t, nvlinkStateEventType, down.event.EventType)
	assert.Equal(t, "gpu", down.event.SourceTypeName)
	assert.Equal(t, uuid, down.event.AggregationKey)
	assert.Equal(t, now, down.occurredAt)
	assert.Equal(t, Medium, down.Priority())
	assert.ElementsMatch(t, []string{"nvlink_port:1", "nvlink_state:inactive", "nvlink_previous_state:active"}, down.Tags())

	up := events[1]
	assert.Equal(t, "NVLink port 3 on "+uuid+" went up", up.event.Title)
	assert.Equal(t, event.AlertTypeSuccess, up.event.AlertType)
	assert.ElementsMatch(t, []string{"nvlink_port:3", "nvlink_state:active", "nvlink_previous_state:inactive"}, up.Tags())

	// No change since the previous collection: no events.
	samples, err = collector.Collect()
	require.NoError(t, err)
	require.Empty(t, samples)
}

func TestNVLinkStateCollectorReadErrorsAreNotTransitions(t *testing.T) {
	device, collector, _ := nvlinkStateTestDevice(t, []nvml.EnableState{nvml.FEATURE_ENABLED, nvml.FEATURE_ENABLED})

	// Link 1 can't be read: no event, and its last known state is kept.
	device.errors[1] = ddnvml.NewNvmlAPIErrorOrNil("GetNvLinkState", nvml.ERROR_UNKNOWN)
	samples, err := collector.Collect()
	require.Error(t, err)
	require.Empty(t, samples)

	// Link 1 is readable again and still active: no event.
	delete(device.errors, 1)
	samples, err = collector.Collect()
	require.NoError(t, err)
	require.Empty(t, samples)

	// A change that happened while the link couldn't be read is reported
	// once it can be read again.
	device.errors[1] = ddnvml.NewNvmlAPIErrorOrNil("GetNvLinkState", nvml.ERROR_UNKNOWN)
	_, err = collector.Collect()
	require.Error(t, err)
	delete(device.errors, 1)
	device.states[1] = nvml.FEATURE_DISABLED
	samples, err = collector.Collect()
	require.NoError(t, err)
	events := requireNVLinkEvents(t, samples)
	require.Len(t, events, 1)
	assert.Contains(t, events[0].Tags(), "nvlink_port:2")
	assert.Contains(t, events[0].Tags(), "nvlink_state:inactive")
}

func TestNVLinkStateCollectorUnsupported(t *testing.T) {
	t.Run("no nvlinks", func(t *testing.T) {
		device := setupMockDevice(t)
		_, err := newNVLinkStateCollector(device, &CollectorDependencies{})
		require.ErrorIs(t, err, errUnsupportedDevice)
	})

	t.Run("state API not supported", func(t *testing.T) {
		device := setupMockDevice(t,
			testutil.WithNVLinkLinkCount(2),
			testutil.WithCapabilities(testutil.Capabilities{NvLinkGenerationSupported: 0}),
		)
		_, err := newNVLinkStateCollector(device, &CollectorDependencies{})
		require.ErrorIs(t, err, errUnsupportedDevice)
	})

	t.Run("no permission is retried", func(t *testing.T) {
		device := setupMockDevice(t,
			testutil.WithCapabilities(testutil.Capabilities{NvLinkGenerationSupported: 1}),
			testutil.WithNVLinkStates(
				[]nvml.EnableState{nvml.FEATURE_ENABLED, nvml.FEATURE_ENABLED},
				map[int]nvml.Return{0: nvml.ERROR_NO_PERMISSION, 1: nvml.ERROR_NO_PERMISSION},
			),
		)
		_, err := newNVLinkStateCollector(device, &CollectorDependencies{})
		require.Error(t, err)
		require.NotErrorIs(t, err, errUnsupportedDevice)
	})
}
