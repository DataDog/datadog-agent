// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver && test

// This test lives in package autodiscoveryimpl_test (not autodiscoveryimpl)
// because it imports collectorimpl, and collectorimpl already imports
// autodiscoveryimpl via agent_check_metadata.go. The external test package
// avoids the cycle. Cross-package helpers come from export_test.go in
// autodiscoveryimpl.
package autodiscoveryimpl_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/collector/collector/collectorimpl"
	agenttelemetry "github.com/DataDog/datadog-agent/comp/core/agenttelemetry/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	healthplatformnoopimpl "github.com/DataDog/datadog-agent/comp/healthplatform/store/noop-impl"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	checkscheduler "github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/collector/worker"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// trialTestState carries per-test behavior into the package-level test loader.
// Tests in this file run sequentially (no t.Parallel), so a single global slot
// is enough.
var trialTestState struct {
	mu    sync.Mutex
	runFn func(n uint64) error
}

func setTrialRunFn(fn func(n uint64) error) {
	trialTestState.mu.Lock()
	defer trialTestState.mu.Unlock()
	trialTestState.runFn = fn
}

func init() {
	// Register the test loader once, before any code (in particular
	// pkgcollector.InitCheckScheduler) calls loaders.LoaderCatalog and locks
	// the catalog via sync.Once. Without this, the catalog would only contain
	// production loaders, none of which know how to load our synthetic
	// discovery configs.
	loaders.RegisterLoader(func(sender.SenderManager, option.Option[integrations.Component], tagger.Component, workloadfilter.Component) (check.Loader, int, error) {
		return &trialTestLoader{}, 0, nil
	})
}

// trialTestCheck has an ID built via (*CheckBase).BuildID so it matches AD's
// popConfig formula at configmgr.go:591. Run() consults the package-level
// runFn so each test can control per-run outcomes.
type trialTestCheck struct {
	core.CheckBase
	runCount *atomic.Uint64
}

// Interval is a small positive value so the internal scheduler's jobQueue
// (re-)emits this check at sub-second cadence. setupPipeline lowers the
// scheduler's minAllowedInterval to permit this.
func (c *trialTestCheck) Interval() time.Duration { return 5 * time.Millisecond }

func (c *trialTestCheck) Run() error {
	n := c.runCount.Inc() - 1
	trialTestState.mu.Lock()
	fn := trialTestState.runFn
	trialTestState.mu.Unlock()
	if fn == nil {
		return nil
	}
	return fn(n)
}

// GetSenderStats overrides CheckBase: the embedded base would dereference a
// nil senderManager (the loader never calls Configure), so the worker's
// stats-gathering call panics. Returning empty stats is fine — the tests
// assert on AD state and expvars, not on per-check sender stats.
func (c *trialTestCheck) GetSenderStats() (stats.SenderStats, error) {
	return stats.NewSenderStats(), nil
}

type trialTestLoader struct{}

func (l *trialTestLoader) Name() string { return "trial-integration-test" }

func (l *trialTestLoader) Load(_ sender.SenderManager, config integration.Config, instance integration.Data, _ int) (check.Check, error) {
	c := &trialTestCheck{
		CheckBase: core.NewCheckBase(config.Name),
		runCount:  atomic.NewUint64(0),
	}
	c.BuildID(config.FastDigest(), instance, config.InitConfig)
	return c, nil
}

// collectorTestDeps captures the real collector.Component built via
// collectorimpl.Module() so the test pipeline exercises the same StopCheck
// path production uses (collectorImpl.StopCheck → runner.StopCheck →
// CheckWrapper.Cancel) rather than a hand-rolled scheduler-only fake.
type collectorTestDeps struct {
	fx.In
	Coll option.Option[collector.Component]
}

// trialTestProvider yields the prepared configs on first Collect, then empty.
type trialTestProvider struct {
	configs   []integration.Config
	collected bool
}

func (p *trialTestProvider) String() string                                { return "trial-integration-test" }
func (p *trialTestProvider) GetConfigErrors() map[string]types.ErrorMsgSet { return nil }
func (p *trialTestProvider) Collect(context.Context) ([]integration.Config, error) {
	if p.collected {
		return nil, nil
	}
	p.collected = true
	return p.configs, nil
}
func (p *trialTestProvider) IsUpToDate(context.Context) (bool, error) { return true, nil }

