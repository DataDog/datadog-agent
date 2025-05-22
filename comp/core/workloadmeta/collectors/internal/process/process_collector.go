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
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	collectorID       = "process-collector"
	componentName     = "workloadmeta-process"
	cacheValidityNoRT = 2 * time.Second
)

type collector struct {
	id      string
	store   workloadmeta.Component
	catalog workloadmeta.AgentType ``

	// generates clock ticker for collection interval
	clock clock.Clock

	// fetches process data
	processProbe procutil.Probe

	// channel for async processing of events
	processEventsCh chan *ProcessEvent

	// cache of last collect processes for diff generation
	lastCollectedProcesses map[int32]*procutil.Process
}

type ProcessEvent struct {
	Created []*workloadmeta.Process
	Deleted []*workloadmeta.Process
}

func newProcessCollector(id string, store workloadmeta.Component, catalog workloadmeta.AgentType, clock clock.Clock, processProbe procutil.Probe, processEventsCh chan *ProcessEvent, lastCollectedProcesses map[int32]*procutil.Process) collector {
	return collector{
		id:                     id,
		store:                  store,
		catalog:                catalog,
		clock:                  clock,
		processProbe:           processProbe,
		processEventsCh:        processEventsCh,
		lastCollectedProcesses: lastCollectedProcesses,
	}
}

// NewProcessCollector returns a new process collector provider and an error.
// Currently, this is only used on Linux when language detection and run in core agent are enabled.
func NewProcessCollectorProvider() (workloadmeta.CollectorProvider, error) {
	collector := newProcessCollector(collectorID, nil, workloadmeta.NodeAgent, clock.New(), procutil.NewProcessProbe(), make(chan *ProcessEvent), make(map[int32]*procutil.Process))
	return workloadmeta.CollectorProvider{
		Collector: &collector,
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewProcessCollectorProvider)
}

// isEnabled returns a boolean indicating if the process collector is enabled and what collection interval to use if it is.
func (c *collector) isEnabled() (bool, time.Duration) {
	// TODO: implement the logic to check if the process collector is enabled based on dependent configs (process collection, language detection, service discovery)
	return false, time.Second * 10
}

// Start starts the collector. The collector should run until the context
// is done. It also gets a reference to the store that started it so it
// can use Notify, or get access to other entities in the store.
func (c *collector) Start(ctx context.Context, store workloadmeta.Component) error {
	// TODO: implement the start-up logic for the process collector
	// Once setup logic is complete, start collection and streaming goroutines
	enabled, collectionInterval := c.isEnabled()

	if enabled {
		c.store = store
		go c.collect(ctx, c.clock.Ticker(collectionInterval))
		go c.stream(ctx)
	}

	return nil
}

// start used for testing purposes while we wait for configuration logic to be sorted out
func (c *collector) start(ctx context.Context, store workloadmeta.Component, collectionInterval time.Duration) error {
	c.store = store
	go c.collect(ctx, c.clock.Ticker(collectionInterval))
	go c.stream(ctx)
	return nil
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
			// Fetch process data and submit events to streaming channel for asynchronous processing
			procs, err := c.processProbe.ProcessesByPID(time.Now(), false)
			if err != nil {
				log.Errorf("Error getting processes by pid: %v", err)
				return
			}

			// categorize the processes into storable events
			var createdProcesses []*workloadmeta.Process
			var deletedProcesses []*workloadmeta.Process

			// determine new processes
			for pid, proc := range procs {
				if proc == nil {
					log.Warnf("collected process %d was nil", pid)
					continue
				}
				oldProc, exists := c.lastCollectedProcesses[pid]

				// new process
				if !exists {
					createdProcesses = append(createdProcesses, processToWorkloadMetaProcess(proc))
				} else if exists && oldProc.Stats.CreateTime != proc.Stats.CreateTime {
					// new process but same PID
					createdProcesses = append(createdProcesses, processToWorkloadMetaProcess(proc))
				}
			}

			// determine deleted processes
			for pid, oldProc := range c.lastCollectedProcesses {
				if oldProc == nil {
					log.Warnf("last stored process %d was nil", pid)
					continue
				}
				proc, exists := procs[pid]

				// old process was deleted
				if !exists {
					deletedProcesses = append(deletedProcesses, processToWorkloadMetaProcess(oldProc))
				} else if exists && oldProc.Stats.CreateTime != proc.Stats.CreateTime {
					// old process but same PID
					deletedProcesses = append(deletedProcesses, processToWorkloadMetaProcess(oldProc))
				}
			}

			// send these events to the channel
			c.processEventsCh <- &ProcessEvent{
				Created: createdProcesses,
				Deleted: deletedProcesses,
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
	// TODO: implement the full streaming logic for the process collector
	health := health.RegisterLiveness(componentName)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for {
		select {
		case <-health.C:

		case processEvent := <-c.processEventsCh:
			if processEvent == nil {
				log.Warn("sent process event was nil")
				continue
			}

			// TODO: implement the logic to handle events
			var events []workloadmeta.CollectorEvent
			for _, proc := range processEvent.Created {
				events = append(events, workloadmeta.CollectorEvent{
					Type:   workloadmeta.EventTypeSet,
					Entity: proc,
					Source: workloadmeta.SourceProcessCollector,
				})
			}

			for _, proc := range processEvent.Deleted {
				events = append(events, workloadmeta.CollectorEvent{
					Type:   workloadmeta.EventTypeUnset,
					Entity: proc,
					Source: workloadmeta.SourceProcessCollector,
				})
			}

			c.store.Notify(events)

		case <-ctx.Done():
			err := health.Deregister()
			if err != nil {
				log.Warnf("error de-registering health check: %s", err)
			}
			return
		}
	}
}

// processToWorkloadMetaProcess maps a procutil process to a workloadmeta process
func processToWorkloadMetaProcess(process *procutil.Process) *workloadmeta.Process {
	if process == nil {
		return nil
	}

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
