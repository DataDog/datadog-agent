// Package common provides a set of common symbols needed by different packages,
// to avoid circular dependencies.
package common

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/kardianos/osext"
)

var (
	// Stopper is the channel used by other packages to ask for stopping the agent
	Stopper = make(chan bool)

	// AgentRunner is the current global checks Runner
	AgentRunner *check.Runner

	// AgentScheduler is the current global Scheduler
	AgentScheduler *scheduler.Scheduler

	// utility variables
	_here, _ = osext.ExecutableFolder()
	// DistPath holds the path to the folder containing distribution files
	DistPath = filepath.Join(_here, "dist")
)
