// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package process implements the process collector for Workloadmeta.
package process

import (
	"context"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/benbjohnson/clock"
	"go.uber.org/fx"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID   = "process-collector"
	componentName = "workloadmeta-process"
)

type collector struct {
	id                     string
	store                  workloadmeta.Component
	catalog                workloadmeta.AgentType
	clock                  clock.Clock
	processProbe           procutil.Probe
	processEventsCh        chan *Event
	lastCollectedProcesses map[int32]*procutil.Process
}

// Event is a message type used to communicate with the stream function asynchronously
type Event struct {
	Created []*workloadmeta.Process
	Deleted []*workloadmeta.Process
}

func newProcessCollector(id string, catalog workloadmeta.AgentType, clock clock.Clock, processProbe procutil.Probe) collector {
	return collector{
		id:                     id,
		catalog:                catalog,
		clock:                  clock,
		processProbe:           processProbe,
		processEventsCh:        make(chan *Event),
		lastCollectedProcesses: make(map[int32]*procutil.Process),
	}
}

// NewProcessCollectorProvider returns a new process collector provider and an error.
// Currently, this is only used on Linux when language detection and run in core agent are enabled.
func NewProcessCollectorProvider() (workloadmeta.CollectorProvider, error) {
	collector := newProcessCollector(collectorID, workloadmeta.NodeAgent, clock.New(), procutil.NewProcessProbe())
	return workloadmeta.CollectorProvider{
		Collector: &collector,
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewProcessCollectorProvider)
}

// isEnabled returns a boolean indicating if the process collector is enabled and what collection interval to use if it is.
func (c *collector) isEnabled() bool {
	// TODO: implement the logic to check if the process collector is enabled based on dependent configs (process collection, language detection, service discovery)
	// hardcoded to false until the new collector has all functionality/consolidation completed (service discovery, language collection, etc)
	return false
}

// collectionIntervalConfig returns the configured collection interval
func (c *collector) collectionIntervalConfig() time.Duration {
	// TODO: read configured collection interval once implemented
	return time.Second * 10
}

// Start starts the collector. The collector should run until the context
// is done. It also gets a reference to the store that started it so it
// can use Notify, or get access to other entities in the store.
func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	if c.isEnabled() {
		c.store = store
		go c.collect(ctx, c.clock.Ticker(c.collectionIntervalConfig()))
		go c.stream(ctx)
	} else {
		return errors.NewDisabled(componentName, "process collection is disabled")
	}

	return nil
}

// processCacheDifference returns new processes that exist in procCacheA and not in procCacheB
func processCacheDifference(procCacheA map[int32]*procutil.Process, procCacheB map[int32]*procutil.Process) []*procutil.Process {
	// attempt to pre-allocate right slice size to reduce number of slice growths
	diffSize := 0
	if len(procCacheA) > len(procCacheB) {
		diffSize = len(procCacheA) - len(procCacheB)
	} else {
		diffSize = len(procCacheB) - len(procCacheA)
	}
	newProcs := make([]*procutil.Process, 0, diffSize)
	for pid, procA := range procCacheA {
		procB, exists := procCacheB[pid]

		// new process
		if !exists {
			newProcs = append(newProcs, procA)
		} else if procB.Stats.CreateTime != procA.Stats.CreateTime {
			// same process PID exists, but different process due to creation time
			newProcs = append(newProcs, procA)
		}
	}
	return newProcs
}

// collect captures all the required process data for the process check
func (c *collector) collect(ctx context.Context, collectionTicker *clock.Ticker) {
	// TODO: implement the full collection logic for the process collector. Once collection is done, submit events.
	ctx, cancel := context.WithCancel(ctx)
	defer collectionTicker.Stop()
	defer cancel()
	for {
		select {
		case <-collectionTicker.C:
			// fetch process data and submit events to streaming channel for asynchronous processing
			procs, err := c.processProbe.ProcessesByPID(time.Now(), false)
			if err != nil {
				log.Errorf("Error getting processes by pid: %v", err)
				return
			}

			// categorize the processes into events for workloadmeta
			createdProcs := processCacheDifference(procs, c.lastCollectedProcesses)
			wlmCreatedProcs := make([]*workloadmeta.Process, len(createdProcs))
			for i, proc := range createdProcs {
				wlmCreatedProcs[i] = processToWorkloadMetaProcess(proc)
			}

			deletedProcs := processCacheDifference(c.lastCollectedProcesses, procs)
			wlmDeletedProcs := make([]*workloadmeta.Process, len(deletedProcs))
			for i, proc := range deletedProcs {
				wlmDeletedProcs[i] = processToWorkloadMetaProcess(proc)
			}

			// send these events to the channel
			c.processEventsCh <- &Event{
				Created: wlmCreatedProcs,
				Deleted: wlmDeletedProcs,
			}

			// store latest collected processes
			c.lastCollectedProcesses = procs
		case <-ctx.Done():
			log.Infof("The %s collector has stopped", collectorID)
			return
		}
	}
}

// stream processes events sent from data collection and notifies WorkloadMeta that updates have occurred
func (c *collector) stream(ctx context.Context) {
	healthCheck := health.RegisterLiveness(componentName)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		select {
		case <-healthCheck.C:

		case processEvent := <-c.processEventsCh:
			events := make([]workloadmeta.CollectorEvent, 0, len(processEvent.Deleted)+len(processEvent.Created))
			for _, proc := range processEvent.Deleted {
				events = append(events, workloadmeta.CollectorEvent{
					Type:   workloadmeta.EventTypeUnset,
					Entity: proc,
					Source: workloadmeta.SourceProcessCollector,
				})
			}

			for _, proc := range processEvent.Created {
				events = append(events, workloadmeta.CollectorEvent{
					Type:   workloadmeta.EventTypeSet,
					Entity: proc,
					Source: workloadmeta.SourceProcessCollector,
				})
			}

			c.store.Notify(events)

		case <-ctx.Done():
			err := healthCheck.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}
			return
		}
	}
}

// processToWorkloadMetaProcess maps a procutil process to a workloadmeta process
func processToWorkloadMetaProcess(process *procutil.Process) *workloadmeta.Process {
	return &workloadmeta.Process{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindProcess,
			ID:   strconv.Itoa(int(process.Pid)),
		},
		Pid:          process.Pid,
		NsPid:        process.NsPid,
		Ppid:         process.Ppid,
		Name:         process.Name,
		Cwd:          process.Cwd,
		Exe:          process.Exe,
		Comm:         process.Comm,
		Cmdline:      process.Cmdline,
		Uids:         process.Uids,
		Gids:         process.Gids,
		CreationTime: time.UnixMilli(process.Stats.CreateTime).UTC(),
	}
}

// Pull triggers an entity collection. To be used by collectors that
// don't have streaming functionality, and called periodically by the
// store. This is not needed for the process collector.
func (c *collector) Pull(_ context.Context) error {
	return nil
}

// GetID returns the identifier for the respective component.
func (c *collector) GetID() string {
	return c.id
}

// GetTargetCatalog gets the expected catalog.
func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}
