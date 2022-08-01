// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	ebpfutils "github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
)

type ActivityDumpLoadController struct {
	tracedEventTypes   []model.EventType
	tracedCgroupsCount uint64

	tracedEventTypesMap     *ebpf.Map
	tracedCgroupsCounterMap *ebpf.Map
	tracedCgroupsLockMap    *ebpf.Map
}

func NewActivityDumpLoadController(cfg *config.Config, man *manager.Manager) (*ActivityDumpLoadController, error) {
	tracedEventTypesMap, found, err := man.GetMap("traced_event_types")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("couldn't find traced_event_types map")
	}

	tracedCgroupsCounterMap, found, err := man.GetMap("traced_cgroups_counter")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("couldn't find traced_cgroups_counter map")
	}

	tracedCgroupsLockMap, found, err := man.GetMap("traced_cgroups_lock")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("couldn't find traced_cgroups_lock map")
	}

	return &ActivityDumpLoadController{
		tracedEventTypes:   cfg.ActivityDumpTracedEventTypes,
		tracedCgroupsCount: uint64(cfg.ActivityDumpTracedCgroupsCount),

		tracedEventTypesMap:     tracedEventTypesMap,
		tracedCgroupsCounterMap: tracedCgroupsCounterMap,
		tracedCgroupsLockMap:    tracedCgroupsLockMap,
	}, nil
}

func (lc *ActivityDumpLoadController) propagateLoadSettings() error {
	// init traced event types
	isTraced := uint64(1)
	for _, evtType := range lc.tracedEventTypes {
		if err := lc.tracedEventTypesMap.Put(evtType, isTraced); err != nil {
			return fmt.Errorf("failed to insert traced event type: %w", err)
		}
	}

	if err := lc.tracedCgroupsLockMap.Put(ebpfutils.ZeroUint32MapItem, uint32(1)); err != nil {
		return fmt.Errorf("failed to lock traced cgroup counter: %w", err)
	}

	defer func() {
		if err := lc.tracedCgroupsLockMap.Put(ebpfutils.ZeroUint32MapItem, uint32(0)); err != nil {
			log.Errorf("failed to unlock traced cgroup counter: %v", err)
		}
	}()

	var counter tracedCgroupsCounter
	if err := lc.tracedCgroupsCounterMap.Lookup(ebpfutils.ZeroUint32MapItem, &counter); err != nil {
		return fmt.Errorf("failed to get traced cgroup counter: %w", err)
	}
	log.Debugf("AD: got counter = %v, when propagating config", counter)

	counter.Max = lc.tracedCgroupsCount
	if err := lc.tracedCgroupsCounterMap.Put(ebpfutils.ZeroUint32MapItem, counter); err != nil {
		return fmt.Errorf("failed to change counter max: %w", err)
	}

	return nil
}
