package runner

import (
	"github.com/stretchr/testify/mock"
	"testing"

	"go.uber.org/fx"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	checkMocks "github.com/DataDog/datadog-agent/pkg/process/checks/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func newMockCheck(t testing.TB, name string) *checkMocks.Check {
	// TODO: Change this to use check componenet once checks are migrated
	mockCheck := checkMocks.NewCheck(t)
	mockCheck.On("Init", mock.Anything, mock.Anything).Return(nil)
	mockCheck.On("Name").Return(name)
	mockCheck.On("SupportsRunOptions").Return(false)
	mockCheck.On("Realtime").Return(false)
	mockCheck.On("Cleanup")
	mockCheck.On("Run", mock.Anything, mock.Anything).Return(&checks.StandardRunResult{}, nil)
	mockCheck.On("ShouldSaveLastRun").Return(false)
	return mockCheck
}

func TestRunnerLifecycle(t *testing.T) {
	fxutil.Test(t, fx.Options(
		fx.Supply(
			&checks.HostInfo{},
			&sysconfig.Config{},
			[]checks.Check{
				newMockCheck(t, "process"),
			},
		),

		Module,
	), func(runner Component) {
		// Start and stop the component
	})
}
