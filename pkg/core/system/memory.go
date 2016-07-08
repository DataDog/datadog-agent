package system

import (
	"github.com/DataDog/datadog-agent/pkg/check"
	"github.com/DataDog/datadog-agent/pkg/core"
	"github.com/op/go-logging"
	"github.com/shirou/gopsutil/mem"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
)

var log = logging.MustGetLogger("datadog-agent")

const MEMORY_CHECK_INTERVAL = 5

type MemoryCheck struct{}

func (c *MemoryCheck) String() string {
	return "MemoryCheck"
}

func (c *MemoryCheck) Run() error {
	v, _ := mem.VirtualMemory()
	aggregator.GetSender(MEMORY_CHECK_INTERVAL).Gauge("system.mem.total", float64(v.Total), "", []string{})
	return nil
}

// Configure the Python check from YAML data
func (c *MemoryCheck) Configure(data check.ConfigData) {
	// do nothing
}

func init() {
	core.RegisterCheck("memory", &MemoryCheck{})
}