// setupPipeline wires runner→scheduler→collectorImpl→CheckScheduler→AutoConfig
// using production constructors. The collector is the real
// collectorimpl.Component (built via fx), so this test exercises
// collectorImpl.StopCheck / runner.StopCheck / CheckWrapper.Cancel — the
// production code that the trial-failure unschedule path actually invokes
// while the worker still has the check marked as running in the
// RunningChecksTracker. The MockScheduler returned alongside gives race-free
// Schedule/Unschedule counters for the assertions; AD dispatches changes to
// both schedulers, so it observes every Unschedule the trial path produces.
func setupPipeline(t *testing.T) (*autodiscoveryimpl.AutoConfig, *autodiscoveryimpl.MockScheduler) {
	t.Helper()
	expvars.Reset()
	worker.ResetTrialCallbacks()
	// Register reset BEFORE fxutil.Test below so the LIFO order is:
	//   1. fxutil.Test's RequireStop → collectorImpl.stop (drains in-flight
	//      worker runs and any late notifyTrialResult)
	//   2. ResetTrialCallbacks (drops AD's registered callback)
	// If we registered the reset after fxutil.Test, a late callback could
	// fire into AutoConfig after the test has already torn AD down.
	t.Cleanup(worker.ResetTrialCallbacks)
	t.Cleanup(func() { setTrialRunFn(nil) })

	// Allow sub-second intervals so trial-threshold failures complete in
	// milliseconds (default minimum is 1s). Same approach as
	// pkg/collector/scheduler/scheduler_test.go. Applies to the scheduler
	// created internally by collectorImpl.start().
	prev := checkscheduler.SetMinAllowedInterval(time.Millisecond)
	t.Cleanup(func() { checkscheduler.SetMinAllowedInterval(prev) })

	ms, ac, deps := autodiscoveryimpl.GetResolveTestSetup(t)

	cd := fxutil.Test[collectorTestDeps](t,
		collectorimpl.Module(),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component {
			return config.NewMockWithOverrides(t, map[string]interface{}{
				"check_cancel_timeout": 500 * time.Millisecond,
			})
		}),
		hostnameimpl.MockModule(),
		demultiplexerimpl.MockModule(),
		haagentmock.Module(),
		fx.Provide(func() healthplatform.Component {
			return healthplatformnoopimpl.NewNoopComponent()
		}),
		fx.Provide(func() option.Option[serializer.MetricSerializer] {
			return option.None[serializer.MetricSerializer]()
		}),
		fx.Provide(func() option.Option[agenttelemetry.Component] {
			return option.None[agenttelemetry.Component]()
		}),
	)

	cs := pkgcollector.InitCheckScheduler(
		cd.Coll,
		aggregator.NewNoOpSenderManager(),
		option.None[integrations.Component](),
		deps.TaggerComp,
		deps.FilterComp,
	)
	ac.AddScheduler("check", cs, false)

	return ac, ms
}

// containsConfigNamed reports whether the AD-known configs include name.
// Uses the public GetAllConfigs API rather than reaching into cfgMgr.
func containsConfigNamed(ac *autodiscoveryimpl.AutoConfig, name string) bool {
	for _, c := range ac.GetAllConfigs() {
		if c.Name == name {
			return true
		}
	}
	return false
}

// TestADWorkerIntegration_UnschedulesAfterThresholdFailures wires the
// production AD pipeline to a real worker.Worker via the same
// RegisterTrialResultCallback call used in production. A discovery config
// arrives via a config provider; the controller dispatches to the real
// CheckScheduler whose GetChecksFromConfigs is the wrap site we want to
// guard. After trialFailureThreshold failed runs, AD must unschedule.
//
// This is the integration test that catches a regression in
// pkg/collector/scheduler.go:278-281 (the
// "if config.IsDiscovery() { c = check.NewTrialModeCheck(c) }" block).
// Without that wrap, the unwrapped check fails the worker's trialModeCheck
// type assertion, no trial callback fires, and AD never unschedules.
func TestADWorkerIntegration_UnschedulesAfterThresholdFailures(t *testing.T) {
	ac, ms := setupPipeline(t)

	// Fail for exactly trialFailureThreshold runs; afterwards, return nil.
	// Once the threshold is crossed and AD unschedules, the real collector's
	// CheckWrapper.Cancel races a goroutine that flips its internal `done`
	// flag — a small number of stale enqueues (already in flight before
	// scheduler.Cancel ran) can slip through and reach the worker. Making
	// those late runs return nil keeps them on the suppress/promote path and
	// prevents the spurious integration-error counts that would otherwise
	// make this test flaky. The 5-failure trigger remains the load-bearing
	// part of the test.
	setTrialRunFn(func(n uint64) error {
		if int(n) < autodiscoveryimpl.TrialFailureThreshold {
			return errors.New("trial probe failed")
		}
		return nil
	})

	provider := &trialTestProvider{
		configs: []integration.Config{{
			Name:       "krakend_threshold",
			Discovery:  &integration.DiscoveryConfig{},
			InitConfig: integration.Data("{}"),
			Instances:  []integration.Data{integration.Data("{}")},
		}},
	}
	ac.AddConfigProvider(provider, false, 0)
	ac.LoadAndRun(context.Background())

	require.Eventually(t, func() bool {
		return ms.Schedules() == 1 && ms.Unschedules() == 1
	}, 10*time.Second, 20*time.Millisecond,
		"AD must dispatch Schedule then Unschedule after trialFailureThreshold failures arrive via the real worker→callback→recordTrialResult chain")
	assert.False(t, containsConfigNamed(ac, "krakend_threshold"),
		"unscheduling must also drop the config from GetAllConfigs")
	assert.Equal(t, 0, int(expvars.GetErrorsCount()),
		"trial-mode failures must not be counted as integration errors")
}

