// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"maps"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nodeunschedulable"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

// stateChangesTestGPU is a mock GPU whose NVLink states and lost state tests change while
// the gatherer worker runs.
type stateChangesTestGPU struct {
	mu          sync.Mutex
	inactive    map[int]bool
	unsupported map[int]bool // links whose state NVML doesn't support
	lost        bool
	linkReads   map[int]int // GetNvLinkState calls per link
	healthReads int         // recovery action field reads
}

func (g *stateChangesTestGPU) reads() (map[int]int, int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return maps.Clone(g.linkReads), g.healthReads
}

func (g *stateChangesTestGPU) setInactive(link int, inactive bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inactive[link] = inactive
}

func (g *stateChangesTestGPU) setLost(lost bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.lost = lost
}

// setupStateChangesTest installs a mock NVML with one GPU with the given NVLink count, and
// returns the GPU controls, its device and a stopped gatherer over the device cache.
func setupStateChangesTest(t *testing.T, nvlinks int, opts ...testutil.NvmlMockOption) (*stateChangesTestGPU, ddnvml.Device, *StateChangesGatherer) {
	t.Helper()
	gpu := &stateChangesTestGPU{inactive: map[int]bool{}, unsupported: map[int]bool{}, linkReads: map[int]int{}}
	devices := setupMockDevices(t, append([]testutil.NvmlMockOption{
		testutil.WithDeviceCount(1),
		testutil.WithNVLinkLinkCount(nvlinks),
		testutil.WithCustomHook(func(d *testutil.MockDevice) {
			getIndex := d.GetIndexFunc
			d.GetIndexFunc = func() (int, nvml.Return) {
				gpu.mu.Lock()
				defer gpu.mu.Unlock()
				if gpu.lost {
					return 0, nvml.ERROR_GPU_IS_LOST
				}
				return getIndex()
			}
			getFieldValues := d.GetFieldValuesFunc
			d.GetFieldValuesFunc = func(values []nvml.FieldValue) nvml.Return {
				for _, v := range values {
					if v.FieldId == nvml.FI_DEV_GET_GPU_RECOVERY_ACTION {
						gpu.mu.Lock()
						gpu.healthReads++
						gpu.mu.Unlock()
					}
				}
				return getFieldValues(values)
			}
			d.GetNvLinkStateFunc = func(link int) (nvml.EnableState, nvml.Return) {
				gpu.mu.Lock()
				defer gpu.mu.Unlock()
				gpu.linkReads[link]++
				if gpu.unsupported[link] {
					return 0, nvml.ERROR_NOT_SUPPORTED
				}
				if gpu.inactive[link] {
					return nvml.FEATURE_DISABLED, nvml.SUCCESS
				}
				return nvml.FEATURE_ENABLED, nvml.SUCCESS
			}
		}),
	}, opts...)...)
	require.Len(t, devices, 1)
	return gpu, devices[0], NewStateChangesGatherer(ddnvml.NewDeviceCache(), &CollectorDependencies{})
}

func collectEvents(t *testing.T, c Collector) []*Event {
	t.Helper()
	samples, err := c.Collect()
	require.NoError(t, err)
	events := make([]*Event, 0, len(samples))
	for _, s := range samples {
		e, ok := s.(*Event)
		require.True(t, ok, "expected only events, got %T", s)
		events = append(events, e)
	}
	return events
}

func TestStateChangesNVLink(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gpu, device, g := setupStateChangesTest(t, 2)
		c, err := newStateChangesCollector(device, &CollectorDependencies{StateChangesGatherer: g})
		require.NoError(t, err)
		require.Equal(t, stateChanges, c.Name())
		g.Start()
		defer g.Stop()

		// The first read is only a baseline.
		synctest.Wait()
		require.Empty(t, collectEvents(t, c))

		// A port goes down between two check runs: the event has the time of the read
		// that saw it, not the time of the collection.
		time.Sleep(2500 * time.Millisecond)
		gpu.setInactive(1, true)
		changedAt := time.Now()
		time.Sleep(6 * time.Second)
		synctest.Wait()

		events := collectEvents(t, c)
		require.Len(t, events, 1)
		require.Equal(t, "GPU NVLink port down", events[0].event.Title)
		require.Equal(t, event.AlertTypeError, events[0].event.AlertType)
		require.Equal(t, []string{"nvlink_port:2"}, events[0].Tags())
		require.Equal(t, changedAt.Add(500*time.Millisecond), events[0].occurredAt, "seen on the next 1s read")
		require.Empty(t, collectEvents(t, c), "Collect takes the events")

		gpu.setInactive(1, false)
		time.Sleep(time.Second)
		synctest.Wait()
		events = collectEvents(t, c)
		require.Len(t, events, 1)
		require.Equal(t, "GPU NVLink port up", events[0].event.Title)
	})
}

