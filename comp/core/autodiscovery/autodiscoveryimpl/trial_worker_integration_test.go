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

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	healthplatformmock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/DataDog/datadog-agent/pkg/collector/check/stub"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/tracker"
	"github.com/DataDog/datadog-agent/pkg/collector/worker"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// fakeCheck is a minimal check.Check whose ID is set explicitly so the
// integration test can verify that the ID built by AD's BuildID matches what
// the worker reports back via notifyTrialResult.
type fakeCheck struct {
	stub.StubCheck
	mu       sync.Mutex
	id       checkid.ID
	runCount *atomic.Uint64
	runFunc  func(uint64) error
}

func (c *fakeCheck) ID() checkid.ID          { return c.id }
func (c *fakeCheck) String() string          { return string(c.id) }
func (c *fakeCheck) Interval() time.Duration { return time.Second }
func (c *fakeCheck) Run() error {
	c.mu.Lock()
	n := c.runCount.Inc() - 1
	fn := c.runFunc
	c.mu.Unlock()
	if fn == nil {
		return nil
	}
	return fn(n)
}
func (c *fakeCheck) GetSenderStats() (stats.SenderStats, error) {
	return stats.NewSenderStats(), nil
}

// runChecks pushes the provided checks through a real worker.Worker, closes
// the channel, and blocks until the worker has finished processing every item.
// All assertions in the integration tests run after this returns, so they
// observe stable post-run state (no races with in-flight error reporting).
func runChecks(t *testing.T, checks []check.Check) {
	t.Helper()
	expvars.Reset()
	pending := make(chan check.Check, len(checks))
	for _, c := range checks {
		pending <- c
	}
	close(pending)

	checksTracker := tracker.NewRunningChecksTracker()
	w, err := worker.NewWorker(
		aggregator.NewNoOpSenderManager(),
		haagentmock.NewMockHaAgent(),
		healthplatformmock.Mock(t),
		0, // runnerID
		1, // ID
		pending,
		checksTracker,
		func(checkid.ID) bool { return true },
		0,
	)
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		w.Run(context.Background())
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not finish draining checks within 10s")
	}
}

// scheduleDiscoveryConfig schedules a discovery config through the real AD
// pipeline and returns the resulting integration.Config together with the
// check ID AD would expect to receive from a worker reporting on that check.
func scheduleDiscoveryConfig(t *testing.T, ac *AutoConfig, name string) (integration.Config, checkid.ID) {
	t.Helper()
	cfg := integration.Config{
		Name:       name,
		Discovery:  &integration.DiscoveryConfig{},
		InitConfig: integration.Data("{}"),
		Instances:  []integration.Data{integration.Data("{}")},
	}
	changes := ac.processNewConfig(cfg)
	require.Len(t, changes.Schedule, 1)
	ac.applyChanges(changes)
	scheduled := changes.Schedule[0]
	id := checkid.BuildID(scheduled.Name, scheduled.FastDigest(), scheduled.Instances[0], scheduled.InitConfig)
	return scheduled, id
}

// TestADWorkerIntegration_UnschedulesAfterThresholdFailures wires a real
// worker.Worker to a real AutoConfig via the same RegisterTrialResultCallback
// call used in production. It pushes a wrapped trial check through the worker
// trialFailureThreshold times, then asserts AD has unscheduled the config.
//
// This is the only test that exercises the check-ID contract between
// scheduler-loaded check.ID() and AD's popConfig lookup: if the worker reports
// an ID that does not match AD's BuildID(...), the unschedule silently no-ops
// (with only a Warnf), and this test catches it.
func TestADWorkerIntegration_UnschedulesAfterThresholdFailures(t *testing.T) {
	configmock.New(t)
	worker.ResetTrialCallbacksForTest()
	t.Cleanup(worker.ResetTrialCallbacksForTest)

	sch, ac := getResolveTestConfig(t)

	scheduled, id := scheduleDiscoveryConfig(t, ac, "krakend")
	require.Eventually(t, func() bool {
		return sch.scheduledSize() == 1
	}, 5*time.Second, 10*time.Millisecond, "config should be scheduled before failures are reported")
	require.Contains(t, scheduledConfigNames(ac), scheduled.Name)

	tc := &fakeCheck{
		id:       id,
		runCount: atomic.NewUint64(0),
		runFunc:  func(uint64) error { return errors.New("trial probe failed") },
	}
	wrapped := check.NewTrialModeCheck(tc)

	checks := make([]check.Check, 0, trialFailureThreshold)
	for i := 0; i < trialFailureThreshold; i++ {
		checks = append(checks, wrapped)
	}
	runChecks(t, checks)

	require.Eventually(t, func() bool {
		return sch.scheduledSize() == 0
	}, 5*time.Second, 10*time.Millisecond, "AD must unschedule after trialFailureThreshold failures arrive via the real worker→callback→recordTrialResult chain")
	require.NotContains(t, scheduledConfigNames(ac), scheduled.Name,
		"unscheduling must also remove the config from scheduledConfigs so GetAllConfigs stays consistent")

	// Trial-mode errors must not have leaked into the global integration-error
	// counter — that is the worker's responsibility, but we verify the
	// end-to-end behavior here.
	assert.Equal(t, 0, int(expvars.GetErrorsCount()),
		"trial-mode failures must not be counted as integration errors")
}

