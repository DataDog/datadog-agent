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
	"github.com/DataDog/datadog-agent/comp/process/containercheck/containercheckimpl"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo/hostinfoimpl"
	"github.com/DataDog/datadog-agent/comp/process/processcheck/processcheckimpl"
	"github.com/DataDog/datadog-agent/comp/process/runner"
	"github.com/DataDog/datadog-agent/comp/process/submitter/submitterimpl"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRunnerLifecycle(t *testing.T) {
	_ = fxutil.Test[runner.Component](t, fx.Options(
		fx.Supply(core.BundleParams{}),

		Module(),
		submitterimpl.MockModule(),
		processcheckimpl.Module(),
		hostinfoimpl.MockModule(),
		core.MockBundle(),
	))
}

func TestRunnerRealtime(t *testing.T) {
	enableProcessAgent(t)

	t.Run("rt allowed", func(t *testing.T) {
		rtChan := make(chan types.RTResponse)

		r := fxutil.Test[runner.Component](t, fx.Options(
			fx.Provide(
				// Cast `chan types.RTResponse` to `<-chan types.RTResponse`.
				// We can't use `fx.As` because `<-chan types.RTResponse` is not an interface.
				func() <-chan types.RTResponse { return rtChan },
			),

			fx.Supply(core.BundleParams{}),
			fx.Replace(config.MockParams{Overrides: map[string]interface{}{
				"process_config.disable_realtime_checks": false,
			}}),

			Module(),
			submitterimpl.MockModule(),
			processcheckimpl.Module(),
			hostinfoimpl.MockModule(),
			core.MockBundle(),
		))

		rtChan <- types.RTResponse{
			&model.CollectorStatus{
				ActiveClients: 1,
				Interval:      10,
			},
		}
		assert.Eventually(t, func() bool {
			return r.(*runnerImpl).IsRealtimeEnabled()
		}, 1*time.Second, 10*time.Millisecond)
	})

	t.Run("rt disallowed", func(t *testing.T) {
		// Buffer the channel because the runner will never consume from it, otherwise we will deadlock
		rtChan := make(chan types.RTResponse, 1)

		r := fxutil.Test[runner.Component](t, fx.Options(
			fx.Supply(core.BundleParams{}),
			fx.Replace(config.MockParams{Overrides: map[string]interface{}{
				"process_config.disable_realtime_checks": true,
			}}),

			fx.Provide(
				// Cast `chan types.RTResponse` to `<-chan types.RTResponse`.
				// We can't use `fx.As` because `<-chan types.RTResponse` is not an interface.
				func() <-chan types.RTResponse { return rtChan },
			),

			Module(),
			submitterimpl.MockModule(),
			processcheckimpl.Module(),
			hostinfoimpl.MockModule(),
			core.MockBundle(),
		))

		rtChan <- types.RTResponse{
			&model.CollectorStatus{
				ActiveClients: 1,
				Interval:      10,
			},
		}
		assert.Never(t, func() bool {
			return r.(*runnerImpl).IsRealtimeEnabled()
		}, 1*time.Second, 10*time.Millisecond)
	})
}

func TestProvidedChecks(t *testing.T) {
	r := fxutil.Test[runner.Component](t, fx.Options(
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
	))
	providedChecks := r.GetProvidedChecks()

	var checkNames []string
	for _, check := range providedChecks {
		checkNames = append(checkNames, check.Object().Name())
	}
	t.Log("Provided Checks:", checkNames)

	assert.Len(t, providedChecks, 2)
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
