/*
Package app implements the Agent main loop, orchestrating
all the components, providing a command line interface and
a public HTTP interface implementing several functionalities.
*/
package app

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/kardianos/osext"
	"github.com/spf13/cobra"
)

// notable variables to be used across the package
var (
	// AgentCmd is the root command
	AgentCmd = &cobra.Command{
		Use:   "agent [command]",
		Short: "Datadog Agent at your service.",
		Long: `
The Datadog Agent faithfully collects events and metrics and brings them 
to Datadog on your behalf so that you can do something useful with your 
monitoring and performance data.`,
	}

	// utility variables
	_here, _  = osext.ExecutableFolder()
	_distPath = filepath.Join(_here, "dist")

	// The forwarder
	_forwarder *forwarder.Forwarder

	// The checks Runner
	_runner *check.Runner

	// The Scheduler
	_scheduler *scheduler.Scheduler
)
