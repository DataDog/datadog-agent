// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package collector

import (
	"fmt"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/internal/middleware"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	stopped uint32 = iota
	started
)

// EventType represents the type of events emitted by the collector
type EventType uint32

const (
	// CheckRun is emitted when a check is added to the collector
	CheckRun EventType = iota
	// CheckStop is emitted when a check is stopped and removed from the collector
	CheckStop
)

// EventReceiver represents a function to receive notification from the collector when running or stopping checks.
type EventReceiver func(checkid.ID, EventType)

// Collector manages a collection of checks and provides operations over them
type Collector interface {
	// Start begins the collector's operation.  The scheduler will not run any checks until this has been called.
	Start()
	// Stop halts any component involved in running a Check
	Stop()
	// RunCheck sends a Check in the execution queue
	RunCheck(inner check.Check) (checkid.ID, error)
	// StopCheck halts a check and remove the instance
	StopCheck(id checkid.ID) error
	// MapOverChecks call the callback with the list of checks locked.
	MapOverChecks(cb func([]check.Info))
	// GetChecks copies checks
	GetChecks() []check.Check
	// GetAllInstanceIDs returns the ID's of all instances of a check
	GetAllInstanceIDs(checkName string) []checkid.ID
	// ReloadAllCheckInstances completely restarts a check with a new configuration
	ReloadAllCheckInstances(name string, newInstances []check.Check) ([]checkid.ID, error)
	// AddEventReceiver adds a callback to the collector to be called each time a check is added or removed.
	AddEventReceiver(cb EventReceiver)
}

type collector struct {
	senderManager  sender.SenderManager
	checkInstances int64

	// state is 'started' or 'stopped'
	state *atomic.Uint32

	scheduler      *scheduler.Scheduler
	runner         *runner.Runner
	checks         map[checkid.ID]*middleware.CheckWrapper
	eventReceivers []EventReceiver

	cancelCheckTimeout time.Duration

	m sync.RWMutex
}

// NewCollector create a Collector instance and sets up the Python Environment
func NewCollector(senderManager sender.SenderManager, cancelCheckTimeout time.Duration, paths ...string) Collector {
	c := &collector{
		senderManager:      senderManager,
		checks:             make(map[checkid.ID]*middleware.CheckWrapper),
		state:              atomic.NewUint32(stopped),
		checkInstances:     int64(0),
		cancelCheckTimeout: cancelCheckTimeout,
	}
	pyVer, pyHome, pyPath := pySetup(paths...)

	// print the Python info if the interpreter was embedded
	if pyVer != "" {
		log.Infof("Embedding Python %s", pyVer)
		log.Debugf("Python Home: %s", pyHome)
		log.Debugf("Python path: %s", pyPath)
	}

	// Prepare python environment if necessary
	if err := pyPrepareEnv(); err != nil {
		log.Errorf("Unable to perform additional configuration of the python environment: %v", err)
	}

	log.Debug("Collector up and running!")
	return c
}

// AddEventReceiver adds a callback to the collector to be called each time a check is added or removed.
func (c *collector) AddEventReceiver(cb EventReceiver) {
	c.m.Lock()
	defer c.m.Unlock()

	c.eventReceivers = append(c.eventReceivers, cb)
}

func (c *collector) notify(cid checkid.ID, e EventType) {
	for _, cb := range c.eventReceivers {
		cb(cid, e)
	}
}

// Start begins the collector's operation.  The scheduler will not run any checks until this has been called.
func (c *collector) Start() {
	c.m.Lock()
	defer c.m.Unlock()

	if c.state.Load() == started {
		return
	}

	run := runner.NewRunner(c.senderManager)
	sched := scheduler.NewScheduler(run.GetChan())

	// let the runner some visibility into the scheduler
	run.SetScheduler(sched)
	sched.Run()

	c.scheduler = sched
	c.runner = run
	c.state.Store(started)
}

