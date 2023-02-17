// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package process

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/process/runner"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	checkMocks "github.com/DataDog/datadog-agent/pkg/process/checks/mocks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func newMockCheck(t testing.TB, name string) *checkMocks.Check {
	// TODO: Change this to use check component once checks are migrated
	mockCheck := checkMocks.NewCheck(t)
	mockCheck.On("Init", mock.Anything, mock.Anything).Return(nil)
	mockCheck.On("Name").Return(name)
	mockCheck.On("SupportsRunOptions").Return(false)
	mockCheck.On("Realtime").Return(false)
	mockCheck.On("Cleanup")
	mockCheck.On("Run", mock.Anything, mock.Anything).Return(&checks.StandardRunResult{}, nil)
	mockCheck.On("ShouldSaveLastRun").Return(false)
	mockCheck.On("IsEnabled").Return(true)
	return mockCheck
}

func TestBundleDependencies(t *testing.T) {
	require.NoError(t, fx.ValidateApp(
		fx.Supply(
			fx.Annotate(t, fx.As(new(testing.TB))),

			[]checks.Check{checkMocks.NewCheck(t)},
			&checks.HostInfo{},
			&sysconfig.Config{},
		),

		// instantiate all of the process components, since this is not done
		// automatically.
		fx.Invoke(func(r runner.Component) {}),

		Bundle))
}

func TestBundleOneShot(t *testing.T) {
	runCmd := func(r runner.Component) {
		checks := r.GetChecks()
		require.Len(t, checks, 2)

		var names []string
		for _, c := range checks {
			require.True(t, c.IsEnabled())
			names = append(names, c.Name())
		}
		require.ElementsMatch(t, []string{"process", "container"}, names)
	}

	c1, c2 := newMockCheck(t, "process"), newMockCheck(t, "container")

	err := fxutil.OneShot(runCmd,
		fx.Supply(
			fx.Annotate(t, fx.As(new(testing.TB))),

			[]checks.Check{c1, c2},
			&checks.HostInfo{},
			&sysconfig.Config{},
		),

		Bundle,
	)
	require.NoError(t, err)
}
