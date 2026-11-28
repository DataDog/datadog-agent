// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package apmattachimpl

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

type fakeHelper struct {
	mu      sync.Mutex
	results map[int][]helperResult // consumed in order; the last one repeats
	calls   []int
}

func (f *fakeHelper) set(pid int, results ...helperResult) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results[pid] = results
}

func (f *fakeHelper) run(_ context.Context, pid int) helperResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, pid)
	rs := f.results[pid]
	if len(rs) == 0 {
		return helperResult{PID: pid, Decision: decisionSkipped, Reason: "not_java"}
	}
	r := rs[0]
	if len(rs) > 1 {
		f.results[pid] = rs[1:]
	}
	r.PID = pid
	return r
}

func (f *fakeHelper) takeCalls() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.calls
	f.calls = nil
	return c
}

type testEnv struct {
	a      *attacher
	proc   *fakeProc
	helper *fakeHelper
	policy string
	clock  time.Time
}

func newTestEnv(t *testing.T) *testEnv {
	dir := t.TempDir()
	injector := filepath.Join(dir, "attach")
	require.NoError(t, os.WriteFile(injector, []byte("#!/bin/sh\n"), 0o755))

	env := &testEnv{
		proc:   newFakeProc(t),
		helper: &fakeHelper{results: map[int][]helperResult{}},
		policy: filepath.Join(dir, "user-wls-policy.bin"),
		clock:  time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC),
	}
	env.a = &attacher{
		log:          logmock.New(t),
		enabled:      true,
		injectorPath: injector,
		scanInterval: time.Minute,
		timeout:      time.Second,
		procRoot:     env.proc.root,
		helper:       env.helper,
		policies:     newPolicyWatcher([]string{env.policy, filepath.Join(dir, "managed", "rc-orgwide-wls-policy.bin")}),
		now:          func() time.Time { return env.clock },
		states:       map[procKey]*processState{},
	}
	return env
}

func (e *testEnv) advance(d time.Duration) { e.clock = e.clock.Add(d) }

func (e *testEnv) java(pid int, startTime uint64) {
	e.proc.add(pid, fakeProcess{exe: "/usr/bin/java", comm: "java", startTime: startTime})
}

func attached() helperResult {
	return helperResult{Decision: decisionAttached, Reason: "ok", TracerVersion: "1.69.0"}
}

func denied() helperResult {
	return helperResult{Decision: decisionSkipped, Reason: "workload_selection_denied", Rule: "deny all", Layer: "user"}
}

func attachError() helperResult {
	return helperResult{Decision: decisionFailed, Reason: "attach_error", Detail: "handshake failed"}
}

func TestScanOnlyEvaluatesJavaAndNeverRetriesAttached(t *testing.T) {
	env := newTestEnv(t)
	env.java(100, 1)
	env.proc.add(200, fakeProcess{exe: "/usr/bin/python3", startTime: 2})
	env.helper.set(100, attached())

	env.a.scan(context.Background())
	assert.Equal(t, []int{100}, env.helper.takeCalls())

	env.advance(time.Hour)
	env.a.scan(context.Background())
	assert.Empty(t, env.helper.takeCalls(), "attached processes are final")

	require.NoError(t, os.WriteFile(env.policy, []byte("v1"), 0o644))
	require.True(t, env.a.checkPolicies())
	env.a.scan(context.Background())
	assert.Empty(t, env.helper.takeCalls(), "a policy change doesn't re-evaluate attached processes")
}

func TestSkippedReevaluatedOnlyOnPolicyChange(t *testing.T) {
	env := newTestEnv(t)
	env.java(100, 1)
	env.helper.set(100, denied(), attached())

	env.a.scan(context.Background())
	assert.Equal(t, []int{100}, env.helper.takeCalls())

	env.advance(time.Hour)
	env.a.scan(context.Background())
	assert.Empty(t, env.helper.takeCalls(), "skipped processes wait for a policy change")

	require.NoError(t, os.WriteFile(env.policy, []byte("allow"), 0o644))
	require.True(t, env.a.checkPolicies())
	assert.False(t, env.a.checkPolicies(), "the change is reported once")
	env.a.scan(context.Background())
	assert.Equal(t, []int{100}, env.helper.takeCalls())

	st := env.a.statusSnapshot()
	require.Len(t, st.Processes, 1)
	assert.Equal(t, decisionAttached, st.Processes[0].Decision)
	assert.NotEmpty(t, st.LastPolicyChange)

	require.NoError(t, os.WriteFile(env.policy, []byte("allow"), 0o644))
	later := env.clock.Add(time.Hour)
	require.NoError(t, os.Chtimes(env.policy, later, later))
	assert.False(t, env.a.checkPolicies(), "rewriting identical content is not a change")

	require.NoError(t, os.Remove(env.policy))
	assert.True(t, env.a.checkPolicies(), "removing a policy file is a change")
	managed := env.a.policies.paths[1]
	require.NoError(t, os.MkdirAll(filepath.Dir(managed), 0o755))
	require.NoError(t, os.WriteFile(managed, []byte("rc"), 0o644))
	assert.True(t, env.a.checkPolicies(), "creating a policy file in a new directory is a change")
}

