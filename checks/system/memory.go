package system

import (
	// stdlib
	// project
	"github.com/DataDog/datadog-agent/aggregator"

	// 3rd party
	"github.com/op/go-logging"
	"github.com/shirou/gopsutil/mem"
)

var log = logging.MustGetLogger("datadog-agent")

type MemoryCheck struct {
	Name string
}

func (c *MemoryCheck) Check(agg *aggregator.DefaultAggregator) {
	log.Info("Running memory check")
	v, _ := mem.VirtualMemory()

	agg.Gauge("system.mem.total", float64(v.Total), "monhost", &[]string{})

}
