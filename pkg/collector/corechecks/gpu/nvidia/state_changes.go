// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"fmt"
	"maps"
	"sync"
	"sync/atomic"
	"time"

	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

const (
	// stateChangesPollInterval is how often the state changes gatherer reads the GPU states,
	// independently of the check interval, so that events have a 1s resolution.
	stateChangesPollInterval = time.Second
	// maxPendingStateChanges bounds the events kept per GPU between two collections, in case
	// the check stops collecting; the oldest are dropped first.
	maxPendingStateChanges = 100
)

// gpuStates are the last states the gatherer read for a GPU.
type gpuStates struct {
	links            map[int]bool       // NVLink link index -> active
	unsupportedLinks map[int]bool       // links whose state NVML doesn't support, not read again
	availability     *availabilityState // nil until read
	// healthUnsupported is set once no health source can ever exist, to not read them again.
	healthUnsupported bool
	pending           []Sample // protected by the gatherer mutex
}

// StateChangesGatherer reads the NVLink link states and the availability of every physical GPU
// of the device cache every second in a background worker, with the same functions the
// stateless collector uses for the nvlink.link_up and device.unavailable metrics. It keeps an
// event for each change until the state changes collector of the GPU takes them.
type StateChangesGatherer struct {
	interval    time.Duration
	deviceCache ddnvml.DeviceCache
	deps        *CollectorDependencies

	// lifecycleMu makes Start and Stop safe to call concurrently, e.g. Cancel while Run starts
	// the gatherer. The worker doesn't take it, so Stop can wait for the worker while holding it.
	lifecycleMu sync.Mutex
	running     atomic.Bool
	stop        chan struct{}
	wg          sync.WaitGroup

	mu   sync.Mutex
	gpus map[string]*gpuStates // uuid -> states
}

// NewStateChangesGatherer creates a stopped gatherer for the GPUs of the device cache. deps
// provides what the availability needs.
func NewStateChangesGatherer(deviceCache ddnvml.DeviceCache, deps *CollectorDependencies) *StateChangesGatherer {
	return &StateChangesGatherer{
		interval:    stateChangesPollInterval,
		deviceCache: deviceCache,
		deps:        deps,
		gpus:        map[string]*gpuStates{},
	}
}

// StateChangesEnabled returns whether the state changes collector is enabled in the
// configuration, i.e. whether the gatherer needs to run.
func StateChangesEnabled(config gpuconfig.Config) bool {
	return !collectorDisabled(stateChanges, config)
}

// Started returns true if the worker is running.
func (g *StateChangesGatherer) Started() bool {
	return g.running.Load()
}

// Start starts the worker.
func (g *StateChangesGatherer) Start() {
	g.lifecycleMu.Lock()
	defer g.lifecycleMu.Unlock()
	if g.running.Load() {
		return
	}
	g.stop = make(chan struct{})
	g.wg.Add(1)
	go g.worker()
	g.running.Store(true)
}

// Stop stops the worker and waits for it. The states and pending events are kept, by GPU
// UUID: after a restart, e.g. once an NVML release for a GPU reset ends, the first read is
// compared with the states from before, so a change during the stop is still reported.
func (g *StateChangesGatherer) Stop() {
	g.lifecycleMu.Lock()
	defer g.lifecycleMu.Unlock()
	if !g.running.Load() {
		return
	}
	close(g.stop)
	g.wg.Wait()
	g.running.Store(false)
}

// takeEvents returns the events of the GPU since the previous call.
func (g *StateChangesGatherer) takeEvents(deviceUUID string) []Sample {
	g.mu.Lock()
	defer g.mu.Unlock()
	states, ok := g.gpus[deviceUUID]
	if !ok {
		return nil
	}
	events := states.pending
	states.pending = nil
	return events
}

func (g *StateChangesGatherer) worker() {
	defer g.wg.Done()
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()
	for {
		g.pollAll(time.Now())
		select {
		case <-g.stop:
			return
		case <-ticker.C:
		}
	}
}

// pollAll reads the states of every GPU of the device cache and keeps an event for each
// change. The lock is not held during the reads, so collections don't wait for NVML.
func (g *StateChangesGatherer) pollAll(now time.Time) {
	devices, err := g.deviceCache.AllPhysicalDevices()
	if err != nil {
		return
	}

	seen := make(map[string]struct{}, len(devices))
	for _, device := range devices {
		uuid := device.GetDeviceInfo().UUID
		seen[uuid] = struct{}{}

		g.mu.Lock()
		states, ok := g.gpus[uuid]
		if !ok {
			states = &gpuStates{links: map[int]bool{}, unsupportedLinks: map[int]bool{}}
			g.gpus[uuid] = states
		}
		g.mu.Unlock()

		events := g.poll(device, states, now)
		if len(events) == 0 {
			continue
		}
		g.mu.Lock()
		states.pending = append(states.pending, events...)
		if excess := len(states.pending) - maxPendingStateChanges; excess > 0 {
			states.pending = states.pending[excess:]
		}
		g.mu.Unlock()
	}

	// Forget the GPUs that are gone.
	g.mu.Lock()
	defer g.mu.Unlock()
	maps.DeleteFunc(g.gpus, func(uuid string, _ *gpuStates) bool {
		_, ok := seen[uuid]
		return !ok
	})
}