func TestStateChangesAvailability(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gpu, device, g := setupStateChangesTest(t, 0)
		c, err := newStateChangesCollector(device, &CollectorDependencies{StateChangesGatherer: g})
		require.NoError(t, err)
		g.Start()
		defer g.Stop()
		synctest.Wait()

		gpu.setLost(true)
		time.Sleep(time.Second)
		synctest.Wait()
		events := collectEvents(t, c)
		require.Len(t, events, 1)
		require.Equal(t, "GPU device unavailable", events[0].event.Title)
		require.Equal(t, "gpu_availability_change", events[0].event.EventType)
		require.Equal(t, []string{"unavailable_reason:lost"}, events[0].Tags())

		gpu.setLost(false)
		time.Sleep(time.Second)
		synctest.Wait()
		events = collectEvents(t, c)
		require.Len(t, events, 1)
		require.Equal(t, "GPU device available", events[0].event.Title)
		require.Equal(t, []string{"previous_unavailable_reason:lost"}, events[0].Tags())
	})
}

func TestStateChangesKeepsStatesAcrossStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gpu, device, g := setupStateChangesTest(t, 2)
		c, err := newStateChangesCollector(device, &CollectorDependencies{StateChangesGatherer: g})
		require.NoError(t, err)
		g.Start()
		synctest.Wait()

		// A port goes down right before a stop, e.g. for an NVML release: the event is kept
		// until it's collected.
		gpu.setInactive(0, true)
		time.Sleep(time.Second)
		synctest.Wait()
		g.Stop()
		g.Stop() // stopping twice is fine
		events := collectEvents(t, c)
		require.Len(t, events, 1)
		require.Equal(t, "GPU NVLink port down", events[0].event.Title)

		// While stopped, e.g. during a GPU reset, nothing is read: the port comes back up and
		// another one goes down.
		gpu.setInactive(0, false)
		gpu.setInactive(1, true)
		time.Sleep(5 * time.Second)
		synctest.Wait()
		require.Empty(t, collectEvents(t, c))

		// Started again: the first read is compared with the states from before the stop.
		g.Start()
		defer g.Stop()
		synctest.Wait()
		events = collectEvents(t, c)
		require.Len(t, events, 2)
		require.Equal(t, "GPU NVLink port up", events[0].event.Title)
		require.Equal(t, []string{"nvlink_port:1"}, events[0].Tags())
		require.Equal(t, "GPU NVLink port down", events[1].event.Title)
		require.Equal(t, []string{"nvlink_port:2"}, events[1].Tags())
	})
}

// Run can start the gatherer while Cancel stops it.
func TestStateChangesConcurrentStartStop(t *testing.T) {
	_, _, g := setupStateChangesTest(t, 2)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			g.Start()
		}()
		go func() {
			defer wg.Done()
			g.Stop()
		}()
	}
	wg.Wait()

	g.Stop()
	require.False(t, g.Started())
	g.Start()
	require.True(t, g.Started())
	g.Stop()
	require.False(t, g.Started())
}

func TestStateChangesDoesNotReadUnsupportedAgain(t *testing.T) {
	// Link 1 doesn't support its state, and the recovery action field isn't supported: with
	// no device plugins, the GPU has no health source.
	env.SetFeatures(t)
	gpu, device, g := setupStateChangesTest(t, 2, testutil.WithUnsupportedFields(nvml.FI_DEV_GET_GPU_RECOVERY_ACTION))
	gpu.unsupported[1] = true
	for i := range 5 {
		g.pollAll(time.Unix(int64(i), 0))
	}

	linkReads, healthReads := gpu.reads()
	require.Equal(t, map[int]int{0: 5, 1: 1}, linkReads, "the unsupported link is read once")
	require.Equal(t, 1, healthReads, "the unsupported health source is read once")
	require.Empty(t, g.takeEvents(device.GetDeviceInfo().UUID))
}

