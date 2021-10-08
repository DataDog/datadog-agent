package app

import (
	"fmt"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func init() {
	SysprobeCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version info",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		if flagNoColor {
			color.NoColor = true
		}
		_, _ = fmt.Fprintln(
			color.Output,
			versionString(),
		)
	},
}

// versionString returns the version information filled in at build time
func versionString() string {
	av, _ := version.Agent()
	meta := ""
	if av.Meta != "" {
		meta = fmt.Sprintf("- Meta: %s ", color.YellowString(av.Meta))
	}
	return fmt.Sprintf("Agent %s %s- Commit: %s - Serialization version: %s - Go version: %s",
		color.CyanString(av.GetNumberAndPre()),
		meta,
		color.GreenString(av.Commit),
		color.YellowString(serializer.AgentPayloadVersion),
		color.RedString(runtime.Version()),
	)
}