// poll updates the stored states of a GPU and returns the events for what changed. If a state
// can't be read, we keep the previous one; the metric collectors already report the error.
func (g *StateChangesGatherer) poll(device ddnvml.Device, states *gpuStates, now time.Time) []Sample {
	uuid := device.GetDeviceInfo().UUID
	var events []Sample

	for link := range device.GetDeviceInfo().NVLinkLinkCount {
		if states.unsupportedLinks[link] {
			continue
		}
		active, ok, err := nvlinkLinkState(device, link)
		if ddnvml.IsUnsupported(err) {
			states.unsupportedLinks[link] = true
		}
		if !ok {
			continue
		}
		if previous, seen := states.links[link]; seen && previous != active {
			events = append(events, nvlinkChangeEvent(uuid, link, active, now))
		}
		states.links[link] = active
	}

	// The state is computed from what could be read, like the device.unavailable metric: a
	// reason that can't be read is not reported, so it doesn't make the GPU unavailable.
	// The unschedulable reason comes from the node state, which the check refreshes once
	// per run: changes caused by it have the check interval resolution, not 1s.
	availability, err := readDeviceAvailabilityWithHealth(device, g.deps, !states.healthUnsupported)
	if errors.Is(err, errUnsupportedDevice) {
		return events
	}
	states.healthUnsupported = availability.healthUnsupported
	current := availability.state()
	if states.availability != nil && !states.availability.equal(current) {
		events = append(events, availabilityChangeEvent(uuid, *states.availability, current, now))
	}
	states.availability = &current

	return events
}

// nvlinkChangeEvent is the event for an NVLink link going up or down. Its port is 1-based,
// like the nvlink_port tag of the NVLink metrics.
func nvlinkChangeEvent(deviceUUID string, link int, active bool, now time.Time) Sample {
	title, alertType, from, to := "GPU NVLink port down", event.AlertTypeError, "active", "inactive"
	if active {
		title, alertType, from, to = "GPU NVLink port up", event.AlertTypeSuccess, "inactive", "active"
	}
	return NewEvent(event.Event{
		Title:          title,
		Text:           fmt.Sprintf("NVLink port %d on GPU %s changed from %s to %s.", link+1, deviceUUID, from, to),
		Priority:       event.PriorityNormal,
		AlertType:      alertType,
		SourceTypeName: "gpu",
		EventType:      "gpu_nvlink_state_change",
		AggregationKey: deviceUUID,
	}, now, Medium, []string{nvlinkPortTag(link + 1)}, nil)
}

// availabilityChangeEvent is the event for a change of the availability of a GPU. It carries
// the current reason tags, and the previous ones with a previous_ prefix.
func availabilityChangeEvent(deviceUUID string, previous, current availabilityState, now time.Time) Sample {
	title, alertType := "GPU device unavailable", event.AlertTypeError
	if current.reason == "" {
		title, alertType = "GPU device available", event.AlertTypeSuccess
	}

	var tags []string
	if current.reason != "" {
		tags = append(tags, "unavailable_reason:"+current.reason)
		tags = append(tags, current.unschedulableTags...)
	}
	if previous.reason != "" {
		tags = append(tags, "previous_unavailable_reason:"+previous.reason)
		for _, tag := range previous.unschedulableTags {
			tags = append(tags, "previous_"+tag)
		}
	}

	evt := NewEvent(event.Event{
		Title:          title,
		Text:           fmt.Sprintf("The availability of GPU %s changed from %s to %s.", deviceUUID, previous.describe(), current.describe()),
		Priority:       event.PriorityNormal,
		AlertType:      alertType,
		SourceTypeName: "gpu",
		EventType:      "gpu_availability_change",
		AggregationKey: deviceUUID,
	}, now, Medium, tags, nil)
	// Like device.unavailable, the event is about the GPU, not the workloads on it.
	evt.noWorkloadTags = true
	return evt
}

// describe returns the state in words, e.g. "unavailable (unhealthy)".
func (a availabilityState) describe() string {
	if a.reason == "" {
		return "available"
	}
	return "unavailable (" + a.reason + ")"
}

// stateChangesCollector sends the events of the state changes gatherer for its device.
type stateChangesCollector struct {
	device   ddnvml.Device
	gatherer *StateChangesGatherer
}

func newStateChangesCollector(device ddnvml.Device, deps *CollectorDependencies) (Collector, error) {
	if deps.StateChangesGatherer == nil {
		return nil, errors.New("state changes gatherer cannot be nil")
	}
	// The gatherer reads the NVLink links and the availability of physical devices.
	if _, physical := device.(*ddnvml.PhysicalDevice); !physical && device.GetDeviceInfo().NVLinkLinkCount <= 0 {
		return nil, errUnsupportedDevice
	}
	return &stateChangesCollector{device: device, gatherer: deps.StateChangesGatherer}, nil
}

// Device returns the device this collector monitors.
func (c *stateChangesCollector) Device() ddnvml.Device {
	return c.device
}

// Name returns the name of the collector.
func (c *stateChangesCollector) Name() CollectorName {
	return stateChanges
}

// Collect returns the events of the device since the previous collection.
func (c *stateChangesCollector) Collect() ([]Sample, error) {
	return c.gatherer.takeEvents(c.device.GetDeviceInfo().UUID), nil
}
