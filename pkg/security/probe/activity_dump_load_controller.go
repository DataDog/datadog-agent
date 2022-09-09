// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"fmt"
	"time"

	"github.com/avast/retry-go"
	"github.com/cilium/ebpf"
	"golang.org/x/time/rate"

	ebpfutils "github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/dump"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// TracedEventTypesReductionOrder is the order by which event types are reduced
	TracedEventTypesReductionOrder = []model.EventType{model.FileOpenEventType, model.SyscallsEventType, model.DNSEventType, model.BindEventType}
	// MinDumpTimeout is the shortest timeout for a dump
	MinDumpTimeout = 10 * time.Minute
)

// ActivityDumpLoadController is a load controller allowing dynamic change of Activity Dump configuration
type ActivityDumpLoadController struct {
	rateLimiter *rate.Limiter
	adm         *ActivityDumpManager

	// config
	tracedCgroupsCount uint64

	// eBPF maps
	tracedCgroupsCounterMap    *ebpf.Map
	tracedCgroupsLockMap       *ebpf.Map
	activityDumpConfigDefaults *ebpf.Map
}

// NewActivityDumpLoadController returns a new activity dump load controller
func NewActivityDumpLoadController(adm *ActivityDumpManager) (*ActivityDumpLoadController, error) {
	tracedCgroupsCounterMap, found, err := adm.probe.manager.GetMap("traced_cgroups_counter")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("couldn't find traced_cgroups_counter map")
	}

	tracedCgroupsLockMap, found, err := adm.probe.manager.GetMap("traced_cgroups_lock")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("couldn't find traced_cgroups_lock map")
	}

	activityDumpConfigDefaultsMap, found, err := adm.probe.manager.GetMap("activity_dump_config_defaults")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("couldn't find activity_dump_config_defaults map")
	}

	return &ActivityDumpLoadController{
		// 1 every timeout, otherwise we do not have time to see real effects from the reduction
		rateLimiter:                rate.NewLimiter(rate.Every(adm.probe.config.ActivityDumpCgroupDumpTimeout), 1),
		tracedCgroupsCounterMap:    tracedCgroupsCounterMap,
		tracedCgroupsLockMap:       tracedCgroupsLockMap,
		tracedCgroupsCount:         uint64(adm.probe.config.ActivityDumpTracedCgroupsCount),
		activityDumpConfigDefaults: activityDumpConfigDefaultsMap,
		adm:                        adm,
	}, nil
}

// PushCurrentConfig pushes the current load controller config to kernel space
func (lc *ActivityDumpLoadController) PushCurrentConfig() error {
	// push default load config values
	defaults := NewActivityDumpLoadConfig(
		lc.adm.probe.config.ActivityDumpTracedEventTypes,
		lc.adm.probe.config.ActivityDumpCgroupDumpTimeout,
		lc.adm.probe.config.ActivityDumpRateLimiter,
		time.Now(),
		lc.adm.probe.resolvers.TimeResolver,
	)
	if err := lc.activityDumpConfigDefaults.Put(uint32(0), defaults); err != nil {
		return fmt.Errorf("couldn't update default activity dump load config: %w", err)
	}

	// push traced cgroups count
	return lc.pushTracedCgroupsCount()
}

// reduceConfig reduces the configuration of the load controller.
// nolint: unused
func (lc *ActivityDumpLoadController) reduceConfig() error {
	if !lc.rateLimiter.Allow() {
		return nil
	}

	// try to reduce the number of concurrent dumps first
	if lc.tracedCgroupsCount > 1 {
		return lc.reduceTracedCgroupsCount()
	}
	return nil
}

// NextPartialDump returns a new dump with the same parameters as the current one, or with reduced load config parameters
// when applicable
func (lc *ActivityDumpLoadController) NextPartialDump(ad *ActivityDump) *ActivityDump {
	newDump := NewActivityDump(ad.adm)
	newDump.DumpMetadata.ContainerID = ad.DumpMetadata.ContainerID
	newDump.DumpMetadata.Comm = ad.DumpMetadata.Comm
	newDump.DumpMetadata.DifferentiateArgs = ad.DumpMetadata.DifferentiateArgs
	newDump.Tags = ad.Tags
	newDump.IsPartialDump = true

	// copy storage requests
	for _, reqList := range ad.StorageRequests {
		for _, req := range reqList {
			newDump.AddStorageRequest(dump.NewStorageRequest(
				req.Type,
				req.Format,
				req.Compression,
				req.OutputDirectory,
			))
		}
	}

	// compute the duration it took to reach the dump size threshold
	timeToThreshold := ad.End.Sub(ad.Start)

	// set new load parameters
	newDump.LoadConfig.SetTimeout(ad.LoadConfig.Timeout - timeToThreshold)
	newDump.LoadConfig.TracedEventTypes = make([]model.EventType, len(ad.LoadConfig.TracedEventTypes))
	copy(newDump.LoadConfig.TracedEventTypes, ad.LoadConfig.TracedEventTypes)
	newDump.LoadConfig.Rate = ad.LoadConfig.Rate

	if timeToThreshold < MinDumpTimeout {
		if err := lc.reduceDumpRate(ad, newDump); err != nil {
			seclog.Errorf("%v", err)
		}
	}

	if timeToThreshold < MinDumpTimeout/4 && ad.LoadConfig.Timeout > MinDumpTimeout {
		if err := lc.reduceDumpTimeout(ad, newDump); err != nil {
			seclog.Errorf("%v", err)
		}
	}

	if timeToThreshold < MinDumpTimeout/10 {
		if err := lc.reduceTracedEventTypes(ad, newDump); err != nil {
			seclog.Errorf("%v", err)
		}
	}
	return newDump
}

