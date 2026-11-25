// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"strings"
	"sync/atomic"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nodeunschedulable"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// The lost reason is always reported on physical devices. The unhealthy reason is only
// reported once a health source can be read: a GPU missing from workloadmeta at startup
// reports it once known, with an error until then.
func TestDeviceUnavailableReasons(t *testing.T) {
	device := setupMockDevice(t, testutil.WithUnsupportedFields(nvml.FI_DEV_GET_GPU_RECOVERY_ACTION))
	reasons := func(deps *CollectorDependencies) ([]string, error) {
		samples, _, err := deviceUnavailableSample(device, deps)
		var reasons []string
		for _, m := range requireMetrics(t, samples) {
			require.Equal(t, "device.unavailable", m.Name)
			for _, tag := range m.Tags() {
				if reason, ok := strings.CutPrefix(tag, "unavailable_reason:"); ok {
					reasons = append(reasons, reason)
				}
			}
		}
		return reasons, err
	}

	env.SetFeatures(t)
	got, err := reasons(&CollectorDependencies{Workloadmeta: testutil.GetWorkloadMetaMock(t)})
	require.NoError(t, err)
	require.Equal(t, []string{"lost"}, got, "no health source can ever exist")

	env.SetFeatures(t, env.KubernetesDevicePlugins)
	got, err = reasons(&CollectorDependencies{Workloadmeta: testutil.GetWorkloadMetaMock(t)})
	require.Error(t, err, "the GPU isn't in workloadmeta yet")
	require.Equal(t, []string{"lost"}, got)

	got, err = reasons(&CollectorDependencies{Workloadmeta: testutil.GetWorkloadMetaMockWithDefaultGPUs(t)})
	require.NoError(t, err)
	require.Equal(t, []string{"lost", "unhealthy"}, got)

	_, _, err = deviceUnavailableSample(&nonPhysicalDevice{device}, &CollectorDependencies{})
	require.ErrorIs(t, err, errUnsupportedDevice, "only physical devices")
}

// nonPhysicalDevice wraps a device so that it isn't a physical device, like a MIG slice.
type nonPhysicalDevice struct{ safenvml.Device }

// A device that can't be queried for another reason than being lost doesn't report the lost
// reason, but still reports the others.
func TestDeviceUnavailableIndexError(t *testing.T) {
	failIndex := atomic.Bool{} // set after the device is created, which queries its index
	device := setupMockDevice(t, testutil.WithCustomHook(func(d *testutil.MockDevice) {
		getIndex := d.GetIndexFunc
		d.GetIndexFunc = func() (int, nvml.Return) {
			if failIndex.Load() {
				return 0, nvml.ERROR_UNKNOWN
			}
			return getIndex()
		}
	}))
	failIndex.Store(true)
	samples, _, err := deviceUnavailableSample(device, &CollectorDependencies{})
	require.Error(t, err)
	metrics := requireMetrics(t, samples)
	require.Len(t, metrics, 1)
	require.Equal(t, []string{"unavailable_reason:unhealthy"}, metrics[0].Tags())
	require.Zero(t, metrics[0].Value)
}

func TestAvailabilityState(t *testing.T) {
	cordoned := nodeunschedulable.Point{Tags: []string{"unavailable_reason:unschedulable", "unschedulable_reason:cordoned"}, Value: 1}
	clearedCause := nodeunschedulable.Point{Tags: []string{"unavailable_reason:unschedulable", "unschedulable_reason:not_ready"}, Value: 0}

	tests := []struct {
		name         string
		availability deviceAvailability
		expected     availabilityState
	}{
		{name: "available", availability: deviceAvailability{lostKnown: true, healthKnown: true}, expected: availabilityState{}},
		{name: "a cleared cause is not a reason", availability: deviceAvailability{lostKnown: true, healthKnown: true, nodePoints: []nodeunschedulable.Point{clearedCause}}, expected: availabilityState{}},
		{
			name:         "unschedulable",
			availability: deviceAvailability{lostKnown: true, healthKnown: true, nodePoints: []nodeunschedulable.Point{cordoned, clearedCause}},
			expected:     availabilityState{reason: "unschedulable", unschedulableTags: []string{"unschedulable_reason:cordoned"}},
		},
		{
			name:         "unhealthy takes precedence over unschedulable",
			availability: deviceAvailability{lostKnown: true, healthKnown: true, unhealthy: true, nodePoints: []nodeunschedulable.Point{cordoned}},
			expected:     availabilityState{reason: "unhealthy"},
		},
		{
			name:         "lost takes precedence over everything",
			availability: deviceAvailability{lostKnown: true, healthKnown: true, lost: true, unhealthy: true, nodePoints: []nodeunschedulable.Point{cordoned}},
			expected:     availabilityState{reason: "lost"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.availability.state())

			// The state is the reason whose device.unavailable series is 1.
			var oneReasons []string
			for _, m := range requireMetrics(t, tt.availability.samples()) {
				if m.Value == 1 {
					oneReasons = append(oneReasons, m.Tags()[0])
				}
			}
			if tt.expected.reason == "" {
				require.Empty(t, oneReasons)
			} else {
				require.Equal(t, []string{"unavailable_reason:" + tt.expected.reason}, oneReasons)
			}
		})
	}
}

// The availability events must follow the device.unavailable metric: for every combination of
// what could be read, the state is the reason whose series is 1, and at most one is 1.
func TestAvailabilityStateMatchesMetric(t *testing.T) {
	point := func(value float64, reason string) nodeunschedulable.Point {
		return nodeunschedulable.Point{Tags: []string{"unavailable_reason:unschedulable", "unschedulable_reason:" + reason}, Value: value}
	}
	nodePoints := [][]nodeunschedulable.Point{
		nil,
		{point(1, "cordoned")},
		{point(0, "cordoned")},
		{point(1, "not_ready"), point(0, "cordoned")},
	}

	for _, lost := range []bool{false, true} {
		for _, lostKnown := range []bool{false, true} {
			for _, healthKnown := range []bool{false, true} {
				for _, unhealthy := range []bool{false, true} {
					for _, points := range nodePoints {
						// readDeviceAvailability only sets lost and unhealthy when they were read.
						if (lost && !lostKnown) || (unhealthy && !healthKnown) {
							continue
						}
						a := deviceAvailability{lost: lost, lostKnown: lostKnown, unhealthy: unhealthy, healthKnown: healthKnown, nodePoints: points}

						var expected availabilityState
						ones := 0
						for _, m := range requireMetrics(t, a.samples()) {
							if m.Value != 1 {
								continue
							}
							ones++
							for _, tag := range m.Tags() {
								if reason, ok := strings.CutPrefix(tag, "unavailable_reason:"); ok {
									expected.reason = reason
								} else if strings.HasPrefix(tag, "unschedulable_reason:") {
									expected.unschedulableTags = append(expected.unschedulableTags, tag)
								}
							}
						}
						require.LessOrEqual(t, ones, 1, "%+v", a)
						require.Equal(t, expected, a.state(), "%+v", a)
					}
				}
			}
		}
	}
}
