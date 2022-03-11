package app

import (
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// VersionCmd is a command that prints the process-agent version data
var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version info",
	RunE: func(cmd *cobra.Command, args []string) error {
		return WriteVersion(os.Stdout)
	},
	SilenceUsage: true,
}

// versionString returns the version information filled in at build time
func versionString(v version.Version) string {
	return fmt.Sprintf(
		"Agent %s - Commit: %s - Serialization version: %s - Go version: %s",
		v.GetNumberAndPre(),
		v.Commit,
		serializer.AgentPayloadVersion,
		runtime.Version(),
	)
}

// WriteVersion writes the version string to out
func WriteVersion(w io.Writer) error {
	v, err := version.Agent()
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(w, versionString(v))
	return err
}
