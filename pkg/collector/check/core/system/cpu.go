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
var cpuInfo = cpu.Info

// CPUCheck doesn't need additional fields
type CPUCheck struct {
	sender aggregator.Sender
	nbCPU  float64
}

func (c *CPUCheck) String() string {
	return "CPUCheck"
}

// Run executes the check
func (c *CPUCheck) Run() error {
	t, err := times(false)
	if err != nil {
		log.Errorf("system.CPUCheck: could not retrieve cpu stats: %s", err)
		return err
	} else if len(t) < 1 {
		errEmpty := fmt.Errorf("no cpu stats retrieve (empty results)")
		log.Errorf("system.CPUCheck: %s", errEmpty)
		return errEmpty
	}

	// gopsutil return the sum of every CPU
	c.sender.Rate("system.cpu.user", t[0].User/c.nbCPU*100.0, "", nil)
	c.sender.Rate("system.cpu.system", t[0].System/c.nbCPU*100.0, "", nil)
	c.sender.Rate("system.cpu.iowait", t[0].Iowait/c.nbCPU*100.0, "", nil)
	c.sender.Rate("system.cpu.idle", t[0].Idle/c.nbCPU*100.0, "", nil)
	c.sender.Rate("system.cpu.stolen", t[0].Stolen/c.nbCPU*100.0, "", nil)
	c.sender.Rate("system.cpu.guest", t[0].Guest/c.nbCPU*100.0, "", nil)

	c.sender.Commit()
	return nil
}

// Configure the CPU check doesn't need configuration
func (c *CPUCheck) Configure(data check.ConfigData) error {
	// do nothing
	info, err := cpuInfo()
	if err != nil {
		return fmt.Errorf("system.CPUCheck: could not query CPU info")
	}
	for _, i := range info {
		c.nbCPU += float64(i.Cores)
	}
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
