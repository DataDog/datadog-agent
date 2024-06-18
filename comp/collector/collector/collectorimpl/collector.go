// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package collectorimpl provides the implementation of the collector component.
package collectorimpl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/atomic"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/collector/collector/collectorimpl/internal/middleware"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/status"
	metadata "github.com/DataDog/datadog-agent/comp/metadata/runner/runnerimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	pkgCollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	collectorStatus "github.com/DataDog/datadog-agent/pkg/status/collector"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	stopped uint32 = iota
	started
)

type dependencies struct {
	fx.In

	Lc     fx.Lifecycle
	Config config.Component
	Log    log.Component

	SenderManager    sender.SenderManager
	MetricSerializer optional.Option[serializer.MetricSerializer]
}

type collectorImpl struct {
	log    log.Component
	config config.Component

	senderManager    sender.SenderManager
	metricSerializer optional.Option[serializer.MetricSerializer]
	checkInstances   int64

	// state is 'started' or 'stopped'
	state *atomic.Uint32

	scheduler      *scheduler.Scheduler
	runner         *runner.Runner
	checks         map[checkid.ID]*middleware.CheckWrapper
	eventReceivers []collector.EventReceiver

	cancelCheckTimeout time.Duration

	m         sync.RWMutex
	createdAt time.Time
}

type provides struct {
	fx.Out

	Comp             collector.Component
	StatusProvider   status.InformationProvider
	MetadataProvider metadata.Provider
	ApiGetPyStatus   api.AgentEndpointProvider
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newProvides),
		fx.Provide(func(c collector.Component) optional.Option[collector.Component] {
			return optional.NewOption[collector.Component](c)
		}),
	)
}

func newProvides(deps dependencies) provides {
	c := newCollector(deps)

	var agentCheckMetadata metadata.Provider
	if _, isSet := deps.MetricSerializer.Get(); isSet {
		agentCheckMetadata = metadata.NewProvider(c.collectMetadata)
	}

	return provides{
		Comp:             c,
		StatusProvider:   status.NewInformationProvider(collectorStatus.Provider{}),
		MetadataProvider: agentCheckMetadata,
		ApiGetPyStatus:   api.NewAgentEndpointProvider(getPythonStatus, "/py/status", "GET"),
	}
}

func newCollector(deps dependencies) *collectorImpl {
	c := &collectorImpl{
		log:                deps.Log,
		config:             deps.Config,
		senderManager:      deps.SenderManager,
		metricSerializer:   deps.MetricSerializer,
		checks:             make(map[checkid.ID]*middleware.CheckWrapper),
		state:              atomic.NewUint32(stopped),
		checkInstances:     int64(0),
		cancelCheckTimeout: deps.Config.GetDuration("check_cancel_timeout"),
		createdAt:          time.Now(),
	}

	pkgCollector.InitPython(common.GetPythonPaths()...)

	deps.Lc.Append(fx.Hook{
		OnStart: c.start,
		OnStop:  c.stop,
	})

	return c
}

// AddEventReceiver adds a callback to the collector to be called each time a check is added or removed.
func (c *collectorImpl) AddEventReceiver(cb collector.EventReceiver) {
	c.m.Lock()
	defer c.m.Unlock()

	c.eventReceivers = append(c.eventReceivers, cb)
}

func (c *collectorImpl) notify(cid checkid.ID, e collector.EventType) {
	for _, cb := range c.eventReceivers {
		cb(cid, e)
	}
}

// start begins the collector's operation.  The scheduler will not run any checks until this has been called.
func (c *collectorImpl) start(_ context.Context) error {
	c.m.Lock()
	defer c.m.Unlock()

	run := runner.NewRunner(c.senderManager)
	sched := scheduler.NewScheduler(run.GetChan())

	// let the runner some visibility into the scheduler
	run.SetScheduler(sched)
	sched.Run()

	c.scheduler = sched
	c.runner = run
	c.state.Store(started)

	c.log.Debug("Collector up and running!")

	return nil
}

// stop halts any component involved in running a Check
func (c *collectorImpl) stop(_ context.Context) error {
	c.m.Lock()
	defer c.m.Unlock()

	if c.scheduler != nil {
		c.scheduler.Stop() //nolint:errcheck
		c.scheduler = nil
	}
	if c.runner != nil {
		c.runner.Stop()
		c.runner = nil
	}
	c.state.Store(stopped)
	return nil
}

