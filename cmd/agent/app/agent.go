package app

import (
	"github.com/spf13/cobra"

	// register core checks
	_ "github.com/DataDog/datadog-agent/pkg/collector/check/core/system"
)

// AgentCmd is the root command
var AgentCmd = &cobra.Command{
	Use:   "agent [command]",
	Short: "Datadog Agent at your service.",
	Long: `
The Datadog Agent faithfully collects events and metrics and brings them 
to Datadog on your behalf so that you can do something useful with your 
monitoring and performance data.`,
}