// TestADWorkerIntegration_SuccessPromotesAndIsolatesFromAD verifies that once
// the worker promotes a check out of trial mode (after the first success), it
// stops reporting outcomes to AD. Subsequent failures must not accumulate in
// AD's trialRegistry and must not unschedule the config — they should flow
// through the normal integration-error path.
func TestADWorkerIntegration_SuccessPromotesAndIsolatesFromAD(t *testing.T) {
	ac, ms := setupPipeline(t)

	// Run 0 succeeds (promotes out of trial mode); runs 1..N fail.
	setTrialRunFn(func(n uint64) error {
		if n == 0 {
			return nil
		}
		return errors.New("post-promotion failure")
	})

	provider := &trialTestProvider{
		configs: []integration.Config{{
			Name:       "krakend_promotion",
			Discovery:  &integration.DiscoveryConfig{},
			InitConfig: integration.Data("{}"),
			Instances:  []integration.Data{integration.Data("{}")},
		}},
	}
	ac.AddConfigProvider(provider, false, 0)
	ac.LoadAndRun(context.Background())

	// Wait for at least trialFailureThreshold+1 errors to land in the global
	// counter — these are the post-promotion failures flowing through the
	// normal worker error-reporting path. Their existence is the load-bearing
	// signal that the trial callback did NOT fire for them.
	require.Eventually(t, func() bool {
		return int(expvars.GetErrorsCount()) >= autodiscoveryimpl.TrialFailureThreshold+1
	}, 10*time.Second, 20*time.Millisecond,
		"post-promotion failures must be reported as integration errors, proving promotion isolates the trial path")

	// Promotion must keep AD out of the trial-counter loop — no Unschedule
	// dispatch should fire despite many post-promotion failures.
	assert.Equal(t, int64(1), ms.Schedules(), "exactly one Schedule")
	assert.Equal(t, int64(0), ms.Unschedules(), "no Unschedule after promotion")
	assert.True(t, containsConfigNamed(ac, "krakend_promotion"))
}

// TestADWorkerIntegration_NonDiscoveryCheckNeverTriggersTrialPath verifies
// that the worker→AD coupling fires only for trial-mode (discovery) checks.
// A non-discovery config produces an unwrapped check; the worker's trial
// type-assertion fails on every run; AD never sees a trial result; the
// config stays scheduled even after many failures.
func TestADWorkerIntegration_NonDiscoveryCheckNeverTriggersTrialPath(t *testing.T) {
	ac, ms := setupPipeline(t)

	setTrialRunFn(func(uint64) error { return errors.New("regular failure") })

	provider := &trialTestProvider{
		configs: []integration.Config{{
			Name:       "regular_check",
			InitConfig: integration.Data("{}"),
			Instances:  []integration.Data{integration.Data("{}")},
			// Discovery is nil — this is NOT a discovery config.
		}},
	}
	ac.AddConfigProvider(provider, false, 0)
	ac.LoadAndRun(context.Background())

	// Wait for enough failures to land — equivalent to what would have
	// crossed the threshold if this were a discovery config.
	require.Eventually(t, func() bool {
		return int(expvars.GetErrorsCount()) >= autodiscoveryimpl.TrialFailureThreshold+3
	}, 10*time.Second, 20*time.Millisecond)

	assert.Equal(t, int64(1), ms.Schedules(), "exactly one Schedule")
	assert.Equal(t, int64(0), ms.Unschedules(),
		"non-discovery check must never trigger the trial-path unschedule")
	assert.True(t, containsConfigNamed(ac, "regular_check"))
}