// RunCheck sends a Check in the execution queue
func (c *collectorImpl) RunCheck(inner check.Check) (checkid.ID, error) {
	c.m.Lock()
	defer c.m.Unlock()

	ch := middleware.NewCheckWrapper(inner, c.senderManager)

	var emptyID checkid.ID

	if c.state.Load() != started {
		return emptyID, fmt.Errorf("the collector is not running")
	}

	if _, found := c.checks[ch.ID()]; found {
		return emptyID, fmt.Errorf("a check with ID %s is already running", ch.ID())
	}

	if err := c.scheduler.Enter(ch); err != nil {
		return emptyID, fmt.Errorf("unable to schedule the check: %s", err)
	}

	// Track the total number of checks running in order to have an appropriate number of workers
	c.checkInstances++
	if ch.Interval() == 0 {
		// Adding a temporary runner for long running check in case the
		// number of runners is lower than the number of long running
		// checks.
		c.log.Infof("Adding an extra runner for the '%s' long running check", ch)
		c.runner.AddWorker()
	} else {
		c.runner.UpdateNumWorkers(c.checkInstances)
	}

	c.checks[ch.ID()] = ch
	c.notify(ch.ID(), collector.CheckRun)
	return ch.ID(), nil
}

// StopCheck halts a check and remove the instance
func (c *collectorImpl) StopCheck(id checkid.ID) error {
	if !c.started() {
		return fmt.Errorf("the collector is not running")
	}

	ch, found := c.get(id)
	if !found {
		return fmt.Errorf("cannot find a check with ID %s", id)
	}

	// unschedule the instance
	if err := c.scheduler.Cancel(id); err != nil {
		return fmt.Errorf("an error occurred while canceling the check schedule: %s", err)
	}

	// delete check from checks map even if we encounter an error
	defer c.delete(id)

	// remove the check from the stats map
	defer expvars.RemoveCheckStats(id)

	stats, found := expvars.CheckStats(id)
	if found {
		stats.SetStateCancelling()
	}

	if err := c.runner.StopCheck(id); err != nil {
		// still attempt to cancel the check before returning the error
		_ = c.cancelCheck(ch, c.cancelCheckTimeout)
		return fmt.Errorf("an error occurred while stopping the check: %s", err)
	}

	if err := c.cancelCheck(ch, c.cancelCheckTimeout); err != nil {
		return fmt.Errorf("an error occurred while calling check.Cancel(): %s", err)
	}

	return nil
}

// cancelCheck calls Cancel on the passed check, with a timeout
func (c *collectorImpl) cancelCheck(ch check.Check, timeout time.Duration) error {
	done := make(chan struct{})

	go func() {
		ch.Cancel()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout while calling check.Cancel() on check ID %s, timeout: %s", ch.ID(), timeout)
	}
}

func (c *collectorImpl) get(id checkid.ID) (check.Check, bool) {
	c.m.RLock()
	defer c.m.RUnlock()

	ch, found := c.checks[id]
	return ch, found
}

// remove the check from the list
func (c *collectorImpl) delete(id checkid.ID) {
	c.m.Lock()
	defer c.m.Unlock()

	delete(c.checks, id)
	c.notify(id, collector.CheckStop)
}

// lightweight shortcut to see if the collector has started
func (c *collectorImpl) started() bool {
	return c.state.Load() == started
}

// MapOverChecks call the callback with the list of checks locked.
func (c *collectorImpl) MapOverChecks(cb func([]check.Info)) {
	c.m.RLock()
	defer c.m.RUnlock()

	cInfo := make([]check.Info, 0, len(c.checks))
	for _, c := range c.checks {
		cInfo = append(cInfo, c)
	}
	cb(cInfo)
}

// GetChecks copies checks
func (c *collectorImpl) GetChecks() []check.Check {
	c.m.RLock()
	defer c.m.RUnlock()

	chks := make([]check.Check, 0, len(c.checks))
	for _, chck := range c.checks {
		chks = append(chks, chck)
	}

	return chks
}

// GetAllInstanceIDs returns the ID's of all instances of a check
func (c *collectorImpl) GetAllInstanceIDs(checkName string) []checkid.ID {
	c.m.RLock()
	defer c.m.RUnlock()

	instances := []checkid.ID{}
	for id, check := range c.checks {
		if check.String() == checkName {
			instances = append(instances, id)
		}
	}

	return instances
}

// ReloadAllCheckInstances completely restarts a check with a new configuration and returns a list of killed check IDs
func (c *collectorImpl) ReloadAllCheckInstances(name string, newInstances []check.Check) ([]checkid.ID, error) {
	if !c.started() {
		return nil, fmt.Errorf("The collector is not running")
	}

	// Stop all the old instances
	killed := c.GetAllInstanceIDs(name)
	for _, id := range killed {
		e := c.StopCheck(id)
		if e != nil {
			return nil, fmt.Errorf("Error stopping check %s: %s", id, e)
		}
	}

	// Start the new instances
	for _, check := range newInstances {
		id, e := c.RunCheck(check)
		if e != nil {
			return nil, fmt.Errorf("Error adding check %s: %s", id, e)
		}
	}
	return killed, nil
}
