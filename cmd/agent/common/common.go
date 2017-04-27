// Package common provides a set of common symbols needed by different packages,
// to avoid circular dependencies.
package common

import (
	"github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/kardianos/osext"
)

var (
	// Collector is the global object orchestrating check runs
	Collector *collector.Collector
	// Stopper is the channel used by other packages to ask for stopping the agent
	Stopper = make(chan bool)

	// utility variables
	_here, _ = osext.ExecutableFolder()
)
