package expvars

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestExpvarServer(t *testing.T) {
	fxutil.Test(t, fx.Options(
		fx.Supply(core.BundleParams{}),

		Module,
		hostinfo.MockModule,
		core.MockBundle,
	), func(Component) {
		res, err := http.Get("http://localhost:6062/debug/vars")
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, res.StatusCode)
	})
}

func TestTelemetry(t *testing.T) {
	fxutil.Test(t, fx.Options(
		fx.Supply(core.BundleParams{}),

		Module,
		hostinfo.MockModule,
		core.MockBundle,
	), func(Component) {
		res, err := http.Get("http://localhost:6062/telemetry")
		require.NoError(t, err)

		assert.Equal(t, http.StatusOK, res.StatusCode)
	})
}
