// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logsprofile

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/profilerec"
)

const (
	sendStage     = "destination_reliable_0"
	minHealthyFor = 10 * time.Minute
)

type env struct {
	c        *checker
	now      time.Time
	running  bool
	summary  logsmetrics.BackpressureSummary
	counters profilerec.Counters
	active   string
	plan     func(string) (profilerec.Plan, bool)
}

func changePlan(profile string) (profilerec.Plan, bool) {
	return profilerec.Plan{
		Name:             profile,
		Version:          1,
		Description:      "Raises send concurrency.",
		ProfileKeySource: "default",
		Current:          []profilerec.PlanSetting{{Key: "logs_config.batch_max_concurrent_send", Value: 0, Source: "default"}},
		Changes:          []profilerec.PlanChange{{Key: "logs_config.batch_max_concurrent_send", From: 0, To: 10}},
	}, true
}

func newEnv(t *testing.T, yaml string) *env {
	t.Helper()
	hn, _ := hostnamemock.NewMock(hostnamemock.MockHostname("host-a"))
	e := &env{
		c:       newChecker(config.NewMockFromYAML(t, yaml), hn),
		now:     time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
		running: true,
		summary: healthy(),
		plan:    changePlan,
	}
	e.c.now = func() time.Time { return e.now }
	e.c.started = e.now
	e.c.logsRunning = func() bool { return e.running }
	e.c.backpressure = func() logsmetrics.BackpressureSummary { return e.summary }
	e.c.counters = func() profilerec.Counters { return e.counters }
	e.c.activeProfile = func() string { return e.active }
	e.c.plan = func(p string) (profilerec.Plan, bool) { return e.plan(p) }
	return e
}

// newReadyEnv returns an env past the restart warm-up with the loss window seeded.
func newReadyEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t, "logs_enabled: true")
	_, err := e.c.Run()
	require.ErrorIs(t, err, runnerdef.ErrStateUnknown)
	e.now = e.now.Add(minHealthyFor + time.Minute)
	reports, err := e.c.Run()
	require.NoError(t, err)
	require.Empty(t, reports)
	return e
}

func (e *env) tick(advance time.Duration) ([]runnerdef.IssueReport, error) {
	e.now = e.now.Add(advance)
	return e.c.Run()
}

func healthy() logsmetrics.BackpressureSummary {
	return logsmetrics.DeriveBackpressure([]logsmetrics.ComponentSnapshot{
		logsmetrics.SaturatedSnapshotForTest("processor", "0", 0.1, 0, false),
		logsmetrics.SaturatedSnapshotForTest(sendStage, "0", 0.1, 0, false),
	})
}

func saturated(component string, for30m time.Duration, now bool) logsmetrics.BackpressureSummary {
	return logsmetrics.DeriveBackpressure([]logsmetrics.ComponentSnapshot{
		logsmetrics.SaturatedSnapshotForTest("processor", "0", 0.1, 0, false),
		logsmetrics.SaturatedSnapshotForTest(component, "0", 0.98, for30m, now),
	})
}

func (e *env) report(t *testing.T, advance time.Duration) runnerdef.IssueReport {
	t.Helper()
	reports, err := e.tick(advance)
	require.NoError(t, err)
	require.Len(t, reports, 1)
	return reports[0]
}

func decodeWire(t *testing.T, r runnerdef.IssueReport) recommendation {
	t.Helper()
	var w recommendation
	require.NoError(t, json.Unmarshal([]byte(r.Context[contextKeyRecommendation]), &w))
	return w
}

func TestRun_GatesResolveOrKeepState(t *testing.T) {
	t.Run("disabled resolves", func(t *testing.T) {
		e := newEnv(t, "logs_enabled: true\nhealth_platform:\n  logs_profile_recommendation:\n    enabled: false")
		reports, err := e.tick(0)
		assert.NoError(t, err)
		assert.Empty(t, reports)
	})
	t.Run("logs disabled resolves", func(t *testing.T) {
		e := newEnv(t, "logs_enabled: false")
		reports, err := e.tick(0)
		assert.NoError(t, err)
		assert.Empty(t, reports)
	})
	t.Run("deprecated log_enabled still runs", func(t *testing.T) {
		e := newEnv(t, "log_enabled: true")
		e.running = false
		_, err := e.tick(0)
		assert.ErrorIs(t, err, runnerdef.ErrStateUnknown)
	})
	t.Run("logs agent not running is unknown", func(t *testing.T) {
		e := newEnv(t, "logs_enabled: true")
		e.running = false
		reports, err := e.tick(0)
		assert.ErrorIs(t, err, runnerdef.ErrStateUnknown)
		assert.Empty(t, reports)
	})
	t.Run("no pipeline monitor is unknown", func(t *testing.T) {
		e := newEnv(t, "logs_enabled: true")
		e.summary = logsmetrics.BackpressureSummary{}
		_, err := e.tick(0)
		assert.ErrorIs(t, err, runnerdef.ErrStateUnknown)
	})
}

