// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"fmt"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

const (
	nvlinkStateEventType = "gpu_nvlink_state_change"

	// nvlinkStateActive and nvlinkStateInactive match the naming of the
	// nvlink.count.active and nvlink.count.inactive metrics.
	nvlinkStateActive   = "active"
	nvlinkStateInactive = "inactive"
)

// nvlinkLinkState is the last observed state of a single NVLink link.
type nvlinkLinkState struct {
	known  bool
	active bool
}

// nvlinkStateCollector emits an event every time an NVLink link of the device
// changes between active and inactive. The first observation of each link is
// only a baseline: no event is emitted for the state a link has when the
// collector starts, so restarts don't report every inactive link again.
type nvlinkStateCollector struct {
	device ddnvml.Device
	links  []nvlinkLinkState
	now    func() time.Time
}

func newNVLinkStateCollector(device ddnvml.Device, _ *CollectorDependencies) (Collector, error) {
	return newNVLinkStateCollectorWithClock(device, time.Now)
}

func newNVLinkStateCollectorWithClock(device ddnvml.Device, now func() time.Time) (*nvlinkStateCollector, error) {
	linkCount := device.GetDeviceInfo().NVLinkLinkCount
	if linkCount <= 0 {
		return nil, errUnsupportedDevice
	}

	c := &nvlinkStateCollector{
		device: device,
		links:  make([]nvlinkLinkState, linkCount),
		now:    now,
	}

	// Take the baseline. A device where no link supports the API doesn't get
	// the collector; a permission error on every link is returned so the
	// creation is retried.
	supported, err := c.refresh(nil)
	if supported == 0 {
		if err != nil && ddnvml.IsNoPermission(err) {
			return nil, fmt.Errorf("get NVLink state: %w", err)
		}
		return nil, errUnsupportedDevice
	}

	return c, nil
}

// Device returns the device this collector monitors.
func (c *nvlinkStateCollector) Device() ddnvml.Device {
	return c.device
}

// Name returns the name of the collector.
func (c *nvlinkStateCollector) Name() CollectorName {
	return nvlinkState
}

// Collect reads the state of every link and returns one event per link whose
// state changed since the previous successful read.
func (c *nvlinkStateCollector) Collect() ([]Sample, error) {
	var samples []Sample
	_, err := c.refresh(&samples)
	return samples, err
}

// refresh reads the state of every link, updates the stored state and, if
// samples is not nil, appends an event for each transition. It returns the
// number of links whose state could be read. A link whose state can't be read
// keeps its previous state: read errors are not transitions.
func (c *nvlinkStateCollector) refresh(samples *[]Sample) (int, error) {
	var errs []error
	supported := 0
	observedAt := c.now()
	deviceUUID := c.device.GetDeviceInfo().UUID

	for link := range c.links {
		state, err := c.device.GetNvLinkState(link)
		if err != nil {
			if !ddnvml.IsUnsupported(err) {
				errs = append(errs, fmt.Errorf("get NVLink state for link %d: %w", link, err))
			}
			continue
		}

		var active bool
		switch state {
		case nvml.FEATURE_ENABLED:
			active = true
		case nvml.FEATURE_DISABLED:
			active = false
		default:
			errs = append(errs, fmt.Errorf("unexpected NVLink state %d for link %d", state, link))
			continue
		}
		supported++

		previous := c.links[link]
		c.links[link] = nvlinkLinkState{known: true, active: active}
		if samples != nil && previous.known && previous.active != active {
			*samples = append(*samples, nvlinkStateChangeEvent(deviceUUID, link, previous.active, active, observedAt))
		}
	}

	return supported, errors.Join(errs...)
}

func nvlinkStateName(active bool) string {
	if active {
		return nvlinkStateActive
	}
	return nvlinkStateInactive
}

// nvlinkStateChangeEvent builds the event for a link going active or inactive.
// The port is 1-based, matching the nvlink_port tag of the NVLink metrics,
// while link is the 0-based NVML link index.
func nvlinkStateChangeEvent(deviceUUID string, link int, wasActive, isActive bool, observedAt time.Time) Sample {
	port := link + 1
	previousState := nvlinkStateName(wasActive)
	currentState := nvlinkStateName(isActive)

	alertType := event.AlertTypeSuccess
	direction := "up"
	if !isActive {
		alertType = event.AlertTypeError
		direction = "down"
	}

	return NewEvent(event.Event{
		Title:          fmt.Sprintf("NVLink port %d on %s went %s", port, deviceUUID, direction),
		Text:           fmt.Sprintf("NVLink port %d (NVML link %d) changed from %s to %s.", port, link, previousState, currentState),
		Priority:       event.PriorityNormal,
		AlertType:      alertType,
		SourceTypeName: "gpu",
		EventType:      nvlinkStateEventType,
		AggregationKey: deviceUUID,
	}, observedAt, Medium, []string{
		nvlinkPortTag(port),
		"nvlink_state:" + currentState,
		"nvlink_previous_state:" + previousState,
	}, nil)
}
