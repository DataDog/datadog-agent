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

	"github.com/DataDog/datadog-agent/pkg/security/config"
	ebpfutils "github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/avast/retry-go"
	"github.com/cilium/ebpf"
)

// ActivityDumpLoadController is a load controller allowing dynamic change of Activity Dump configuration
type ActivityDumpLoadController struct {
	tracedEventTypes   []model.EventType
	tracedCgroupsCount uint64
	dumpTimeout        time.Duration

	tracedEventTypesMap     *ebpf.Map
	tracedCgroupsCounterMap *ebpf.Map
	tracedCgroupsLockMap    *ebpf.Map
	dumpTimeoutMap          *ebpf.Map
}

// NewActivityDumpLoadController returns a new activity dump load controller
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

	tracedCgroupsCount := uint64(cfg.ActivityDumpTracedCgroupsCount)
	if tracedCgroupsCount > probes.MaxTracedCgroupsCount {
		tracedCgroupsCount = probes.MaxTracedCgroupsCount
	}

	dumpTimeoutMap, found, err := man.GetMap("ad_dump_timeout")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("couldn't find ad_dump_timeout map")
	}

	return &ActivityDumpLoadController{
		tracedEventTypes:   cfg.ActivityDumpTracedEventTypes,
		tracedCgroupsCount: tracedCgroupsCount,
		dumpTimeout:        cfg.ActivityDumpCgroupDumpTimeout,

		tracedEventTypesMap:     tracedEventTypesMap,
		tracedCgroupsCounterMap: tracedCgroupsCounterMap,
		tracedCgroupsLockMap:    tracedCgroupsLockMap,
		dumpTimeoutMap:          dumpTimeoutMap,
	}, nil
}

func (lc *ActivityDumpLoadController) propagateLoadSettings() error {
	return retry.Do(lc.propagateLoadSettingsRaw)
}

func (lc *ActivityDumpLoadController) propagateLoadSettingsRaw() error {
	// traced event types
	isTraced := uint64(1)
	for _, evtType := range lc.tracedEventTypes {
		if err := lc.tracedEventTypesMap.Put(evtType, isTraced); err != nil {
			return fmt.Errorf("failed to insert traced event type: %w", err)
		}
	}

	// dump timeout
	if err := lc.dumpTimeoutMap.Put(ebpfutils.ZeroUint32MapItem, uint64(lc.dumpTimeout.Nanoseconds())); err != nil {
		return fmt.Errorf("failed to update dump timeout: %w", err)
	}

	// traced cgroups count
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
	log.Debugf("AD: got counter = %v, when propagating config", counter)

	counter.Max = lc.tracedCgroupsCount
	if err := lc.tracedCgroupsCounterMap.Put(ebpfutils.ZeroUint32MapItem, counter); err != nil {
		return fmt.Errorf("failed to change counter max: %w", err)
	}

	return nil
}
