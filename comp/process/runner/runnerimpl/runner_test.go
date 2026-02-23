// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package runnerimpl

import (
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/process/containercheck/containercheckimpl"
	"github.com/DataDog/datadog-agent/comp/process/hostinfo/hostinfoimpl"
	"github.com/DataDog/datadog-agent/comp/process/processcheck/processcheckimpl"
	"github.com/DataDog/datadog-agent/comp/process/runner"
	"github.com/DataDog/datadog-agent/comp/process/submitter/submitterimpl"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
)

func TestRunnerLifecycle(t *testing.T) {
	_ = createDeps(t, nil)
}

func TestRunnerRealtime(t *testing.T) {
	// https://datadoghq.atlassian.net/browse/CXP-2284
	flake.Mark(t)

	enableProcessAgent(t)

	t.Run("rt allowed", func(t *testing.T) {
		rtChan := make(chan types.RTResponse)
		defer close(rtChan)

		deps := createDeps(t,
			map[string]interface{}{"process_config.disable_realtime_checks": false},
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
		assert.Eventually(t, func() bool {
			return deps.Runner.(*runnerImpl).IsRealtimeEnabled()
		}, 1*time.Second, 10*time.Millisecond)
	})

	t.Run("rt disallowed", func(t *testing.T) {
		// Buffer the channel because the runner will never consume from it, otherwise we will deadlock
		rtChan := make(chan types.RTResponse, 1)
		defer close(rtChan)

		deps := createDeps(t,
			map[string]interface{}{"process_config.disable_realtime_checks": true},
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
	deps := createDeps(t, nil)
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

func createDeps(t *testing.T, confOverrides map[string]interface{}, options ...fx.Option) Deps {
	return fxutil.Test[Deps](t, fx.Options(
		Module(),
		submitterimpl.MockModule(),
		hostinfoimpl.MockModule(),

		// Checks
		processcheckimpl.MockModule(),
		containercheckimpl.MockModule(),

		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		fx.Provide(func(t testing.TB) config.Component { return config.NewMockWithOverrides(t, confOverrides) }),
		workloadmetafx.Module(workloadmeta.NewParams()),
		fx.Provide(func(t testing.TB) tagger.Component { return taggerfxmock.SetupFakeTagger(t) }),
		fx.Options(options...),
		sysprobeconfigimpl.MockModule(),
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
