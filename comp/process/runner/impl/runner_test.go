// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package impl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	containercheck "github.com/DataDog/datadog-agent/comp/process/containercheck/impl"
	hostinfo "github.com/DataDog/datadog-agent/comp/process/hostinfo/impl"
	processcheckComp "github.com/DataDog/datadog-agent/comp/process/processcheck/impl"
	runnerComp "github.com/DataDog/datadog-agent/comp/process/runner"
	submitter "github.com/DataDog/datadog-agent/comp/process/submitter/impl"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRunnerLifecycle(t *testing.T) {
	_ = fxutil.Test[runnerComp.Component](t, fx.Options(
		fx.Supply(core.BundleParams{}),

		Module,
		submitter.MockModule,
		processcheckComp.Module,
		hostinfo.MockModule,
		core.MockBundle,
	))
}

func TestRunnerRealtime(t *testing.T) {
	t.Run("rt allowed", func(t *testing.T) {
		rtChan := make(chan types.RTResponse)

		r := fxutil.Test[runnerComp.Component](t, fx.Options(
			fx.Provide(
				// Cast `chan types.RTResponse` to `<-chan types.RTResponse`.
				// We can't use `fx.As` because `<-chan types.RTResponse` is not an interface.
				func() <-chan types.RTResponse { return rtChan },
			),

			fx.Supply(core.BundleParams{}),
			fx.Replace(config.MockParams{Overrides: map[string]interface{}{
				"process_config.disable_realtime_checks": false,
			}}),

			Module,
			submitter.MockModule,
			processcheckComp.Module,
			hostinfo.MockModule,
			core.MockBundle,
		))
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

	t.Run("rt disallowed", func(t *testing.T) {
		// Buffer the channel because the runner will never consume from it, otherwise we will deadlock
		rtChan := make(chan types.RTResponse, 1)

		r := fxutil.Test[runnerComp.Component](t, fx.Options(
			fx.Supply(core.BundleParams{}),
			fx.Replace(config.MockParams{Overrides: map[string]interface{}{
				"process_config.disable_realtime_checks": true,
			}}),

			fx.Provide(
				// Cast `chan types.RTResponse` to `<-chan types.RTResponse`.
				// We can't use `fx.As` because `<-chan types.RTResponse` is not an interface.
				func() <-chan types.RTResponse { return rtChan },
			),

			Module,
			submitter.MockModule,
			processcheckComp.Module,
			hostinfo.MockModule,
			core.MockBundle,
		))

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
}

func TestProvidedChecks(t *testing.T) {
	r := fxutil.Test[runnerComp.Component](t, fx.Options(
		fx.Supply(
			core.BundleParams{},
		),

		Module,
		submitter.MockModule,
		hostinfo.MockModule,

		// Checks
		processcheckComp.MockModule,
		containercheck.MockModule,

		core.MockBundle,
	))
	providedChecks := r.GetProvidedChecks()

	var checkNames []string
	for _, check := range providedChecks {
		checkNames = append(checkNames, check.Object().Name())
	}
	t.Log("Provided Checks:", checkNames)

	assert.Len(t, providedChecks, 2)
}
