package system

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/core"
	"github.com/op/go-logging"
	"github.com/shirou/gopsutil/mem"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

var log = logging.MustGetLogger("datadog-agent")

const checkInterval = 5

// MemoryCheck doesn't need additional fields
type MemoryCheck struct{}

func (c *MemoryCheck) String() string {
	return "MemoryCheck"
}

// Run executes the check
func (c *MemoryCheck) Run() error {
	v, _ := mem.VirtualMemory()
	aggregator.GetSender(checkInterval).Gauge("system.mem.total", float64(v.Total), "", []string{})
	return nil
}

// Configure the Python check from YAML data
func (c *MemoryCheck) Configure(data check.ConfigData) {
	// do nothing
}

// Interval returns the scheduling time for the check
func (c *MemoryCheck) Interval() time.Duration {
	return check.DefaultCheckInterval
}

func init() {
	core.RegisterCheck("memory", &MemoryCheck{})
}