func TestRun_LossReportsHigh(t *testing.T) {
	e := newReadyEnv(t)
	e.summary = saturated(sendStage, 27*time.Minute, true)
	e.counters = profilerec.Counters{Missed: 4096, SenderLatencyMs: 420}

	r := e.report(t, time.Minute)

	assert.Equal(t, IssueName, r.IssueName)
	assert.True(t, strings.HasPrefix(r.IssueID, IssueID+":"), r.IssueID)
	w := decodeWire(t, r)
	assert.Equal(t, profilerec.ProfileHighConcurrency, w.Profile)
	assert.Equal(t, profilerec.ReasonSendStageSaturatedHighLatency, w.ReasonCode)
	assert.Equal(t, sendStage, w.Bottleneck)
	assert.EqualValues(t, 420, w.SenderLatencyMs)
	assert.True(t, w.MissedRecently)

	issue, err := RecommendedIssue{}.BuildIssue(r.Context)
	require.NoError(t, err)
	assert.Contains(t, extraStrings(t, issue, "evidence"), "Intake latency: 420 ms")
	assert.Contains(t, extraStrings(t, issue, "evidence"), "Network send stage saturated for 27m of the last 30 minutes")
}

func TestRun_DropsAreNotLoss(t *testing.T) {
	e := newReadyEnv(t)
	e.summary = saturated(sendStage, 27*time.Minute, true)
	e.counters = profilerec.Counters{Dropped: 5, Sent: 10}

	assert.Equal(t, SuggestedIssueName, e.report(t, time.Minute).IssueName, "permanent send errors are not capacity loss")

	e.summary = healthy()
	e.counters.Dropped = 9
	e.report(t, time.Minute)
	reports, err := e.tick(minHealthyFor)
	require.NoError(t, err)
	assert.Empty(t, reports, "ongoing drops do not keep the issue open")
}

func TestRun_StalledSendStageDoesNotRecommend(t *testing.T) {
	e := newReadyEnv(t)
	e.summary = saturated(sendStage, 27*time.Minute, true)
	e.counters = profilerec.Counters{Errors: 2}
	_, err := e.tick(time.Minute)
	require.ErrorIs(t, err, runnerdef.ErrStateUnknown)

	e.counters = profilerec.Counters{Errors: 2, Missed: 4096}
	reports, err := e.tick(time.Minute)

	require.ErrorIs(t, err, runnerdef.ErrStateUnknown)
	assert.Empty(t, reports, "nothing moved, so the earlier send errors still mark an outage")
}

func TestRun_SustainedSaturationWithoutLossReportsLow(t *testing.T) {
	e := newReadyEnv(t)
	e.summary = saturated(sendStage, 27*time.Minute, true)

	r := e.report(t, time.Minute)

	assert.Equal(t, SuggestedIssueName, r.IssueName)
	assert.True(t, strings.HasPrefix(r.IssueID, SuggestedIssueID+":"), r.IssueID)
	assert.Equal(t, profilerec.ProfileHighConcurrency, decodeWire(t, r).Profile)
}

func TestRun_BriefSaturationReportsNothing(t *testing.T) {
	e := newReadyEnv(t)
	e.summary = saturated(sendStage, 5*time.Minute, true)

	reports, err := e.tick(time.Minute)

	require.ErrorIs(t, err, runnerdef.ErrStateUnknown, "an unhealthy pipeline holds any persisted issue open")
	assert.Empty(t, reports)
}

