// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package collector

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	stopped uint32 = iota
	started
)

const cancelCheckTimeout time.Duration = 500 * time.Millisecond

// Collector abstract common operations about running a Check
type Collector struct {
	checkInstances int64
	state          uint32

	scheduler *scheduler.Scheduler
	runner    *runner.Runner
	checks    map[check.ID]check.Check

	m sync.RWMutex
}

// NewCollector create a Collector instance and sets up the Python Environment
func NewCollector(paths ...string) *Collector {
	run := runner.NewRunner()
	sched := scheduler.NewScheduler(run.GetChan())

	// let the runner some visibility into the scheduler
	run.SetScheduler(sched)
	sched.Run()

	c := &Collector{
		scheduler:      sched,
		runner:         run,
		checks:         make(map[check.ID]check.Check),
		state:          started,
		checkInstances: int64(0),
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

// Stop halts any component involved in running a Check and shuts down
// the Python Environment
func (c *Collector) Stop() {
	c.m.Lock()
	defer c.m.Unlock()

	if c.state == stopped {
		return
	}

	c.scheduler.Stop() //nolint:errcheck
	c.scheduler = nil
	c.runner.Stop()
	c.runner = nil
	pyTeardown()
	c.state = stopped
}

// RunCheck sends a Check in the execution queue
func (c *Collector) RunCheck(ch check.Check) (check.ID, error) {
	c.m.Lock()
	defer c.m.Unlock()

	var emptyID check.ID

	if c.state != started {
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
	return ch.ID(), nil
}

// StopCheck halts a check and remove the instance
func (c *Collector) StopCheck(id check.ID) error {
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
		_ = c.cancelCheck(ch, cancelCheckTimeout)
		return fmt.Errorf("an error occurred while stopping the check: %s", err)
	}

	err = c.cancelCheck(ch, cancelCheckTimeout)
	if err != nil {
		return fmt.Errorf("an error occurred while calling check.Cancel(): %s", err)
	}

	// remove the check from the stats map
	runner.RemoveCheckStats(id)

	// vaporize the check
	c.delete(id)

	return nil
}

// cancelCheck calls Cancel on the passed check, with a timeout
func (c *Collector) cancelCheck(ch check.Check, timeout time.Duration) error {
	done := make(chan struct{})

	go func() {
		ch.Cancel()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout while calling check.Cancel() on check ID %s", ch.ID())
	}
}

func (c *Collector) get(id check.ID) (check.Check, bool) {
	c.m.RLock()
	defer c.m.RUnlock()

	ch, found := c.checks[id]
	return ch, found
}

// remove the check from the list
func (c *Collector) delete(id check.ID) {
	c.m.Lock()
	defer c.m.Unlock()

	delete(c.checks, id)
}

// lightweight shortcut to see if the collector has started
func (c *Collector) started() bool {
	return atomic.LoadUint32(&(c.state)) == started
}

// GetAllInstanceIDs returns the ID's of all instances of a check
func (c *Collector) GetAllInstanceIDs(checkName string) []check.ID {
	c.m.RLock()
	defer c.m.RUnlock()

	instances := []check.ID{}
	for id, check := range c.checks {
		if check.String() == checkName {
			instances = append(instances, id)
		}
	}

	return instances
}

// ReloadAllCheckInstances completely restarts a check with a new configuration
func (c *Collector) ReloadAllCheckInstances(name string, newInstances []check.Check) ([]check.ID, error) {
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
