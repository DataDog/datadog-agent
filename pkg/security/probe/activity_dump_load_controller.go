// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/avast/retry-go"
	"github.com/cilium/ebpf"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	ebpfutils "github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// TracedEventTypesReductionOrder is the order by which event types are reduced
	TracedEventTypesReductionOrder = []model.EventType{model.FileOpenEventType, model.SyscallsEventType, model.DNSEventType, model.BindEventType}
	// MinDumpTimeout is the shortest timeout for a dump
	MinDumpTimeout = 10 * time.Minute
)

// ActivityDumpLCConfig represents the dynamic configuration managed by the load controller
type ActivityDumpLCConfig struct {
	// dynamic
	tracedEventTypes   []model.EventType
	tracedCgroupsCount uint64
	dumpTimeout        time.Duration

	// static
	cgroupWaitListSize int
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

		cgroupWaitListSize: cfg.ActivityDumpCgroupWaitListSize,
	}
}

// ActivityDumpLoadController is a load controller allowing dynamic change of Activity Dump configuration
type ActivityDumpLoadController struct {
	rateLimiter  *rate.Limiter
	config       *ActivityDumpLCConfig
	statsdClient statsd.ClientInterface

	tracedEventTypesMap     *ebpf.Map
	tracedCgroupsCounterMap *ebpf.Map
	tracedCgroupsLockMap    *ebpf.Map
	dumpTimeoutMap          *ebpf.Map
}

// NewActivityDumpLoadController returns a new activity dump load controller
func NewActivityDumpLoadController(cfg *config.Config, man *manager.Manager, client statsd.ClientInterface) (*ActivityDumpLoadController, error) {
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

	lcConfig := NewActivityDumpLCConfig(cfg)

	return &ActivityDumpLoadController{
		// 1 every timeout, otherwise we do not have time to see real effects from the reduction
		rateLimiter: rate.NewLimiter(rate.Every(lcConfig.dumpTimeout), 1),
		config:      lcConfig,

		tracedEventTypesMap:     tracedEventTypesMap,
		tracedCgroupsCounterMap: tracedCgroupsCounterMap,
		tracedCgroupsLockMap:    tracedCgroupsLockMap,
		dumpTimeoutMap:          dumpTimeoutMap,
		statsdClient:            client,
	}, nil
}

// PushCurrentConfig pushes the current load controller config to kernel space
func (lc *ActivityDumpLoadController) PushCurrentConfig() error {
	if err := lc.pushTracedCgroupsCount(); err != nil {
		return err
	}

	if err := lc.pushDumpTimeout(); err != nil {
		return err
	}

	if err := lc.pushTracedEventTypes(); err != nil {
		return err
	}
	return nil
}

func (lc *ActivityDumpLoadController) getCgroupWaitTimeout() time.Duration {
	return lc.config.dumpTimeout * time.Duration(lc.config.cgroupWaitListSize)
}

// reduceConfig reduces the configuration of the load controller.
func (lc *ActivityDumpLoadController) reduceConfig() error {
	if !lc.rateLimiter.Allow() {
		return nil
	}

	// try to reduce the number of concurrent dumps first
	if lc.config.tracedCgroupsCount > 1 {
		return lc.reduceTracedCgroupsCount()
	}

	// next up, try to reduce the dumps timeout
	if lc.config.dumpTimeout > MinDumpTimeout {
		return lc.reduceDumpTimeout()
	}

	// finally, remove an event type
	return lc.reduceTracedEventTypes()
}

// reduceTracedCgroupsCount decrements the maximum count of cgroups that can be traced simultaneously and applies the
// updated value to kernel space.
func (lc *ActivityDumpLoadController) reduceTracedCgroupsCount() error {
	// sanity check
	if lc.config.tracedCgroupsCount <= 1 {
		return nil
	}
	lc.config.tracedCgroupsCount--

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
		counter.Max = lc.config.tracedCgroupsCount
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

// reduceDumpTimeout reduces the dump timeout configuration and applies the updated value to kernel space
func (lc *ActivityDumpLoadController) reduceDumpTimeout() error {
	newTimeout := lc.config.dumpTimeout * 3 / 4 // reduce by 25%
	if newTimeout < MinDumpTimeout {
		newTimeout = MinDumpTimeout
	}
	lc.config.dumpTimeout = newTimeout
	lc.rateLimiter.SetLimit(rate.Every(lc.config.dumpTimeout))

	// push new value to kernel space
	if err := lc.pushDumpTimeout(); err != nil {
		return nil
	}

	// send metric
	return lc.sendLoadControllerTriggeredMetric([]string{"reduction:dump_timeout"})
}

// pushDumpTimeout pushes the current dump timeout to kernel space
func (lc *ActivityDumpLoadController) pushDumpTimeout() error {
	if err := lc.dumpTimeoutMap.Put(ebpfutils.ZeroUint32MapItem, uint64(lc.config.dumpTimeout.Nanoseconds())); err != nil {
		return fmt.Errorf("failed to update dump timeout: %w", err)
	}
	return nil
}

// reduceTracedEventTypes removes an event type from the list of traced events types and updates the list of enabled
// event types in kernel space
func (lc *ActivityDumpLoadController) reduceTracedEventTypes() error {
	var reducedEventType model.EventType

reductionOrder:
	for _, et := range TracedEventTypesReductionOrder {
		for i, tracedEt := range lc.config.tracedEventTypes {
			if et == tracedEt {
				reducedEventType = et
				lc.config.tracedEventTypes = append(lc.config.tracedEventTypes[:i], lc.config.tracedEventTypes[i+1:]...)
				break reductionOrder
			}
		}
	}

	// delete event type in kernel space filter
	if err := lc.tracedEventTypesMap.Delete(reducedEventType); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
		return fmt.Errorf("failed to delete old traced event type %s: %w", reducedEventType, err)
	}

	// send metric
	return lc.sendLoadControllerTriggeredMetric([]string{"reduction:traced_event_types", "event_type:" + reducedEventType.String()})
}

// pushTracedEventTypes pushes the list of traced event types to kernel space
func (lc *ActivityDumpLoadController) pushTracedEventTypes() error {
	// init traced event types
	isTraced := uint64(1)
	for _, evtType := range lc.config.tracedEventTypes {
		if err := lc.tracedEventTypesMap.Put(evtType, isTraced); err != nil {
			return fmt.Errorf("failed to insert traced event type: %w", err)
		}
	}
	return nil
}

func (lc *ActivityDumpLoadController) sendLoadControllerTriggeredMetric(tags []string) error {
	if err := lc.statsdClient.Count(metrics.MetricActivityDumpLoadControllerTriggered, 1, tags, 1.0); err != nil {
		return fmt.Errorf("couldn't send %s metric: %v", metrics.MetricActivityDumpLoadControllerTriggered, err)
	}
	return nil
}