// reduceDumpRate reduces the dump rate configuration and applies the updated value to kernel space
func (lc *ActivityDumpLoadController) reduceDumpRate(old, new *ActivityDump) error {
	new.LoadConfig.Rate = old.LoadConfig.Rate * 3 / 4 // reduce by 25%

	// send metric
	return lc.sendLoadControllerTriggeredMetric([]string{"reduction:rate"})
}

// reduceTracedEventTypes removes an event type from the list of traced events types and updates the list of enabled
// event types for a given dump
func (lc *ActivityDumpLoadController) reduceTracedEventTypes(old, new *ActivityDump) error {
	var evtToRemove model.EventType

reductionOrder:
	for _, evt := range TracedEventTypesReductionOrder {
		for _, tracedEvt := range old.LoadConfig.TracedEventTypes {
			if evt == tracedEvt {
				evtToRemove = evt
				break reductionOrder
			}
		}
	}

	for _, evt := range old.LoadConfig.TracedEventTypes {
		if evt == evtToRemove {
			continue
		}
		new.LoadConfig.TracedEventTypes = append(new.LoadConfig.TracedEventTypes, evt)
	}

	// send metric
	if evtToRemove != model.UnknownEventType {
		if err := lc.sendLoadControllerTriggeredMetric([]string{"reduction:traced_event_types", "event_type:" + evtToRemove.String()}); err != nil {
			return err
		}
	}
	return nil
}

// reduceDumpTimeout reduces the dump timeout configuration
func (lc *ActivityDumpLoadController) reduceDumpTimeout(old, new *ActivityDump) error {
	newTimeout := old.LoadConfig.Timeout * 3 / 4 // reduce by 25%
	if newTimeout < MinDumpTimeout {
		newTimeout = MinDumpTimeout
	}
	new.LoadConfig.SetTimeout(newTimeout)

	// send metric
	return lc.sendLoadControllerTriggeredMetric([]string{"reduction:dump_timeout"})
}

// reduceTracedCgroupsCount decrements the maximum count of cgroups that can be traced simultaneously and applies the
// updated value to kernel space.
// nolint: unused
func (lc *ActivityDumpLoadController) reduceTracedCgroupsCount() error {
	// sanity check
	if lc.tracedCgroupsCount <= 1 {
		return nil
	}
	lc.tracedCgroupsCount--

	// push new value to kernel space
	if err := lc.pushTracedCgroupsCount(); err != nil {
		return err
	}

	// send metric
	return lc.sendLoadControllerTriggeredMetric([]string{"reduction:traced_cgroups_count"})
}

// pushTracedCgroupsCount pushes the current traced cgroups count to kernel space
func (lc *ActivityDumpLoadController) pushTracedCgroupsCount() error {
	return retry.Do(lc.editCgroupsCounter(func(counter *tracedCgroupsCounter) error {
		log.Debugf("AD: got counter = %v, when propagating config", counter)
		counter.Max = lc.tracedCgroupsCount
		return nil
	}))
}

func (lc *ActivityDumpLoadController) releaseTracedCgroupSpot() error {
	return retry.Do(lc.editCgroupsCounter(func(counter *tracedCgroupsCounter) error {
		if counter.Counter > 0 {
			counter.Counter--
		}
		return nil
	}))
}

type cgroupsCounterEditor = func(*tracedCgroupsCounter) error

func (lc *ActivityDumpLoadController) editCgroupsCounter(editor cgroupsCounterEditor) func() error {
	return func() error {
		if err := lc.tracedCgroupsLockMap.Update(ebpfutils.ZeroUint32MapItem, uint32(1), ebpf.UpdateNoExist); err != nil {
			return fmt.Errorf("failed to lock traced cgroup counter: %w", err)
		}

		defer func() {
			if err := lc.tracedCgroupsLockMap.Delete(ebpfutils.ZeroUint32MapItem); err != nil {
				log.Errorf("failed to unlock traced cgroup counter: %v", err)
			}
		}()

		var counter tracedCgroupsCounter
		if err := lc.tracedCgroupsCounterMap.Lookup(ebpfutils.ZeroUint32MapItem, &counter); err != nil {
			return fmt.Errorf("failed to get traced cgroup counter: %w", err)
		}

		if err := editor(&counter); err != nil {
			return err
		}

		if err := lc.tracedCgroupsCounterMap.Put(ebpfutils.ZeroUint32MapItem, counter); err != nil {
			return fmt.Errorf("failed to change counter max: %w", err)
		}
		return nil
	}
}

func (lc *ActivityDumpLoadController) sendLoadControllerTriggeredMetric(tags []string) error {
	if err := lc.adm.probe.statsdClient.Count(metrics.MetricActivityDumpLoadControllerTriggered, 1, tags, 1.0); err != nil {
		return fmt.Errorf("couldn't send %s metric: %v", metrics.MetricActivityDumpLoadControllerTriggered, err)
	}
	return nil
}
