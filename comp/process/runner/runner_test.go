// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package runner

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/process/containercheck"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo"
	"github.com/DataDog/datadog-agent/comp/process/processcheck"
	"github.com/DataDog/datadog-agent/comp/process/submitter"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/comp/process/utils"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRunnerLifecycle(t *testing.T) {
	fxutil.Test(t, fx.Options(
		utils.DisableContainerFeatures,

		fx.Supply(core.BundleParams{}),

		Module,
		submitter.MockModule,
		processcheck.Module,
		hostinfo.MockModule,
		core.MockBundle,
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
			fx.Provide(
				// Cast `chan types.RTResponse` to `<-chan types.RTResponse`.
				// We can't use `fx.As` because `<-chan types.RTResponse` is not an interface.
				func() <-chan types.RTResponse { return rtChan },
			),

			utils.DisableContainerFeatures,

			fx.Supply(core.BundleParams{}),

			Module,
			submitter.MockModule,
			processcheck.Module,
			hostinfo.MockModule,
			core.MockBundle,
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

			utils.DisableContainerFeatures,

			fx.Supply(core.BundleParams{}),

			Module,
			submitter.MockModule,
			processcheck.Module,
			hostinfo.MockModule,
			core.MockBundle,
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
	config.SetDetectedFeatures(config.FeatureMap{config.Docker: {}})
	t.Cleanup(func() { config.SetDetectedFeatures(nil) })

	fxutil.Test(t, fx.Options(
		fx.Supply(
			core.BundleParams{},
		),

		Module,
		submitter.MockModule,
		hostinfo.MockModule,

		// Checks
		processcheck.MockModule,
		containercheck.MockModule,

		core.MockBundle,
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
