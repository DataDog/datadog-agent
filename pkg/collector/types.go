package collector

import (
	"sync"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
)

const (
	stopped uint32 = iota
	started
)

// Collector abstract common operations about running a Check
type CollectorBase struct {
	scheduler *scheduler.Scheduler
	runner    *runner.Runner
	checks    map[check.ID]check.Check
	state     uint32
	m         sync.RWMutex
}
