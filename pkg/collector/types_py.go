// +build cpython

package collector

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/py"

	python "github.com/sbinet/go-python"
)

// Collector struct that provides a collector with python support
type Collector struct {
	AbstractCollector
	pyState *python.PyThreadState
}

// CreateCollector creates a collector for the current implementation
func CreateCollector(s *scheduler.Scheduler, r *runner.Runner, paths ...string) {
	c := &Collector{
		AbstractCollector{
			scheduler: s,
			runner:    r,
			checks:    make(map[check.ID]check.Check),
			state:     started,
		},
		pyState: py.Initialize(paths...),
	}
}

// StopImplementation implementation specific stop routine
func (c *Collector) StopImplementation() {
	python.PyEval_RestoreThread(c.pyState)
	c.pyState = nil

	return
}
