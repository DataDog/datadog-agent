package run

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/cmd/trace-agent/subcommands"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestFxRun(t *testing.T) {
	fxutil.TestRun(t, func() error {
		ctx := context.Background()
		cliParams := RunParams{GlobalParams: &subcommands.GlobalParams{}}
		defaultConfPath := ""
		return runFx(ctx, &cliParams, defaultConfPath)
	})
}
