package system

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/core"
	log "github.com/cihub/seelog"
	"github.com/shirou/gopsutil/cpu"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// For testing purpose
var times = cpu.Times

// CPUCheck doesn't need additional fields
type CPUCheck struct {
	sender aggregator.Sender
}

func (c *CPUCheck) String() string {
	return "CPUCheck"
}

// Run executes the check
func (c *CPUCheck) Run() error {
	t, err := times(false)
	if err != nil {
		log.Error("system.CPUCheck: could not retrieve cpu stats: %s", err)
		return err
	} else if len(t) < 1 {
		errEmpty := fmt.Errorf("no cpu stats retrieve (empty results)")
		log.Error("system.CPUCheck: %s", errEmpty)
		return errEmpty
	}

	c.sender.Gauge("system.cpu.user", t[0].User, "", nil)
	c.sender.Gauge("system.cpu.system", t[0].System, "", nil)
	c.sender.Gauge("system.cpu.iowait", t[0].Iowait, "", nil)
	c.sender.Gauge("system.cpu.idle", t[0].Idle, "", nil)
	c.sender.Gauge("system.cpu.stolen", t[0].Stolen, "", nil)
	c.sender.Gauge("system.cpu.guest", t[0].Guest, "", nil)

	c.sender.Commit()
	return nil
}

// Configure the CPU check doesn't need configuration
func (c *CPUCheck) Configure(data check.ConfigData) error {
	// do nothing
	return nil
}

// InitSender initializes a sender
func (c *CPUCheck) InitSender() {
	s, err := aggregator.GetSender()
	if err != nil {
		log.Error(err)
		return
	}

	c.sender = s
}

// Interval returns the scheduling time for the check
func (c *CPUCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// ID FIXME: this should return a real identifier
func (c *CPUCheck) ID() string {
	return c.String()
}

// Stop does nothing
func (c *CPUCheck) Stop() {}

func init() {
	core.RegisterCheck("cpu", &CPUCheck{})
}