func TestFailedRetriedWithBackoffUpToMaxAttempts(t *testing.T) {
	env := newTestEnv(t)
	env.java(100, 1)
	env.helper.set(100, attachError())

	env.a.scan(context.Background())
	assert.Equal(t, []int{100}, env.helper.takeCalls())

	env.advance(retryBaseBackoff - time.Second)
	env.a.scan(context.Background())
	assert.Empty(t, env.helper.takeCalls(), "no retry before the backoff expires")

	env.advance(time.Second)
	env.a.scan(context.Background())
	assert.Equal(t, []int{100}, env.helper.takeCalls())
	st := env.a.statusSnapshot()
	assert.Equal(t, 2, st.Processes[0].Failures)
	assert.NotEmpty(t, st.Processes[0].NextRetry)

	env.advance(4*retryBaseBackoff - time.Second)
	env.a.scan(context.Background())
	assert.Empty(t, env.helper.takeCalls(), "the second backoff is four times the first")
	env.advance(time.Second)
	env.a.scan(context.Background())
	assert.Equal(t, []int{100}, env.helper.takeCalls())

	env.advance(24 * time.Hour)
	require.NoError(t, os.WriteFile(env.policy, []byte("v2"), 0o644))
	require.True(t, env.a.checkPolicies())
	env.a.scan(context.Background())
	assert.Empty(t, env.helper.takeCalls(), "out of attempts, even after a policy change")
	st = env.a.statusSnapshot()
	assert.Equal(t, maxAttempts, st.Processes[0].Failures)
	assert.Empty(t, st.Processes[0].NextRetry)
}

func TestFailureThenSuccess(t *testing.T) {
	env := newTestEnv(t)
	env.java(100, 1)
	env.helper.set(100, attachError(), attached())

	env.a.scan(context.Background())
	env.advance(retryBaseBackoff)
	env.a.scan(context.Background())
	assert.Equal(t, []int{100, 100}, env.helper.takeCalls())
	st := env.a.statusSnapshot()
	assert.Equal(t, decisionAttached, st.Processes[0].Decision)
	assert.Equal(t, 0, st.Processes[0].Failures)
}

func TestPIDReuseAndExitedProcesses(t *testing.T) {
	env := newTestEnv(t)
	env.java(100, 1)
	env.java(101, 1)
	env.helper.set(100, attached())
	env.helper.set(101, attached())

	env.a.scan(context.Background())
	assert.ElementsMatch(t, []int{100, 101}, env.helper.takeCalls())

	env.proc.remove(101)
	env.proc.remove(100)
	env.java(100, 99)
	env.a.scan(context.Background())
	assert.Equal(t, []int{100}, env.helper.takeCalls())

	st := env.a.statusSnapshot()
	require.Len(t, st.Processes, 1)
	assert.Equal(t, uint64(99), st.Processes[0].StartTime)
}

func TestFailureOfExitedProcessIsDropped(t *testing.T) {
	env := newTestEnv(t)
	env.java(100, 1)
	env.helper.set(100, attachError())
	env.a.helper = helperFunc(func(ctx context.Context, pid int) helperResult {
		env.proc.remove(100)
		return env.helper.run(ctx, pid)
	})
	env.a.scan(context.Background())
	assert.Empty(t, env.a.statusSnapshot().Processes)
}

func TestResultOfReusedPIDIsDropped(t *testing.T) {
	env := newTestEnv(t)
	env.java(100, 1)
	env.helper.set(100, attached())
	env.a.helper = helperFunc(func(ctx context.Context, pid int) helperResult {
		env.proc.remove(100)
		env.java(100, 2)
		return env.helper.run(ctx, pid)
	})
	env.a.scan(context.Background())
	assert.Empty(t, env.a.statusSnapshot().Processes)
}

type helperFunc func(ctx context.Context, pid int) helperResult

func (f helperFunc) run(ctx context.Context, pid int) helperResult { return f(ctx, pid) }

func TestMissingInjector(t *testing.T) {
	env := newTestEnv(t)
	env.java(100, 1)
	require.NoError(t, os.Remove(env.a.injectorPath))

	env.a.scan(context.Background())
	assert.Empty(t, env.helper.takeCalls())
	st := env.a.statusSnapshot()
	assert.False(t, st.InjectorAvailable)
	assert.Contains(t, st.LastError, "attach helper not found")
	assert.NotEmpty(t, st.LastScan)

	require.NoError(t, os.WriteFile(env.a.injectorPath, []byte("#!/bin/sh\n"), 0o755))
	env.helper.set(100, attached())
	env.a.scan(context.Background())
	assert.Equal(t, []int{100}, env.helper.takeCalls())
	assert.Empty(t, env.a.statusSnapshot().LastError)
}

