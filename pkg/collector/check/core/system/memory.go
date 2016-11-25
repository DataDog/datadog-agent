package system

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/core"
	log "github.com/cihub/seelog"
	"github.com/shirou/gopsutil/mem"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

// MemoryCheck doesn't need additional fields
type MemoryCheck struct {
	sender aggregator.Sender
}

func (c *MemoryCheck) String() string {
	return "MemoryCheck"
}

// Run executes the check
func (c *MemoryCheck) Run() error {
	v, _ := mem.VirtualMemory()
	c.sender.Gauge("system.mem.total", float64(v.Total), "", []string{})
	c.sender.Commit()
	return nil
}

// Configure the Python check from YAML data
func (c *MemoryCheck) Configure(data check.ConfigData) {
	// do nothing
}

// InitSender initializes a sender
func (c *MemoryCheck) InitSender() {
	s, err := aggregator.GetSender()
	if err != nil {
		log.Error(err)
		return
	}

	c.sender = s
}

// Interval returns the scheduling time for the check
func (c *MemoryCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

// ID FIXME: this should return a real identifier
func (c *MemoryCheck) ID() string {
	return c.String()
}

// Stop does nothing
func (c *MemoryCheck) Stop() {}

func init() {
	core.RegisterCheck("memory", &MemoryCheck{})
}
