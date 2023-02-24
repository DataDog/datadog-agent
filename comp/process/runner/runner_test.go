// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/fx"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/process/containercheck"
	"github.com/DataDog/datadog-agent/comp/process/processcheck"
	"github.com/DataDog/datadog-agent/comp/process/submitter"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/config"
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
	return mockCheck
}

func TestRunnerLifecycle(t *testing.T) {
	fxutil.Test(t, fx.Options(
		fx.Supply(
			&checks.HostInfo{},
			&sysconfig.Config{},
		),

		Module,
		submitter.MockModule,
		processcheck.Module,
	), func(runner Component) {
		// Start and stop the component
	})
}

func TestRunnerRealtime(t *testing.T) {
	t.Run("rt allowed", func(t *testing.T) {
		rtChan := make(chan types.RTResponse)

		mockConfig := config.Mock(t)
		mockConfig.Set("process_config.disable_realtime_checks", false)

		fxutil.Test(t, fx.Options(
			fx.Supply(
				&checks.HostInfo{},
				&sysconfig.Config{},
			),

			fx.Provide(
				// Cast `chan types.RTResponse` to `<-chan types.RTResponse`.
				// We can't use `fx.As` because `<-chan types.RTResponse` is not an interface.
				func() <-chan types.RTResponse { return rtChan },
			),

			Module,
			submitter.MockModule,
			processcheck.Module,
		), func(r Component) {
			rtChan <- types.RTResponse{
				{
					ActiveClients: 1,
					Interval:      10,
				},
			}
			assert.Eventually(t, func() bool {
				return r.(*runner).IsRealtimeEnabled()
			}, 1*time.Second, 10*time.Millisecond)
		})
	})

	t.Run("rt disallowed", func(t *testing.T) {
		// Buffer the channel because the runner will never consume from it, otherwise we will deadlock
		rtChan := make(chan types.RTResponse, 1)

		mockConfig := config.Mock(t)
		mockConfig.Set("process_config.disable_realtime_checks", true)

		fxutil.Test(t, fx.Options(
			fx.Supply(
				&checks.HostInfo{},
				&sysconfig.Config{},
			),

			fx.Provide(
				// Cast `chan types.RTResponse` to `<-chan types.RTResponse`.
				// We can't use `fx.As` because `<-chan types.RTResponse` is not an interface.
				func() <-chan types.RTResponse { return rtChan },
			),

			Module,
			submitter.MockModule,
			processcheck.Module,
		), func(r Component) {
			rtChan <- types.RTResponse{
				{
					ActiveClients: 1,
					Interval:      10,
				},
			}
			assert.Never(t, func() bool {
				return r.(*runner).IsRealtimeEnabled()
			}, 1*time.Second, 10*time.Millisecond)
		})
	})
}

func TestProvidedChecks(t *testing.T) {
	fxutil.Test(t, fx.Options(
		fx.Supply(
			&checks.HostInfo{},
			&sysconfig.Config{},
		),

		Module,
		submitter.MockModule,

		// Checks
		processcheck.Module,
		containercheck.Module,
	), func(r Component) {
		providedChecks := r.GetProvidedChecks()

		var checkNames []string
		for _, check := range providedChecks {
			checkNames = append(checkNames, check.Object().Name())
		}
		t.Log("Provided Checks:", checkNames)

		assert.Len(t, providedChecks, 2)
	})
}