func TestRun_HighSupersedesLow(t *testing.T) {
	e := newReadyEnv(t)
	e.summary = saturated(sendStage, 27*time.Minute, true)
	low := e.report(t, time.Minute)
	require.Equal(t, SuggestedIssueName, low.IssueName)

	e.counters = profilerec.Counters{Missed: 3}
	high := e.report(t, time.Minute)

	assert.Equal(t, IssueName, high.IssueName)
	assert.NotEqual(t, low.IssueID, high.IssueID)
}

func TestRun_HighHeldDoesNotFlipToLow(t *testing.T) {
	e := newReadyEnv(t)
	e.summary = saturated(sendStage, 27*time.Minute, true)
	e.counters = profilerec.Counters{Missed: 3}
	require.Equal(t, IssueName, e.report(t, time.Minute).IssueName)

	r := e.report(t, profilerec.LossRecencyWindow+time.Minute)

	assert.Equal(t, IssueName, r.IssueName, "loss aged out but the held HIGH must not downgrade")
}

func TestRun_HeldUntilHealthyForMinPeriod(t *testing.T) {
	e := newReadyEnv(t)
	e.summary = saturated(sendStage, 27*time.Minute, true)
	first := e.report(t, time.Minute)

	e.summary = healthy()
	started := e.report(t, time.Minute)
	assert.Equal(t, first.IssueID, started.IssueID)
	assert.Equal(t, first.Context, started.Context)

	r := e.report(t, minHealthyFor-time.Minute)
	assert.Equal(t, first.IssueID, r.IssueID, "still inside the healthy period")

	reports, err := e.tick(time.Minute)
	require.NoError(t, err)
	assert.Empty(t, reports, "healthy for the full period resolves")

	reports, err = e.tick(time.Minute)
	require.NoError(t, err)
	assert.Empty(t, reports)
}

func TestRun_RecurrenceResetsHealthyPeriod(t *testing.T) {
	e := newReadyEnv(t)
	e.summary = saturated(sendStage, 27*time.Minute, true)
	e.report(t, time.Minute)

	e.summary = healthy()
	e.report(t, time.Minute)
	e.report(t, minHealthyFor-2*time.Minute)

	e.summary = saturated(sendStage, 27*time.Minute, true)
	e.report(t, time.Minute)

	e.summary = healthy()
	e.report(t, time.Minute)
	r := e.report(t, minHealthyFor-time.Minute)
	assert.Equal(t, SuggestedIssueName, r.IssueName, "the period restarts after a recurrence")

	reports, err := e.tick(time.Minute)
	require.NoError(t, err)
	assert.Empty(t, reports)
}

func TestRun_UnhealthyWarningKeepsIssueOpen(t *testing.T) {
	e := newReadyEnv(t)
	e.summary = saturated(sendStage, 27*time.Minute, true)
	e.report(t, time.Minute)

	e.summary = saturated(sendStage, 5*time.Minute, false)
	for range 3 {
		e.report(t, minHealthyFor)
	}
}

func TestRun_RestartWarmUpIsUnknown(t *testing.T) {
	e := newEnv(t, "logs_enabled: true")

	reports, err := e.tick(time.Minute)
	assert.ErrorIs(t, err, runnerdef.ErrStateUnknown)
	assert.Empty(t, reports)

	reports, err = e.tick(minHealthyFor)
	assert.NoError(t, err)
	assert.Empty(t, reports)
}

func TestRun_RestartStaysUnknownUntilHealthy(t *testing.T) {
	e := newEnv(t, "logs_enabled: true")
	e.summary = logsmetrics.BackpressureSummary{State: logsmetrics.BackpressureWarning}

	_, err := e.tick(time.Minute)
	assert.ErrorIs(t, err, runnerdef.ErrStateUnknown)
	_, err = e.tick(2 * minHealthyFor)
	assert.ErrorIs(t, err, runnerdef.ErrStateUnknown, "an unhealthy pipeline must not resolve a persisted issue")

	e.summary = healthy()
	_, err = e.tick(time.Minute)
	assert.ErrorIs(t, err, runnerdef.ErrStateUnknown)
	reports, err := e.tick(minHealthyFor)
	assert.NoError(t, err)
	assert.Empty(t, reports)
}

