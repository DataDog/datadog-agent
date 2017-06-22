// +build cpython

package collector

import (
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/py"

	python "github.com/sbinet/go-python"
)

type Collector struct {
	CollectorBase
	pyState *python.PyThreadState
}

func CreateCollector(s *scheduler.Scheduler, r *runner.Runner, paths ...string) {
	c := &Collector{
		CollectorBase{
			scheduler: s,
			runner:    r,
			checks:    make(map[check.ID]check.Check),
			state:     started,
		},
		pyState: py.Initialize(paths...),
	}
}

func (c *Collector) StopImplementation() {
	python.PyEval_RestoreThread(c.pyState)
	c.pyState = nil

	return
}
