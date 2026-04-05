// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package containers has logic extracting network information from containers (currently, resolv.conf)
package containers

import (
	"context"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/events"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	resolvConfInputMaxSizeBytes = 4096
	resolvConfMaxSizeBytes      = 1024
	// the queue is oversized to account for the procfs snapshot when system-probe starts.
	// these are pointers, so the memory cost is not severe
	maxProcessQueueLen = 2048
	moduleName         = "network_tracer__containerStore"

	// containerTTL: when to evict a container that hasn't been accessed
	containerTTL = 10 * time.Minute
	// cleanerInterval: how often to evict old containers
	cleanerInterval = 5 * time.Minute
	// errorRetryInterval: how long to wait before retrying a failed read
	errorRetryInterval = time.Minute
)

var containerStoreTelemetry = struct {
	capacityEvictions telemetry.Counter
	eventsDropped     telemetry.Counter
	readFailures      telemetry.Counter
}{
	telemetryimpl.GetCompatComponent().NewCounter(moduleName, "capacity_evictions", []string{}, "Counter measuring the number of LRU capacity evictions of non-expired containers"),
	telemetryimpl.GetCompatComponent().NewCounter(moduleName, "events_dropped", []string{}, "Counter measuring the number of dropped process events"),
	telemetryimpl.GetCompatComponent().NewCounter(moduleName, "read_failures", []string{}, "Counter measuring the number of failures to read container data such as resolv.conf"),
}

type containerStoreItem struct {
	timestamp  time.Time
	resolvConf network.ResolvConf
}

// ContainerStore reads container data (currently just resolv.conf) into a hashmap
// which is later attached to connections via the ResolvConf field
type ContainerStore struct {
	ctx       context.Context
	cancelCtx func()

	// mu protects timestamp updates in GetResolvConf from racing with
	// expiry checks in cleanMap.
	mu    sync.Mutex
	cache *lru.Cache[network.ContainerID, containerStoreItem]

	in chan *events.Process

	warnLimit  *log.Limit
	errorLimit *log.Limit
	debugLimit *log.Limit

	containerReader containerReader

	readContainerItem func(ctx context.Context, entry *events.Process) (readContainerItemResult, error)
}

func (csi containerStoreItem) isExpired() bool {
	return time.Since(csi.timestamp) > containerTTL
}

// NewContainerStore initializes the container store
func NewContainerStore(maxContainers int) (*ContainerStore, error) {
	warnLimit := log.NewLogLimit(5, 10*time.Minute)
	errorLimit := log.NewLogLimit(5, 10*time.Minute)
	debugLimit := log.NewLogLimit(10, time.Minute)

	cache, err := lru.NewWithEvict(maxContainers, func(key *intern.Value, item containerStoreItem) {
		if log.ShouldLog(log.DebugLvl) && debugLimit.ShouldLog() {
			logEvictingID(key)
		}
		if !item.isExpired() {
			containerStoreTelemetry.capacityEvictions.Add(1)
			if warnLimit.ShouldLog() {
				log.Warnf("CNM ContainerStore capacity eviction of non-expired container %s", containerIDStr(key))
			}
		}
	})
	if err != nil {
		return nil, err
	}

	ctx, cancelCtx := context.WithCancel(context.Background())

	cs := &ContainerStore{
		ctx:       ctx,
		cancelCtx: cancelCtx,

		cache: cache,
		in:    make(chan *events.Process, maxProcessQueueLen),

		warnLimit:  warnLimit,
		errorLimit: errorLimit,
		debugLimit: debugLimit,

		containerReader: newContainerReader(makeResolvStripper(resolvConfInputMaxSizeBytes)),
	}
	// this function is only ever replaced in tests for mocking purposes
	cs.readContainerItem = cs.containerReader.readContainerItem

	cleanTicker := time.NewTicker(cleanerInterval)

	go func() {
		defer cleanTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-cleanTicker.C:
				cs.cleanMap()
			case p := <-cs.in:
				cs.addProcess(p)
			}
		}
	}()

	return cs, nil
}

