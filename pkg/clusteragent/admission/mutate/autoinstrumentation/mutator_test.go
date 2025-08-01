package autoinstrumentation

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestNewMutatorWithFilter(t *testing.T) {
	mockWmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Supply(coreconfig.Params{}),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		coreconfig.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	configDisabled := &Config{
		Instrumentation: &InstrumentationConfig{
			Enabled: false,
		},
	}

	// configEnabled := &Config{
	// 	Instrumentation: &InstrumentationConfig{
	// 		Enabled: true,
	// 	},
	// }
	t.Run("return namespace mutator when instrumentation disabled", func(t *testing.T) {
		mutator, err := NewMutatorWithFilter(configDisabled, mockWmeta)
		require.NoError(t, err)
		require.IsType(t, &NamespaceMutator{}, mutator)
	})
}
