package app

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/kardianos/osext"
	"github.com/spf13/cobra"
)

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

	// The checks Runner
	_runner *check.Runner

	// The Scheduler
	_scheduler *scheduler.Scheduler
)