// TestADWorkerIntegration_SuccessPromotesAndIsolatesFromAD verifies that once
// the worker promotes a check out of trial mode (after the first success), it
// stops reporting outcomes to AD. Subsequent failures must not accumulate in
// AD's trialRegistry and must not unschedule the config — they should be
// counted as normal integration errors instead.
func TestADWorkerIntegration_SuccessPromotesAndIsolatesFromAD(t *testing.T) {
	configmock.New(t)
	worker.ResetTrialCallbacksForTest()
	t.Cleanup(worker.ResetTrialCallbacksForTest)

	sch, ac := getResolveTestConfig(t)

	scheduled, id := scheduleDiscoveryConfig(t, ac, "krakend")
	require.Eventually(t, func() bool {
		return sch.scheduledSize() == 1
	}, 5*time.Second, 10*time.Millisecond)

	// Run 1 succeeds; runs 2..N fail.
	tc := &fakeCheck{
		id:       id,
		runCount: atomic.NewUint64(0),
		runFunc: func(n uint64) error {
			if n == 0 {
				return nil
			}
			return errors.New("post-promotion failure")
		},
	}
	wrapped := check.NewTrialModeCheck(tc)

	const totalRuns = trialFailureThreshold + 1 // 1 success + threshold failures
	checks := make([]check.Check, 0, totalRuns)
	for i := 0; i < totalRuns; i++ {
		checks = append(checks, wrapped)
	}
	runChecks(t, checks)
	require.Equal(t, uint64(totalRuns), tc.runCount.Load(), "every queued run should have executed")

	// The config must still be scheduled — the first run cleared the trial
	// counter and the wrapper promoted out of trial mode, so the later
	// failures never made it back to AD.
	assert.Equal(t, 1, sch.scheduledSize(), "config must remain scheduled after promotion")
	assert.Contains(t, scheduledConfigNames(ac), scheduled.Name)

	ac.trialRegistry.mu.Lock()
	_, hasCounter := ac.trialRegistry.counts[id]
	ac.trialRegistry.mu.Unlock()
	assert.False(t, hasCounter, "trialRegistry counter must stay clear once the check is promoted")

	// Post-promotion failures must be counted as normal integration errors,
	// which is the contract the trial path is supposed to preserve.
	assert.GreaterOrEqual(t, int(expvars.GetErrorsCount()), trialFailureThreshold,
		"post-promotion failures must be reported via the normal integration-error path")
}

// TestADWorkerIntegration_NonDiscoveryCheckNeverTriggersTrialPath verifies
// that the worker→AD coupling only fires for trial-mode (discovery) checks.
// A regular check failing repeatedly must not be unscheduled by the trial
// path, since it is not wrapped in NewTrialModeCheck and therefore does not
// implement the trialModeCheck interface.
func TestADWorkerIntegration_NonDiscoveryCheckNeverTriggersTrialPath(t *testing.T) {
	configmock.New(t)
	worker.ResetTrialCallbacksForTest()
	t.Cleanup(worker.ResetTrialCallbacksForTest)

	sch, ac := getResolveTestConfig(t)

	// Schedule a non-discovery config (Discovery is nil).
	cfg := integration.Config{
		Name:       "regular_check",
		InitConfig: integration.Data("{}"),
		Instances:  []integration.Data{integration.Data("{}")},
	}
	changes := ac.processNewConfig(cfg)
	require.Len(t, changes.Schedule, 1)
	ac.applyChanges(changes)
	scheduled := changes.Schedule[0]
	id := checkid.BuildID(scheduled.Name, scheduled.FastDigest(), scheduled.Instances[0], scheduled.InitConfig)
	require.Eventually(t, func() bool {
		return sch.scheduledSize() == 1
	}, 5*time.Second, 10*time.Millisecond)

	tc := &fakeCheck{
		id:       id,
		runCount: atomic.NewUint64(0),
		runFunc:  func(uint64) error { return errors.New("regular failure") },
	}

	// Feed the bare (un-wrapped) check N>threshold times.
	const runs = trialFailureThreshold + 3
	checks := make([]check.Check, 0, runs)
	for i := 0; i < runs; i++ {
		checks = append(checks, tc)
	}
	runChecks(t, checks)
	require.Equal(t, uint64(runs), tc.runCount.Load())

	// Trial-registry must remain untouched and the config must still be
	// scheduled — the worker has no reason to invoke the trial callback for a
	// check that does not implement trialModeCheck.
	ac.trialRegistry.mu.Lock()
	_, hasCounter := ac.trialRegistry.counts[id]
	ac.trialRegistry.mu.Unlock()
	assert.False(t, hasCounter, "non-trial checks must never appear in trialRegistry")
	assert.Equal(t, 1, sch.scheduledSize(), "non-trial check must stay scheduled regardless of run outcomes")
}
