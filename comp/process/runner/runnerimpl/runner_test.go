// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runnerimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/process/containercheck/containercheckimpl"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo/hostinfoimpl"
	"github.com/DataDog/datadog-agent/comp/process/processcheck/processcheckimpl"
	"github.com/DataDog/datadog-agent/comp/process/runner"
	"github.com/DataDog/datadog-agent/comp/process/submitter/submitterimpl"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRunnerLifecycle(t *testing.T) {
	_ = createDeps(t)
}

func TestRunnerRealtime(t *testing.T) {
	enableProcessAgent(t)

	t.Run("rt allowed", func(t *testing.T) {
		rtChan := make(chan types.RTResponse)

		deps := createDeps(t,
			fx.Provide(
				// Cast `chan types.RTResponse` to `<-chan types.RTResponse`.
				// We can't use `fx.As` because `<-chan types.RTResponse` is not an interface.
				func() <-chan types.RTResponse { return rtChan },
			),
			fx.Replace(config.MockParams{Overrides: map[string]interface{}{
				"process_config.disable_realtime_checks": false,
			}}),
		)

		rtChan <- types.RTResponse{
			&model.CollectorStatus{
				ActiveClients: 1,
				Interval:      10,
			},
		}
		assert.Eventually(t, func() bool {
			return deps.Runner.(*runnerImpl).IsRealtimeEnabled()
		}, 1*time.Second, 10*time.Millisecond)
	})

	t.Run("rt disallowed", func(t *testing.T) {
		// Buffer the channel because the runner will never consume from it, otherwise we will deadlock
		rtChan := make(chan types.RTResponse, 1)

		deps := createDeps(t,
			fx.Replace(config.MockParams{Overrides: map[string]interface{}{
				"process_config.disable_realtime_checks": true,
			}}),

			fx.Provide(
				// Cast `chan types.RTResponse` to `<-chan types.RTResponse`.
				// We can't use `fx.As` because `<-chan types.RTResponse` is not an interface.
				func() <-chan types.RTResponse { return rtChan },
			),
		)

		rtChan <- types.RTResponse{
			&model.CollectorStatus{
				ActiveClients: 1,
				Interval:      10,
			},
		}
		assert.Never(t, func() bool {
			return deps.Runner.(*runnerImpl).IsRealtimeEnabled()
		}, 1*time.Second, 10*time.Millisecond)
	})
}

func TestProvidedChecks(t *testing.T) {
	deps := createDeps(t)
	providedChecks := deps.Runner.GetProvidedChecks()

	var checkNames []string
	for _, check := range providedChecks {
		checkNames = append(checkNames, check.Object().Name())
	}
	t.Log("Provided Checks:", checkNames)

	assert.Len(t, providedChecks, 2)
}

type Deps struct {
	fx.In
	Runner runner.Component
}

func createDeps(t *testing.T, options ...fx.Option) Deps {
	return fxutil.Test[Deps](t, fx.Options(
		fx.Supply(
			core.BundleParams{},
		),
		Module(),
		submitterimpl.MockModule(),
		hostinfoimpl.MockModule(),

		// Checks
		processcheckimpl.MockModule(),
		containercheckimpl.MockModule(),

		core.MockBundle(),
		fx.Supply(workloadmeta.NewParams()),
		workloadmeta.Module(),
		fx.Options(options...),
	))
}

// enableProcessAgent overrides the process agent enabled function to always return true start/stop the runner
func enableProcessAgent(t *testing.T) {
	previousFn := agentEnabled
	agentEnabled = func(_ config.Component, _ []types.CheckComponent, _ log.Component) bool {
		return true
	}
	t.Cleanup(func() {
		agentEnabled = previousFn
	})
}
