package local

import (
	"github.com/stretchr/testify/require"
	"testing"

	complog "github.com/DataDog/datadog-agent/comp/core/log"

	"github.com/DataDog/datadog-agent/pkg/agentless/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStartAgentlessScannerLocal(t *testing.T) {
	grp := GroupCommand()
	fxutil.TestOneShotSubcommand(t,
		grp.Commands(),
		[]string{"scan", "/"},
		localScanCmd,
		func(params *localScanParams, log complog.Component, sc *types.ScannerConfig) {
			require.NotNil(t, log)
			require.NotNil(t, sc)

			require.NotNil(t, params)
			require.Equal(t, "/", params.resourceID)
			require.Equal(t, "unknown", params.targetName)
		})
}
