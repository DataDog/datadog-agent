package system

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	log "github.com/cihub/seelog"
	"github.com/shirou/gopsutil/load"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// For testing purpose
var loadAvg = load.Avg

// LoadCheck doesn't need additional fields
type LoadCheck struct {
	sender aggregator.Sender
	nbCPU  int32
}

func (c *LoadCheck) String() string {
	return "LoadCheck"
}

// Run executes the check
func (c *LoadCheck) Run() error {
	avg, err := loadAvg()
	if err != nil {
		log.Errorf("system.LoadCheck: could not retrieve load stats: %s", err)
		return err
	}

	c.sender.Gauge("system.load.1", avg.Load1, "", nil)
	c.sender.Gauge("system.load.5", avg.Load5, "", nil)
	c.sender.Gauge("system.load.15", avg.Load15, "", nil)
	cpus := float64(c.nbCPU)
	c.sender.Gauge("system.load.norm.1", avg.Load1/cpus, "", nil)
	c.sender.Gauge("system.load.norm.5", avg.Load5/cpus, "", nil)
	c.sender.Gauge("system.load.norm.15", avg.Load15/cpus, "", nil)
	c.sender.Commit()

	return nil
}

// Configure the CPU check doesn't need configuration
func (c *LoadCheck) Configure(data check.ConfigData, initConfig check.ConfigData) error {
	// do nothing
	info, err := cpuInfo()
	if err != nil {
		return fmt.Errorf("system.LoadCheck: could not query CPU info")
	}
	for _, i := range info {
		c.nbCPU += i.Cores
	}
	return nil
}

// InitSender initializes a sender
func (c *LoadCheck) InitSender() {
	s, err := aggregator.GetSender(c.ID())
	if err != nil {
		log.Error(err)
		return
	}

	c.sender = s
}

// Interval returns the scheduling time for the check
func (c *LoadCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// ID returns the name of the check since there should be only one instance running
func (c *LoadCheck) ID() check.ID {
	return check.ID(c.String())
}

// Stop does nothing
func (c *LoadCheck) Stop() {}

func loadFactory() check.Check {
	return &LoadCheck{}
}

func init() {
	core.RegisterCheck("load", loadFactory)
}
