// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build clusterchecks && kubeapiserver && test

package autodiscoveryimpl

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/loaders"
	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	checkscheduler "github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/collector/worker"
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

// schedulerCollector is a minimal collector.Component that forwards
// RunCheck/StopCheck to a real *checkscheduler.Scheduler (same scheduler
// production uses inside collectorImpl). Enter/Cancel handle the jobQueue
// lifecycle, so the worker drives the check on the check's own Interval.
type schedulerCollector struct {
	sch *checkscheduler.Scheduler
}

func (c *schedulerCollector) RunCheck(ch check.Check) (checkid.ID, error) {
	if err := c.sch.Enter(ch); err != nil {
		return "", err
	}
	return ch.ID(), nil
}

func (c *schedulerCollector) StopCheck(id checkid.ID) error {
	return c.sch.Cancel(id)
}

func (c *schedulerCollector) ReloadAllCheckInstances(string, []check.Check) ([]checkid.ID, error) {
	return nil, nil
}
func (c *schedulerCollector) GetChecks() []check.Check                   { return nil }
func (c *schedulerCollector) MapOverChecks(cb func([]check.Info))        { cb(nil) }
func (c *schedulerCollector) AddEventReceiver(_ collector.EventReceiver) {}

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

// setupPipeline wires runner→scheduler.Scheduler→schedulerCollector→
// CheckScheduler→AutoConfig using production constructors. The
// MockScheduler returned alongside gives race-free Schedule/Unschedule
// counters for the assertions.
func setupPipeline(t *testing.T) (*AutoConfig, *MockScheduler) {
	t.Helper()
	expvars.Reset()
	worker.ResetTrialCallbacksForTest()
	t.Cleanup(worker.ResetTrialCallbacksForTest)
	t.Cleanup(func() { setTrialRunFn(nil) })

	// Allow sub-second intervals so trial-threshold failures complete in
	// milliseconds (default minimum is 1s). Same approach as
	// pkg/collector/scheduler/scheduler_test.go.
	prev := checkscheduler.SetMinAllowedIntervalForTest(time.Millisecond)
	t.Cleanup(func() { checkscheduler.SetMinAllowedIntervalForTest(prev) })

	ms, ac, deps := getResolveTestSetup(t)

	r := runner.NewRunner(aggregator.NewNoOpSenderManager(), haagentmock.NewMockHaAgent(), healthplatformmock.Mock(t))
	t.Cleanup(r.Stop)

	sch := checkscheduler.NewScheduler(r.GetChan())
	sch.Run()
	// Stop the scheduler before the runner so its jobQueue goroutines don't
	// race with the runner closing pendingChecksChan (LIFO cleanup order).
	t.Cleanup(func() { _ = sch.Stop() })
	coll := &schedulerCollector{sch: sch}

	cs := pkgcollector.InitCheckScheduler(
		option.New[collector.Component](coll),
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
func containsConfigNamed(ac *AutoConfig, name string) bool {
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

	setTrialRunFn(func(uint64) error { return errors.New("trial probe failed") })

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
		return ms.schedules.Load() == 1 && ms.unschedules.Load() == 1
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
		return int(expvars.GetErrorsCount()) >= trialFailureThreshold+1
	}, 10*time.Second, 20*time.Millisecond,
		"post-promotion failures must be reported as integration errors, proving promotion isolates the trial path")

	// Promotion must keep AD out of the trial-counter loop — no Unschedule
	// dispatch should fire despite many post-promotion failures.
	assert.Equal(t, int64(1), ms.schedules.Load(), "exactly one Schedule")
	assert.Equal(t, int64(0), ms.unschedules.Load(), "no Unschedule after promotion")
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
		return int(expvars.GetErrorsCount()) >= trialFailureThreshold+3
	}, 10*time.Second, 20*time.Millisecond)

	assert.Equal(t, int64(1), ms.schedules.Load(), "exactly one Schedule")
	assert.Equal(t, int64(0), ms.unschedules.Load(),
		"non-discovery check must never trigger the trial-path unschedule")
	assert.True(t, containsConfigNamed(ac, "regular_check"))
}
