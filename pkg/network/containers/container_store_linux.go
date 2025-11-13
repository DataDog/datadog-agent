// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package containers has logic extracting network information from containers (currently, resolv.conf)
package containers

import (
	"context"
	"errors"
	"fmt"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	"go4.org/intern"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/events"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// containerItemNoDataError is returned when readContainerItem() didn't fail, but doesn't
// have data to share. This can happen when the process exited at the same time, or on unsupported platforms
type containerItemNoDataError struct {
	Err error
}

func (e *containerItemNoDataError) Error() string {
	return fmt.Sprintf("readContainerItem() doesn't have data: %s", e.Err)
}
func (e *containerItemNoDataError) Unwrap() error {
	return e.Err
}

const (
	resolvConfMaxSizeBytes = 1024
	maxProcessQueueLen     = 100
	moduleName             = "network_tracer__containerStore"

	// containerStaleTime: when to re-fetch a container's resolv.conf
	containerStaleTime = 2 * time.Minute
	// containerTTL: when to evict a container
	containerTTL = 5 * time.Minute
	// cleanerInterval: how often to evict old containers
	cleanerInterval = 2 * time.Minute
)

var containerStoreTelemetry = struct {
	liveEvictions telemetry.Counter
	eventsDropped telemetry.Counter
	readFailures  telemetry.Counter
}{
	telemetry.NewCounter(moduleName, "live_evicts", []string{}, "Counter measuring the number of evictions of live containers in the container store (this is bad)"),
	telemetry.NewCounter(moduleName, "events_dropped", []string{}, "Counter measuring the number of dropped process events"),
	telemetry.NewCounter(moduleName, "read_failures", []string{}, "Counter measuring the number of failures to read container data such as resolv.conf"),
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

	cache *lru.Cache[network.ContainerID, containerStoreItem]

	in chan *events.Process

	warnLimit  *log.Limit
	errorLimit *log.Limit

	readContainerItem func(ctx context.Context, entry *events.Process) (containerStoreItem, error)
}

func (csi containerStoreItem) isStale() bool {
	return time.Since(csi.timestamp) > containerStaleTime
}
func (csi containerStoreItem) isExpired() bool {
	return time.Since(csi.timestamp) > containerTTL
}

// NewContainerStore initializes the container store
func NewContainerStore(maxContainers int) (*ContainerStore, error) {
	warnLimit := log.NewLogLimit(5, 10*time.Second)
	errorLimit := log.NewLogLimit(5, 10*time.Second)

	cache, err := lru.NewWithEvict(maxContainers, func(_key *intern.Value, item containerStoreItem) {
		if !item.isStale() {
			log.Errorf("CNM ContainerStore has more than %d live containers, was forced to evict a live container", maxContainers)
			containerStoreTelemetry.liveEvictions.Add(1)
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

		readContainerItem: readContainerItem,
	}

	cleanTicker := time.NewTicker(cleanerInterval)

	go func() {
		for {
			select {
			case <-ctx.Done():
				cleanTicker.Stop()
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
	if cs.ctx.Err() != nil {
		return
	}

	select {
	case cs.in <- entry:
	default:
		if cs.warnLimit.ShouldLog() {
			log.Warn("CNM ContainerStore dropped a process event (too many in queue)")
		}
		containerStoreTelemetry.eventsDropped.Inc()
	}
}

func (cs *ContainerStore) addProcess(entry *events.Process) {
	prevItem, ok := cs.cache.Get(entry.ContainerID)
	if ok && !prevItem.isStale() {
		// we already pulled resolv.conf recently, so skip
		return
	}

	item, err := cs.readContainerItem(cs.ctx, entry)
	log.Debugf("CNM ContainerStore handled ID=%v with item=%v, err=%v", entry.ContainerID, item, err)
	var noData *containerItemNoDataError
	if cs.ctx.Err() != nil || errors.As(err, &noData) {
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

	log.Debugf("CNM ContainerStore successfully read resolv.conf of size %d", len(item.resolvConf.Get()))

	cs.cache.Add(entry.ContainerID, item)
}

func (cs *ContainerStore) cleanMap() {
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
func (cs *ContainerStore) GetResolvConf(containerID network.ContainerID) network.ResolvConf {
	item, ok := cs.cache.Get(containerID)
	if !ok {
		return nil
	}
	return item.resolvConf
}

// GetResolvConfMap scans a slice of connections for containers and returns
// a mapping to resolv.conf
func (cs *ContainerStore) GetResolvConfMap(conns []network.ConnectionStats) map[network.ContainerID]network.ResolvConf {
	allContainers := make(map[network.ContainerID]struct{})
	resolvConfs := make(map[network.ContainerID]network.ResolvConf)
	for i := range conns {
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

	log.Debugf("CNM ContainerStore mapped %d out of %d containerIDs", len(resolvConfs), len(allContainers))

	return resolvConfs
}
