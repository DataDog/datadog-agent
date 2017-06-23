package collector

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/config"
	log "github.com/cihub/seelog"
)

const (
	stopped uint32 = iota
	started
)

// Collector abstract common operations about running a Check
type Collector struct {
	scheduler *scheduler.Scheduler
	runner    *runner.Runner
	checks    map[check.ID]check.Check
	state     uint32
	m         sync.RWMutex
}

// NewCollector create a Collector instance and sets up the Python Environment
func NewCollector(paths ...string) *Collector {
	log.Debug("NewCollector")
	run := runner.NewRunner(config.Datadog.GetInt("check_runners"))
	sched := scheduler.NewScheduler(run.GetChan())
	log.Debug("scheduler created")
	sched.Run()

	// Send the agent startup event
	// TODO
	log.Debug("Creating collector")
	c := &Collector{
		scheduler: sched,
		runner:    run,
		checks:    make(map[check.ID]check.Check),
		state:     started,
	}
	pySetup(paths...)
	log.Debug("created collector")
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

	c.scheduler.Stop()
	c.scheduler = nil
	c.runner.Stop()
	c.runner = nil
	pyTeardown()
	c.state = stopped
}

// RunCheck sends a Check in the execution queue
func (c *Collector) RunCheck(check check.Check) error {
	c.m.Lock()
	defer c.m.Unlock()

	if c.state != started {
		return fmt.Errorf("the collector is not running")
	}

	if _, found := c.checks[check.ID()]; found {
		return fmt.Errorf("a check with ID %s is already running", check.ID())
	}

	err := c.scheduler.Enter(check)
	if err != nil {
		return fmt.Errorf("unable to schedule the check: %s", err)
	}

	c.checks[check.ID()] = check
	return nil
}

// ReloadCheck stops and restart a check with a new configuration
func (c *Collector) ReloadCheck(id check.ID, config, initConfig check.ConfigData) error {
	if !c.started() {
		return fmt.Errorf("the collector is not running")
	}

	// do we know this check instance?
	// BUG(massi): we could create the Check if it doesn't exist, see https://github.com/DataDog/datadog-agent/pull/148
	// for reference
	if !c.find(id) {
		return fmt.Errorf("cannot find a check with ID %s", id)
	}

	c.m.Lock()
	defer c.m.Unlock()

	// unschedule the instance
	err := c.scheduler.Cancel(id)
	if err != nil {
		return fmt.Errorf("an error occurred while cancelling the check schedule: %s", err)
	}

	// stop the instance
	err = c.runner.StopCheck(id)
	if err != nil {
		return fmt.Errorf("an error occurred while stopping the check: %s", err)
	}

	// re-configure
	check := c.checks[id]
	err = check.Configure(config, initConfig)
	if err != nil {
		return fmt.Errorf("error configuring the check with ID %s", id)
	}

	// re-schedule
	c.scheduler.Enter(check)

	return nil
}

// StopCheck halts a check and remove the instance
func (c *Collector) StopCheck(id check.ID) error {
	if !c.started() {
		return fmt.Errorf("the collector is not running")
	}

	if !c.find(id) {
		return fmt.Errorf("cannot find a check with ID %s", id)
	}

	// unschedule the instance
	err := c.scheduler.Cancel(id)
	if err != nil {
		return fmt.Errorf("an error occurred while cancelling the check schedule: %s", err)
	}

	// stop the instance
	err = c.runner.StopCheck(id)
	if err != nil {
		return fmt.Errorf("an error occurred while stopping the check: %s", err)
	}

	// vaporize the check
	c.delete(id)

	return nil
}

// check if the check is on the list
func (c *Collector) find(id check.ID) bool {
	c.m.RLock()
	defer c.m.RUnlock()

	_, found := c.checks[id]
	return found
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
