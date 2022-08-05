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

// ActivityDumpLCConfig represents the dynamic configuration managed by the load controller
type ActivityDumpLCConfig struct {
	tracedEventTypes   []model.EventType
	tracedCgroupsCount uint64
	dumpTimeout        time.Duration
}

// NewActivityDumpLCConfig returns a new dynamic config from user config
func NewActivityDumpLCConfig(cfg *config.Config) *ActivityDumpLCConfig {
	tracedCgroupsCount := uint64(cfg.ActivityDumpTracedCgroupsCount)
	if tracedCgroupsCount > probes.MaxTracedCgroupsCount {
		tracedCgroupsCount = probes.MaxTracedCgroupsCount
	}

	return &ActivityDumpLCConfig{
		tracedEventTypes:   cfg.ActivityDumpTracedEventTypes,
		tracedCgroupsCount: tracedCgroupsCount,
		dumpTimeout:        cfg.ActivityDumpCgroupDumpTimeout,
	}
}

const minDumpTimeout = 10 * time.Minute

func (lcCfg *ActivityDumpLCConfig) reduced() *ActivityDumpLCConfig {
	// first we try reducing the amount of concurrently traced cgroups
	if lcCfg.tracedCgroupsCount > 1 {
		return &ActivityDumpLCConfig{
			tracedEventTypes:   lcCfg.tracedEventTypes,
			tracedCgroupsCount: lcCfg.tracedCgroupsCount - 1,
			dumpTimeout:        lcCfg.dumpTimeout,
		}
	}

	// then we try to reduce the timeout
	if lcCfg.dumpTimeout > minDumpTimeout {
		newTimeout := lcCfg.dumpTimeout * 3 / 4 // reduce by 25%
		if newTimeout < minDumpTimeout {
			newTimeout = minDumpTimeout
		}
		return &ActivityDumpLCConfig{
			tracedEventTypes:   lcCfg.tracedEventTypes,
			tracedCgroupsCount: lcCfg.tracedCgroupsCount,
			dumpTimeout:        newTimeout,
		}
	}

	// finally, as a last resort, we try removing file events
	newEventTypes := make([]model.EventType, 0, len(lcCfg.tracedEventTypes))
	for _, et := range lcCfg.tracedEventTypes {
		if et != model.FileOpenEventType {
			newEventTypes = append(newEventTypes, et)
		}
	}
	return &ActivityDumpLCConfig{
		tracedEventTypes:   newEventTypes,
		tracedCgroupsCount: lcCfg.tracedCgroupsCount,
		dumpTimeout:        lcCfg.dumpTimeout,
	}
}

// ActivityDumpLoadController is a load controller allowing dynamic change of Activity Dump configuration
type ActivityDumpLoadController struct {
	originalConfig *ActivityDumpLCConfig
	currentConfig  *ActivityDumpLCConfig

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

	dumpTimeoutMap, found, err := man.GetMap("ad_dump_timeout")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("couldn't find ad_dump_timeout map")
	}

	return &ActivityDumpLoadController{
		originalConfig: NewActivityDumpLCConfig(cfg),

		tracedEventTypesMap:     tracedEventTypesMap,
		tracedCgroupsCounterMap: tracedCgroupsCounterMap,
		tracedCgroupsLockMap:    tracedCgroupsLockMap,
		dumpTimeoutMap:          dumpTimeoutMap,
	}, nil
}

func (lc *ActivityDumpLoadController) getCurrentConfig() *ActivityDumpLCConfig {
	if lc.currentConfig != nil {
		return lc.currentConfig
	}
	return lc.originalConfig
}

func (lc *ActivityDumpLoadController) reduceConfig() {
	lcCfg := lc.getCurrentConfig()
	newCfg := lcCfg.reduced()
	lc.currentConfig = newCfg
}

func (lc *ActivityDumpLoadController) propagateLoadSettings() error {
	return retry.Do(lc.propagateLoadSettingsRaw)
}

func (lc *ActivityDumpLoadController) propagateLoadSettingsRaw() error {
	lcConfig := lc.getCurrentConfig()

	// traced event types
	isTraced := uint64(1)
	for _, evtType := range lcConfig.tracedEventTypes {
		if err := lc.tracedEventTypesMap.Put(evtType, isTraced); err != nil {
			return fmt.Errorf("failed to insert traced event type: %w", err)
		}
	}

	// dump timeout
	if err := lc.dumpTimeoutMap.Put(ebpfutils.ZeroUint32MapItem, uint64(lcConfig.dumpTimeout.Nanoseconds())); err != nil {
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

	counter.Max = lcConfig.tracedCgroupsCount
	if err := lc.tracedCgroupsCounterMap.Put(ebpfutils.ZeroUint32MapItem, counter); err != nil {
		return fmt.Errorf("failed to change counter max: %w", err)
	}

	return nil
}
