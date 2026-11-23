// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"slices"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nodeunschedulable"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// The unavailable reasons of device.unavailable, by precedence: a GPU is unavailable for at
// most one reason.
const (
	unavailableReasonLost          = "lost"
	unavailableReasonUnhealthy     = "unhealthy"
	unavailableReasonUnschedulable = "unschedulable"
)

// deviceAvailability is what device.unavailable reports for a physical GPU.
type deviceAvailability struct {
	lost bool
	// lostKnown is false when the device can't be queried for another reason than being lost:
	// lost is then not reported.
	lostKnown bool
	unhealthy bool
	// healthKnown is false when no health source can be read: unhealthy is then not reported.
	healthKnown bool
	// healthUnsupported is true when no health source can ever exist on this device.
	healthUnsupported bool
	// nodePoints are the points of the node unschedulable state, nil when it isn't reported.
	nodePoints []nodeunschedulable.Point
}

// readDeviceAvailability reads the availability of a physical GPU: whether it is lost,
// whether it is unhealthy (the device plugin marks it unhealthy or NVML asks for a recovery
// action) and the node unschedulable state. It returns errUnsupportedDevice for other
// devices. A health read error is returned along with what could be read.
func readDeviceAvailability(device ddnvml.Device, deps *CollectorDependencies) (deviceAvailability, error) {
	return readDeviceAvailabilityWithHealth(device, deps, true)
}

// readDeviceAvailabilityWithHealth is readDeviceAvailability, where readHealth false skips the
// health sources, for a caller that already knows that they are unsupported.
func readDeviceAvailabilityWithHealth(device ddnvml.Device, deps *CollectorDependencies, readHealth bool) (deviceAvailability, error) {
	if _, ok := device.(*ddnvml.PhysicalDevice); !ok {
		return deviceAvailability{}, errUnsupportedDevice
	}

	a := deviceAvailability{lostKnown: true}
	var indexErr error
	if _, err := device.GetIndex(); err != nil {
		if ddnvml.IsGPULost(err) {
			a.lost = true
		} else {
			// The lost series is not reported, the other reasons still are.
			a.lostKnown, indexErr = false, err
		}
	}
	if deps != nil && deps.NodeUnschedulable != nil {
		a.nodePoints = deps.NodeUnschedulable.Points()
	}
	if a.lost {
		a.healthKnown = true
		return a, nil
	}
	if !readHealth {
		a.healthUnsupported = true
		return a, indexErr
	}

	unhealthy, err := gpuUnhealthy(device, deps)
	switch {
	case err == nil:
		a.healthKnown, a.unhealthy = true, unhealthy
	case errors.Is(err, errUnsupportedDevice):
		// No health source can ever exist on this device.
		a.healthUnsupported, err = true, nil
	}
	return a, errors.Join(indexErr, err)
}

// samples returns the device.unavailable series, one per reason. Only the reason that takes
// precedence reports 1; unschedulable series report 0 for a while after their cause clears.
func (a deviceAvailability) samples() []Sample {
	unavailable := func(value bool, tags ...string) Sample {
		return &Metric{
			baseSample: baseSample{tags: tags, noWorkloadTags: true},
			Name:       "device.unavailable",
			Value:      boolToFloat(value),
			Type:       metrics.GaugeType,
		}
	}
	var samples []Sample
	if a.lostKnown {
		samples = append(samples, unavailable(a.lost, "unavailable_reason:"+unavailableReasonLost))
	}
	if a.healthKnown {
		samples = append(samples, unavailable(a.unhealthy && !a.lost, "unavailable_reason:"+unavailableReasonUnhealthy))
	}
	// Unknown health does not hold back the node state.
	for _, p := range a.nodePoints {
		samples = append(samples, unavailable(p.Value == 1 && !a.lost && !a.unhealthy, slices.Clone(p.Tags)...))
	}
	return samples
}

// availabilityState is the availability of a GPU: the reason it is unavailable, empty when
// it is available, and for an unschedulable GPU, its unschedulable_reason tags.
type availabilityState struct {
	reason            string
	unschedulableTags []string
}

func (a availabilityState) equal(other availabilityState) bool {
	return a.reason == other.reason && slices.Equal(a.unschedulableTags, other.unschedulableTags)
}

// state returns the availability state, the reason of device.unavailable that is 1.
func (a deviceAvailability) state() availabilityState {
	switch {
	case a.lost:
		return availabilityState{reason: unavailableReasonLost}
	case a.unhealthy:
		return availabilityState{reason: unavailableReasonUnhealthy}
	}
	for _, p := range a.nodePoints {
		if p.Value != 1 {
			continue
		}
		var tags []string
		for _, tag := range p.Tags {
			if strings.HasPrefix(tag, nodeunschedulable.TagKey+":") {
				tags = append(tags, tag)
			}
		}
		return availabilityState{reason: unavailableReasonUnschedulable, unschedulableTags: tags}
	}
	return availabilityState{}
}

// deviceUnavailableSample reports device.unavailable on physical devices.
func deviceUnavailableSample(device ddnvml.Device, deps *CollectorDependencies) ([]Sample, uint64, error) {
	a, err := readDeviceAvailability(device, deps)
	if errors.Is(err, errUnsupportedDevice) {
		return nil, 0, err
	}
	return a.samples(), 0, err
}

// gpuUnhealthy reports whether the device plugin marks the GPU unhealthy or NVML asks for a
// recovery action. It returns errUnsupportedDevice when neither source can ever exist on this
// device, and another error when none is readable yet.
func gpuUnhealthy(device ddnvml.Device, deps *CollectorDependencies) (unhealthy bool, err error) {
	known := false
	devicePlugins := deps != nil && deps.Workloadmeta != nil && env.IsFeaturePresent(env.KubernetesDevicePlugins)
	if devicePlugins {
		gpu, gpuErr := deps.Workloadmeta.GetGPU(device.GetDeviceInfo().UUID)
		if gpuErr == nil {
			known = true
			unhealthy = !gpu.Healthy
		} else {
			err = gpuErr
		}
	}
	action, actionErr := fieldValueForField(device, nvml.FI_DEV_GET_GPU_RECOVERY_ACTION, "FI_DEV_GET_GPU_RECOVERY_ACTION")
	if actionErr == nil {
		known = true
		unhealthy = unhealthy || nvml.DeviceGpuRecoveryAction(action) != nvml.GPU_RECOVERY_ACTION_NONE
	} else if err == nil {
		err = actionErr
	}
	switch {
	case known:
		return unhealthy, nil
	case !devicePlugins && ddnvml.IsAPIUnsupportedOnDevice(actionErr, device):
		return false, errUnsupportedDevice
	default:
		return false, err
	}
}
