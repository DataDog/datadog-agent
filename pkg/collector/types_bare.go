// +build !cpython

package collector

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
)

type Collector struct {
	CollectorBase
}

func CreateCollector(s *scheduler.Scheduler, r *runner.Runner, paths ...string) *Collector {
	c := &Collector{
		CollectorBase{
			scheduler: s,
			runner:    r,
			checks:    make(map[check.ID]check.Check),
			state:     started,
		},
	}

	return c
}

func (c *Collector) StopImplementation() {
	return //NOP
}
