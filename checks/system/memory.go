package system

import (
	"github.com/DataDog/datadog-agent/aggregator"
	"github.com/op/go-logging"
	"github.com/shirou/gopsutil/mem"
)

var log = logging.MustGetLogger("datadog-agent")

type MemoryCheck struct{}

func (c *MemoryCheck) String() string {
	return "MemoryCheck"
}

func (c *MemoryCheck) Run() error {
	v, _ := mem.VirtualMemory()
	aggregator.Get().Gauge("system.mem.total", float64(v.Total), "", []string{})
	return nil
}