func TestScanStopsWhenCancelled(t *testing.T) {
	env := newTestEnv(t)
	env.java(100, 1)
	env.java(101, 1)
	ctx, cancel := context.WithCancel(context.Background())
	env.a.helper = helperFunc(func(_ context.Context, pid int) helperResult {
		cancel()
		return helperResult{PID: pid, Decision: decisionFailed, Reason: reasonHelperError}
	})
	env.a.scan(ctx)
	assert.Empty(t, env.a.statusSnapshot().Processes, "results of a cancelled run are not recorded")
}

func TestRetryBackoff(t *testing.T) {
	assert.Equal(t, time.Minute, retryBackoff(1))
	assert.Equal(t, 4*time.Minute, retryBackoff(2))
	assert.Equal(t, 16*time.Minute, retryBackoff(3))
}

func TestStatusOutput(t *testing.T) {
	env := newTestEnv(t)
	env.java(100, 1)
	env.java(200, 2)
	env.helper.set(100, helperResult{Decision: decisionAttached, Reason: "ok", Service: "checkout", TracerVersion: "1.69.0", DurationMs: 1056})
	env.helper.set(200, denied())
	env.a.scan(context.Background())

	p := statusProvider{a: env.a}
	assert.Equal(t, "APM Attach", p.Name())
	assert.Equal(t, "APM Attach", p.Section())

	var text bytes.Buffer
	require.NoError(t, p.Text(false, &text))
	out := text.String()
	assert.Contains(t, out, "Enabled: true")
	assert.Contains(t, out, "(available)")
	assert.Contains(t, out, "Last scan: 2026-11-18T12:00:00Z")
	assert.Contains(t, out, "Processes: 2")
	assert.Contains(t, out, `pid=100 decision=attached reason=ok rule="" layer="" service="checkout" tracer_version="1.69.0" failures=0`)
	assert.Contains(t, out, `pid=200 decision=skipped reason=workload_selection_denied rule="deny all" layer="user"`)

	var html bytes.Buffer
	require.NoError(t, p.HTML(false, &html))
	assert.Contains(t, html.String(), "PID 200")
	assert.Contains(t, html.String(), "deny all")

	stats := map[string]interface{}{}
	require.NoError(t, p.JSON(false, stats))
	data, err := json.Marshal(stats)
	require.NoError(t, err)
	var decoded struct {
		APMAttach attachStatus `json:"apmAttach"`
	}
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.True(t, decoded.APMAttach.Enabled)
	require.Len(t, decoded.APMAttach.Processes, 2)
	assert.Equal(t, "workload_selection_denied", decoded.APMAttach.Processes[1].Reason)
}

func TestStatusTextEscapesDetail(t *testing.T) {
	env := newTestEnv(t)
	env.java(100, 1)
	env.helper.set(100, helperResult{Decision: decisionFailed, Reason: "attach_error", Detail: "x\n    pid=1 decision=attached"})
	env.a.scan(context.Background())

	var text bytes.Buffer
	require.NoError(t, statusProvider{a: env.a}.Text(false, &text))
	assert.NotContains(t, text.String(), "\n    pid=1 ")
	assert.Contains(t, text.String(), `detail: "x\n    pid=1 decision=attached"`)
}

func TestComponentDisabled(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	provides, err := NewComponent(Requires{Lifecycle: lc, Log: logmock.New(t), Config: config.NewMock(t)})
	require.NoError(t, err)
	lc.AssertHooksNumber(0)
	assert.Nil(t, provides.Status.Provider)
}

func TestComponentEnabledConfig(t *testing.T) {
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		cfgEnabled:      true,
		cfgScanInterval: "15s",
		cfgInjectorPath: "/tmp/attach",
		cfgTimeout:      "60",
	})
	lc := compdef.NewTestLifecycle(t)
	provides, err := NewComponent(Requires{Lifecycle: lc, Log: logmock.New(t), Config: cfg})
	require.NoError(t, err)
	lc.AssertHooksNumber(1)
	require.NotNil(t, provides.Status.Provider)

	a := provides.Comp.(*attacher)
	assert.Equal(t, 15*time.Second, a.scanInterval)
	assert.Equal(t, defaultTimeout, a.timeout, "a duration without a unit is rejected")
	assert.Equal(t, "/tmp/attach", a.injectorPath)
}

func TestConfigDefaults(t *testing.T) {
	cfg := config.NewMock(t)
	assert.False(t, cfg.GetBool(cfgEnabled))
	assert.Equal(t, defaultScanInterval, cfg.GetDuration(cfgScanInterval))
	assert.Equal(t, defaultTimeout, cfg.GetDuration(cfgTimeout))
	assert.Equal(t, "/opt/datadog-packages/datadog-apm-inject/stable/inject/attach", cfg.GetString(cfgInjectorPath))
}

func TestStartStop(t *testing.T) {
	env := newTestEnv(t)
	env.java(100, 1)
	env.helper.set(100, attached())
	require.NoError(t, env.a.start(context.Background()))
	assert.Eventually(t, func() bool {
		return len(env.a.statusSnapshot().Processes) == 1
	}, 5*time.Second, 10*time.Millisecond, "the first scan runs at start")
	require.NoError(t, env.a.stop(context.Background()))
}
