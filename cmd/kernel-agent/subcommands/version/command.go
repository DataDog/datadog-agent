package version

import (
	"fmt"
	"os"
	"runtime"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
)

type params struct {
	binary string
}

// MakeCommand returns a `version` command to be used by agent binaries.
func MakeCommand(binary string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version info",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(run, fx.Supply(&params{binary}))
		},
	}

	return cmd
}

func run(params *params) error {
	av, _ := version.Agent()
	meta := ""

	if av.Meta != "" {
		meta = fmt.Sprintf("- Meta: %s ", color.YellowString(av.Meta))
	}

	fmt.Fprintf(os.Stdout,
		"%s %s %s- Commit: %s - Go version: %s\n",
		params.binary,
		av.GetNumberAndPre(),
		meta,
		version.Commit,
		runtime.Version(),
	)

	return nil
}