func TestRun_AppliedProfileStillLossyResolvesAfterVerifyWindow(t *testing.T) {
	e := newEnv(t, "logs_enabled: true")
	e.active = profilerec.ProfileHighConcurrency
	e.summary = saturated(sendStage, 27*time.Minute, true)

	_, err := e.tick(time.Minute)
	assert.ErrorIs(t, err, runnerdef.ErrStateUnknown)
	e.counters = profilerec.Counters{Missed: 5}
	_, err = e.tick(time.Minute)
	assert.ErrorIs(t, err, runnerdef.ErrStateUnknown, "a persisted issue is held while the deploy verifies")

	reports, err := e.tick(e.c.verifyWindow())
	assert.NoError(t, err, "nothing left to recommend once the verify window passes")
	assert.Empty(t, reports)
}

func TestRun_ConditionDuringWarmUpReports(t *testing.T) {
	e := newEnv(t, "logs_enabled: true")
	e.summary = saturated(sendStage, 27*time.Minute, true)

	assert.Equal(t, SuggestedIssueName, e.report(t, time.Second).IssueName)
}

func TestRun_ActiveProfileAlreadyAppliedReportsNothing(t *testing.T) {
	e := newReadyEnv(t)
	e.active = profilerec.ProfileHighConcurrency
	e.summary = saturated(sendStage, 27*time.Minute, true)
	e.counters = profilerec.Counters{Missed: 5}

	reports, err := e.tick(time.Minute)
	require.ErrorIs(t, err, runnerdef.ErrStateUnknown)
	assert.Empty(t, reports, "loss with the profile already active")

	e.counters = profilerec.Counters{}
	e.now = e.now.Add(profilerec.LossRecencyWindow + time.Minute)
	reports, err = e.tick(0)
	require.ErrorIs(t, err, runnerdef.ErrStateUnknown)
	assert.Empty(t, reports, "saturation with the profile already active")
}

func TestRun_PlanGate(t *testing.T) {
	tests := []struct {
		name    string
		plan    profilerec.Plan
		ok      bool
		reports int
	}{
		{name: "unknown profile", ok: false},
		{name: "nothing to change or block", ok: true},
		{
			name: "only blocked keys",
			ok:   true,
			plan: profilerec.Plan{Name: "high-concurrency", Version: 1, Blocked: []profilerec.PlanBlockedKey{{Key: "logs_config.batch_max_concurrent_send", Source: "file"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newReadyEnv(t)
			e.plan = func(string) (profilerec.Plan, bool) { return tt.plan, tt.ok }
			e.summary = saturated(sendStage, 27*time.Minute, true)
			e.counters = profilerec.Counters{Missed: 5}

			reports, err := e.tick(time.Minute)

			if tt.reports == 0 {
				require.ErrorIs(t, err, runnerdef.ErrStateUnknown)
			} else {
				require.NoError(t, err)
			}
			assert.Len(t, reports, tt.reports)
		})
	}
}

func TestRun_ProcessorBottleneckRecommendsHighThroughput(t *testing.T) {
	e := newReadyEnv(t)
	e.summary = saturated("processor", 20*time.Minute, true)
	e.counters = profilerec.Counters{Missed: 4096}

	w := decodeWire(t, e.report(t, time.Minute))

	assert.Equal(t, profilerec.ProfileHighThroughput, w.Profile)
	assert.Equal(t, profilerec.ReasonProcessorSaturated, w.ReasonCode)
	assert.True(t, w.MissedRecently)
}

func TestRun_ReportIsStableAndNotShared(t *testing.T) {
	e := newReadyEnv(t)
	e.summary = saturated(sendStage, 27*time.Minute, true)
	a := e.report(t, time.Minute)
	a.Context[contextKeyRecommendation] = "mutated"

	b := e.report(t, time.Minute)
	c := e.report(t, time.Minute)

	assert.NotEqual(t, "mutated", b.Context[contextKeyRecommendation])
	assert.Equal(t, b, c)
}

func TestRun_ListsAreCapped(t *testing.T) {
	plan, _ := changePlan(profilerec.ProfileHighConcurrency)
	for i := 0; i < 2*maxListed; i++ {
		plan.Changes = append(plan.Changes, plan.Changes[0])
		plan.Current = append(plan.Current, plan.Current[0])
	}
	e := newReadyEnv(t)
	e.plan = func(string) (profilerec.Plan, bool) { return plan, true }
	e.summary = saturated(sendStage, 27*time.Minute, true)
	e.counters.Missed = 1

	w := decodeWire(t, e.report(t, time.Minute))

	assert.Len(t, w.Changes, maxListed)
	assert.Len(t, w.Current, maxListed)
}
