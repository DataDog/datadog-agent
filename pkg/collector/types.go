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

// AbstractCollector abstract common structure for the Collector
type AbstractCollector struct {
	scheduler *scheduler.Scheduler
	runner    *runner.Runner
	checks    map[check.ID]check.Check
	state     uint32
	m         sync.RWMutex
}
