// +build !cpython

package collector

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
)

// Collector struct that provides a collector without python support
type Collector struct {
	AbstractCollector
}

// CreateCollector creates a collector for the current implementation
func CreateCollector(s *scheduler.Scheduler, r *runner.Runner, paths ...string) *Collector {
	c := &Collector{
		AbstractCollector{
			scheduler: s,
			runner:    r,
			checks:    make(map[check.ID]check.Check),
			state:     started,
		},
	}

	return c
}

// StopImplementation implementation specific stop routine
func (c *Collector) StopImplementation() {
	return //NOP
}
