// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package dump holds dump related files
package dump

import (
	"fmt"
	"time"

	"github.com/cilium/ebpf"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

var (
	// TracedEventTypesReductionOrder is the order by which event types are reduced
	TracedEventTypesReductionOrder = []model.EventType{model.BindEventType, model.IMDSEventType, model.DNSEventType, model.SyscallsEventType, model.FileOpenEventType}

	absoluteMinimumDumpTimeout = 10 * time.Second
)

// ActivityDumpLoadController is a load controller allowing dynamic change of Activity Dump configuration
type ActivityDumpLoadController struct {
	adm            *ActivityDumpManager
	minDumpTimeout time.Duration

	// eBPF maps
	activityDumpConfigDefaults *ebpf.Map
}

// NewActivityDumpLoadController returns a new activity dump load controller
func NewActivityDumpLoadController(adm *ActivityDumpManager) (*ActivityDumpLoadController, error) {
	activityDumpConfigDefaultsMap, found, err := adm.manager.GetMap("activity_dump_config_defaults")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("couldn't find activity_dump_config_defaults map")
	}

	minDumpTimeout := adm.config.RuntimeSecurity.ActivityDumpLoadControlMinDumpTimeout
	if minDumpTimeout < absoluteMinimumDumpTimeout {
		minDumpTimeout = absoluteMinimumDumpTimeout
	}

	return &ActivityDumpLoadController{
		activityDumpConfigDefaults: activityDumpConfigDefaultsMap,
		adm:                        adm,
		minDumpTimeout:             minDumpTimeout,
	}, nil
}

func (lc *ActivityDumpLoadController) getDefaultLoadConfig() *model.ActivityDumpLoadConfig {
	defaults := NewActivityDumpLoadConfig(
		lc.adm.config.RuntimeSecurity.ActivityDumpTracedEventTypes,
		lc.adm.config.RuntimeSecurity.ActivityDumpCgroupDumpTimeout,
		0,
		lc.adm.config.RuntimeSecurity.ActivityDumpRateLimiter,
		time.Now(),
		lc.adm.resolvers.TimeResolver,
	)
	defaults.WaitListTimestampRaw = uint64(lc.adm.config.RuntimeSecurity.ActivityDumpCgroupWaitListTimeout)
	return defaults
}

// PushCurrentConfig pushes the current load controller config to kernel space
func (lc *ActivityDumpLoadController) PushCurrentConfig() error {
	// push default load config values
	if err := lc.activityDumpConfigDefaults.Put(uint32(0), lc.getDefaultLoadConfig()); err != nil {
		return fmt.Errorf("couldn't update default activity dump load config: %w", err)
	}
	return nil
}

// NextPartialDump returns a new dump with the same parameters as the current one, or with reduced load config parameters
// when applicable
func (lc *ActivityDumpLoadController) NextPartialDump(ad *ActivityDump) *ActivityDump {
	newDump := NewActivityDump(ad.adm)
	newDump.Metadata.ContainerID = ad.Metadata.ContainerID
	newDump.Metadata.DifferentiateArgs = ad.Metadata.DifferentiateArgs
	newDump.Tags = ad.Tags
	newDump.selector = ad.selector

	// copy storage requests
	for _, reqList := range ad.StorageRequests {
		for _, req := range reqList {
			newDump.AddStorageRequest(config.NewStorageRequest(
				req.Type,
				req.Format,
				req.Compression,
				req.OutputDirectory,
			))
		}
	}

	// compute the duration it took to reach the dump size threshold
	timeToThreshold := time.Since(ad.Start)

	// set new load parameters
	newDump.SetTimeout(ad.LoadConfig.Timeout - timeToThreshold)
	newDump.LoadConfig.TracedEventTypes = make([]model.EventType, len(ad.LoadConfig.TracedEventTypes))
	copy(newDump.LoadConfig.TracedEventTypes, ad.LoadConfig.TracedEventTypes)
	newDump.LoadConfig.Rate = ad.LoadConfig.Rate
	newDump.LoadConfigCookie = ad.LoadConfigCookie

	if timeToThreshold < lc.minDumpTimeout {
		if err := lc.reduceDumpRate(ad, newDump); err != nil {
			seclog.Errorf("%v", err)
		}
	}

	if timeToThreshold < lc.minDumpTimeout/2 && ad.LoadConfig.Timeout > lc.minDumpTimeout {
		if err := lc.reduceDumpTimeout(newDump); err != nil {
			seclog.Errorf("%v", err)
		}
	}

	if timeToThreshold < lc.minDumpTimeout/4 {
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
	new.LoadConfig.TracedEventTypes = new.LoadConfig.TracedEventTypes[:0]

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
		if evt != evtToRemove {
			new.LoadConfig.TracedEventTypes = append(new.LoadConfig.TracedEventTypes, evt)
		}
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
func (lc *ActivityDumpLoadController) reduceDumpTimeout(new *ActivityDump) error {
	newTimeout := new.LoadConfig.Timeout * 3 / 4 // reduce by 25%
	if minTimeout := lc.adm.config.RuntimeSecurity.ActivityDumpLoadControlMinDumpTimeout; newTimeout < minTimeout {
		newTimeout = minTimeout
	}
	new.SetTimeout(newTimeout)

	// send metric
	return lc.sendLoadControllerTriggeredMetric([]string{"reduction:dump_timeout"})
}

func (lc *ActivityDumpLoadController) sendLoadControllerTriggeredMetric(tags []string) error {
	if err := lc.adm.statsdClient.Count(metrics.MetricActivityDumpLoadControllerTriggered, 1, tags, 1.0); err != nil {
		return fmt.Errorf("couldn't send %s metric: %v", metrics.MetricActivityDumpLoadControllerTriggered, err)
	}
	return nil
}