// The availability is computed from what could be read, like the device.unavailable metric:
// a health source that can't be read doesn't hold back the node state.
func TestStateChangesAvailabilityWithUnreadableHealth(t *testing.T) {
	// Device plugins are enabled, but the GPU is not in workloadmeta, so its health can't
	// be read; the node is cordoned.
	env.SetFeatures(t, env.Kubernetes, env.KubernetesDevicePlugins)
	_, device, _ := setupStateChangesTest(t, 0, testutil.WithUnsupportedFields(nvml.FI_DEV_GET_GPU_RECOVERY_ACTION))
	src := &cordonSource{}
	nodeState := nodeunschedulable.NewCache(src, time.Nanosecond)
	deps := &CollectorDependencies{Workloadmeta: testutil.GetWorkloadMetaMock(t), NodeUnschedulable: nodeState}
	g := NewStateChangesGatherer(ddnvml.NewDeviceCache(), deps)
	uuid := device.GetDeviceInfo().UUID

	_, err := readDeviceAvailability(device, deps)
	require.Error(t, err, "the health can't be read")

	nodeState.Refresh(time.Unix(1, 0))
	g.pollAll(time.Unix(1, 0))

	src.cordoned = true
	nodeState.Refresh(time.Unix(2, 0))
	g.pollAll(time.Unix(2, 0))
	events := g.takeEvents(uuid)
	require.Len(t, events, 1)
	require.Equal(t, "GPU device unavailable", events[0].(*Event).event.Title)
	require.Equal(t, []string{"unavailable_reason:unschedulable", "unschedulable_reason:cordoned"}, events[0].Tags())
}

// cordonSource is a node state source for a node that is cordoned or not.
type cordonSource struct{ cordoned bool }

func (s *cordonSource) NodeUnschedulable() (nodeunschedulable.Result, error) {
	if s.cordoned {
		return nodeunschedulable.Result{Unschedulable: true, Reasons: []string{"cordoned"}}, nil
	}
	return nodeunschedulable.Result{Reasons: []string{}}, nil
}
func (s *cordonSource) Probe() error { return nil }

func TestStateChangesUnsupported(t *testing.T) {
	_, device, g := setupStateChangesTest(t, 1)
	_, err := newStateChangesCollector(device, &CollectorDependencies{})
	require.Error(t, err, "the gatherer is required")

	// Neither NVLink nor a physical device (e.g. a MIG slice).
	type nonPhysicalDevice struct{ ddnvml.Device }
	_, device, _ = setupStateChangesTest(t, 0)
	_, err = newStateChangesCollector(nonPhysicalDevice{device}, &CollectorDependencies{StateChangesGatherer: g})
	require.ErrorIs(t, err, errUnsupportedDevice)
}

func TestStateChangesBoundsPendingEvents(t *testing.T) {
	// A port flipping on every read, with nothing collecting the events.
	gpu, device, g := setupStateChangesTest(t, 1)
	reads := maxPendingStateChanges + 10
	for i := range reads {
		gpu.setInactive(0, i%2 == 1)
		g.pollAll(time.Unix(int64(i), 0))
	}

	events := g.takeEvents(device.GetDeviceInfo().UUID)
	require.Len(t, events, maxPendingStateChanges, "the first read is a baseline, then the oldest are dropped")
	require.Equal(t, time.Unix(int64(reads-1), 0), events[len(events)-1].(*Event).occurredAt, "the newest are kept")
}

func TestAvailabilityChangeEvent(t *testing.T) {
	now := time.Unix(100, 0)
	cordoned := availabilityState{reason: "unschedulable", unschedulableTags: []string{"unschedulable_reason:cordoned"}}
	unhealthy := availabilityState{reason: "unhealthy"}

	tests := []struct {
		name              string
		previous, current availabilityState
		title             string
		alertType         event.AlertType
		tags              []string
	}{
		{
			name: "becomes unschedulable", previous: availabilityState{}, current: cordoned,
			title: "GPU device unavailable", alertType: event.AlertTypeError,
			tags: []string{"unavailable_reason:unschedulable", "unschedulable_reason:cordoned"},
		},
		{
			name: "the reason escalates", previous: cordoned, current: unhealthy,
			title: "GPU device unavailable", alertType: event.AlertTypeError,
			tags: []string{"unavailable_reason:unhealthy", "previous_unavailable_reason:unschedulable", "previous_unschedulable_reason:cordoned"},
		},
		{
			name: "available again", previous: unhealthy, current: availabilityState{},
			title: "GPU device available", alertType: event.AlertTypeSuccess,
			tags: []string{"previous_unavailable_reason:unhealthy"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt := availabilityChangeEvent("GPU-1", tt.previous, tt.current, now).(*Event)
			require.Equal(t, tt.title, evt.event.Title)
			require.Equal(t, tt.alertType, evt.event.AlertType)
			require.Equal(t, "gpu_availability_change", evt.event.EventType)
			require.Equal(t, "GPU-1", evt.event.AggregationKey)
			require.Equal(t, now, evt.occurredAt)
			require.True(t, evt.NoWorkloadTags())
			require.Equal(t, tt.tags, evt.Tags())
		})
	}
}