// HandleProcessEvent passes a process event from CWS into a channel for
// later processing (to avoid blocking CWS)
func (cs *ContainerStore) HandleProcessEvent(entry *events.Process) {
	select {
	case <-cs.ctx.Done():
	case cs.in <- entry:
	default:
		if cs.warnLimit.ShouldLog() {
			log.Warnf("CNM ContainerStore dropped a process event (too many in queue)")
		}
		containerStoreTelemetry.eventsDropped.Inc()
	}
}

func (cs *ContainerStore) addProcess(entry *events.Process) {
	prevItem, ok := cs.cache.Get(entry.ContainerID)
	if ok {
		if prevItem.resolvConf != nil {
			// we already have resolv.conf for this container, no need to re-read
			return
		}
		// previous read failed (resolvConf is nil); only retry after errorRetryInterval
		if time.Since(prevItem.timestamp) < errorRetryInterval {
			return
		}
	}

	result, err := cs.readContainerItem(cs.ctx, entry)
	if log.ShouldLog(log.DebugLvl) && cs.debugLimit.ShouldLog() {
		logHandledID(entry.ContainerID, result, err)
	}
	if cs.ctx.Err() != nil {
		return
	}
	if err != nil {
		if cs.errorLimit.ShouldLog() {
			log.Errorf("CNM ContainerStore failed to readContainerItem: %s", err)
		}
		containerStoreTelemetry.readFailures.Add(1)

		// remember the failure so that we don't spam reading
		cs.cache.Add(entry.ContainerID, containerStoreItem{
			timestamp: time.Now(),
		})

		return
	}
	if result.noDataReason != "" {
		return
	}

	cs.cache.Add(entry.ContainerID, result.item)
}

func containerIDStr(containerID network.ContainerID) string {
	if containerID != nil {
		if s, ok := containerID.Get().(string); ok {
			return s
		}
	}
	return "host"
}

// logEvictingID logs in a separate function to avoid allocation
func logEvictingID(containerID network.ContainerID) {
	log.Debugf("CNM ContainerStore evicting ID %s", containerIDStr(containerID))
}

// logHandledID logs in a separate function to avoid allocation
func logHandledID(containerID network.ContainerID, result readContainerItemResult, err error) {
	log.Debugf("CNM ContainerStore handled ID=%s with result=%s, err=%v", containerIDStr(containerID), result.String(), err)
}

func (cs *ContainerStore) cleanMap() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	for _, containerID := range cs.cache.Keys() {
		item, ok := cs.cache.Get(containerID)
		if !ok {
			continue
		}
		if item.isExpired() {
			cs.cache.Remove(containerID)
		}
	}
}

// Stop stops the ContainerStore from running
func (cs *ContainerStore) Stop() {
	cs.cancelCtx()
}

// GetResolvConf returns the resolv.conf for a containerID.
// Accessing an entry refreshes its timestamp, preventing expiry for active containers.
func (cs *ContainerStore) GetResolvConf(containerID network.ContainerID) network.ResolvConf {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	item, ok := cs.cache.Get(containerID)
	if !ok || item.resolvConf == nil {
		return nil
	}
	item.timestamp = time.Now()
	cs.cache.Add(containerID, item)
	return item.resolvConf
}

// GetResolvConfMap scans a slice of connections for containers and returns
// a mapping to resolv.conf
func (cs *ContainerStore) GetResolvConfMap(conns []network.ConnectionStats) map[network.ContainerID]network.ResolvConf {
	allContainers := make(map[network.ContainerID]struct{})
	resolvConfs := make(map[network.ContainerID]network.ResolvConf)
	for i := range conns {
		// if containerID is nil, this represents the host
		containerID := conns[i].ContainerID.Source
		allContainers[containerID] = struct{}{}
		if _, ok := resolvConfs[containerID]; ok {
			continue
		}

		resolvConf := cs.GetResolvConf(containerID)

		if resolvConf != nil {
			resolvConfs[containerID] = resolvConf
		}
	}

	// we know this one will only run once per 30s connections check, so no need to use log.Limit
	log.Debugf("CNM ContainerStore mapped %d out of %d containerIDs", len(resolvConfs), len(allContainers))

	return resolvConfs
}