// Stop halts any component involved in running a Check
func (c *collector) Stop() {
	c.m.Lock()
	defer c.m.Unlock()

	if c.state.Load() == stopped {
		return
	}

	if c.scheduler != nil {
		c.scheduler.Stop() //nolint:errcheck
		c.scheduler = nil
	}
	if c.runner != nil {
		c.runner.Stop()
		c.runner = nil
	}
	c.state.Store(stopped)
}

// RunCheck sends a Check in the execution queue
func (c *collector) RunCheck(inner check.Check) (checkid.ID, error) {
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

	err := c.scheduler.Enter(ch)
	if err != nil {
		return emptyID, fmt.Errorf("unable to schedule the check: %s", err)
	}

	// Track the total number of checks running in order to have an appropriate number of workers
	c.checkInstances++
	if ch.Interval() == 0 {
		// Adding a temporary runner for long running check in case the
		// number of runners is lower than the number of long running
		// checks.
		log.Infof("Adding an extra runner for the '%s' long running check", ch)
		c.runner.AddWorker()
	} else {
		c.runner.UpdateNumWorkers(c.checkInstances)
	}

	c.checks[ch.ID()] = ch
	c.notify(ch.ID(), CheckRun)
	return ch.ID(), nil
}

// StopCheck halts a check and remove the instance
func (c *collector) StopCheck(id checkid.ID) error {
	if !c.started() {
		return fmt.Errorf("the collector is not running")
	}

	ch, found := c.get(id)
	if !found {
		return fmt.Errorf("cannot find a check with ID %s", id)
	}

	// unschedule the instance
	err := c.scheduler.Cancel(id)
	if err != nil {
		return fmt.Errorf("an error occurred while canceling the check schedule: %s", err)
	}

	err = c.runner.StopCheck(id)
	if err != nil {
		// still attempt to cancel the check before returning the error
		_ = c.cancelCheck(ch, c.cancelCheckTimeout)
		return fmt.Errorf("an error occurred while stopping the check: %s", err)
	}

	err = c.cancelCheck(ch, c.cancelCheckTimeout)
	if err != nil {
		return fmt.Errorf("an error occurred while calling check.Cancel(): %s", err)
	}

	// remove the check from the stats map
	expvars.RemoveCheckStats(id)

	// vaporize the check
	c.delete(id)

	return nil
}

// cancelCheck calls Cancel on the passed check, with a timeout
func (c *collector) cancelCheck(ch check.Check, timeout time.Duration) error {
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

func (c *collector) get(id checkid.ID) (check.Check, bool) {
	c.m.RLock()
	defer c.m.RUnlock()

	ch, found := c.checks[id]
	return ch, found
}

// remove the check from the list
func (c *collector) delete(id checkid.ID) {
	c.m.Lock()
	defer c.m.Unlock()

	delete(c.checks, id)
	c.notify(id, CheckStop)
}

// lightweight shortcut to see if the collector has started
func (c *collector) started() bool {
	return c.state.Load() == started
}

// MapOverChecks call the callback with the list of checks locked.
func (c *collector) MapOverChecks(cb func([]check.Info)) {
	c.m.RLock()
	defer c.m.RUnlock()

	cInfo := []check.Info{}
	for _, c := range c.checks {
		cInfo = append(cInfo, c)
	}
	cb(cInfo)
}

// GetChecks copies checks
func (c *collector) GetChecks() []check.Check {
	c.m.RLock()
	defer c.m.RUnlock()

	chks := make([]check.Check, 0, len(c.checks))
	for _, chck := range c.checks {
		chks = append(chks, chck)
	}

	return chks
}

// GetAllInstanceIDs returns the ID's of all instances of a check
func (c *collector) GetAllInstanceIDs(checkName string) []checkid.ID {
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

// ReloadAllCheckInstances completely restarts a check with a new configuration
func (c *collector) ReloadAllCheckInstances(name string, newInstances []check.Check) ([]checkid.ID, error) {
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
