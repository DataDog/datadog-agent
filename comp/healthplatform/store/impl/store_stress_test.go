// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package storeimpl

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/utils/selfident"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

const (
	// This is a caller-facing API contract, including time waiting for locks or
	// queue space. It must not be relaxed to accommodate a slow dependency.
	callerP99Budget = 100 * time.Millisecond
	burstDeadline   = 5 * time.Second
	stressIssueName = "Caller latency stress"
)

// dependencyGate stays wedged until cleanup explicitly releases it. Unlike a
// sleep, it cannot accidentally recover before the burst fills the event queue.
type dependencyGate struct {
	armed   atomic.Bool
	entered chan struct{}
	release chan struct{}
	enter   sync.Once
	open    sync.Once
}

func newDependencyGate() *dependencyGate {
	return &dependencyGate{entered: make(chan struct{}), release: make(chan struct{})}
}

func (g *dependencyGate) wait() {
	if g.armed.Load() {
		g.enter.Do(func() { close(g.entered) })
		<-g.release
	}
}

func (g *dependencyGate) unblock() { g.open.Do(func() { close(g.release) }) }

type stalledIdentity struct {
	deployment, cluster *dependencyGate
}

func (s *stalledIdentity) DeploymentID() string {
	s.deployment.wait()
	return ""
}
func (s *stalledIdentity) ClusterID() string {
	s.cluster.wait()
	return ""
}
func (s *stalledIdentity) IssueDiscriminator() string { return s.DeploymentID() }

// Deliberately ignore cancellation: the dependency itself is wedged, so the
// store must isolate callers even if a resolver fails to honor its context.
func (s *stalledIdentity) DeploymentIDWithContext(context.Context) string { return s.DeploymentID() }
func (s *stalledIdentity) ClusterIDWithContext(context.Context) string    { return s.ClusterID() }

var _ selfident.Resolver = (*stalledIdentity)(nil)

// faultDisk exercises the real disk implementation outside the injected fault.
type faultDisk struct {
	issuesPersistence
	gate  *dependencyGate
	fail  atomic.Bool
	calls atomic.Int64
}

func (d *faultDisk) save(state *PersistedState) error {
	d.calls.Add(1)
	d.gate.wait()
	if d.fail.Load() {
		return assert.AnError
	}
	return d.issuesPersistence.save(state)
}

type stressStore struct {
	h        *healthPlatformImpl
	tel      telemetry.Mock
	identity *stalledIdentity
	disk     *faultDisk
	callers  sync.WaitGroup
}

func newStressStore(t *testing.T, agentFlavor string) *stressStore {
	t.Helper()
	tel := telemetrymock.New(t)
	logger := logmock.New(t)
	lifecycle := compdef.NewTestLifecycle(t)
	provided, err := NewComponent(Requires{
		Lifecycle: lifecycle,
		Config: config.NewMockWithOverrides(t, map[string]interface{}{
			"health_platform.enabled": true,
			"run_path":                t.TempDir(),
		}),
		Log:       logger,
		Telemetry: tel,
		Hostname:  &mockHostname{name: "stress-host"},
	})
	require.NoError(t, err)
	f := &stressStore{
		h:        provided.Comp.(*healthPlatformImpl),
		tel:      tel,
		identity: &stalledIdentity{newDependencyGate(), newDependencyGate()},
		disk: &faultDisk{
			issuesPersistence: newDiskPersistence(filepath.Join(t.TempDir(), "issues.json"), logger),
			gate:              newDependencyGate(),
		},
	}
	f.h.selfIdent = f.identity
	f.h.persistence = f.disk
	f.h.agentFlavor = agentFlavor
	t.Cleanup(func() {
		// Release faults before joining callers or stopping the worker. Failed
		// assertions must not leak goroutines into the next subtest.
		f.unblock()
		done := make(chan struct{})
		go func() { f.callers.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(burstDeadline):
			t.Error("store callers did not exit after dependencies recovered")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), burstDeadline)
		defer cancel()
		require.NoError(t, lifecycle.Stop(ctx))
	})
	require.NoError(t, lifecycle.Start(context.Background()))
	return f
}

func (f *stressStore) unblock() {
	f.identity.deployment.unblock()
	f.identity.cluster.unblock()
	f.disk.gate.unblock()
}

func stressIssue(id string) *healthplatformpayload.Issue {
	return &healthplatformpayload.Issue{
		Id:        id,
		IssueName: stressIssueName,
		IssueType: "caller_latency_stress",
		Title:     "Caller latency stress",
		Source:    "stress-test",
		Severity:  healthplatformpayload.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
	}
}

// wedge waits for the fault to be reached by processing a public report. In
// particular, a disk scenario cannot pass without the persistence timer firing.
func (f *stressStore) wedge(t *testing.T, gate *dependencyGate) {
	t.Helper()
	gate.armed.Store(true)
	f.callers.Add(1)
	go func() {
		defer f.callers.Done()
		_ = f.h.ReportIssue(stressIssue("prime-fault"))
	}()
	select {
	case <-gate.entered:
	case <-time.After(35 * time.Second):
		t.Fatal("report processing did not reach the injected fault within 35s")
	}
}

type stressCall struct {
	name string
	call func(int)
}

type latencySample struct {
	method  int
	elapsed time.Duration
}

// burst bounds both concurrency and the entire run. Unfinished calls cannot be
// silently omitted from the percentile: every requested call must return.
func (f *stressStore) burst(t *testing.T, workers, perWorker int, methods ...stressCall) {
	t.Helper()
	start, cancel := make(chan struct{}), make(chan struct{})
	samples := make(chan latencySample, workers*perWorker)
	var ready, done sync.WaitGroup
	ready.Add(workers)
	done.Add(workers)
	for worker := range workers {
		f.callers.Add(1)
		go func() {
			defer f.callers.Done()
			defer done.Done()
			ready.Done()
			<-start
			for i := range perWorker {
				select {
				case <-cancel:
					return
				default:
				}
				id := worker*perWorker + i
				method := (worker + i) % len(methods)
				began := time.Now()
				methods[method].call(id)
				samples <- latencySample{method, time.Since(began)}
			}
		}()
	}
	ready.Wait()
	close(start)
	finished := make(chan struct{})
	go func() { done.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(burstDeadline):
		close(cancel)
		t.Fatalf("caller burst exceeded %s: %d/%d calls returned (p99 budget %s)",
			burstDeadline, len(samples), workers*perWorker, callerP99Budget)
	}
	close(samples)
	durations := make([][]time.Duration, len(methods))
	for sample := range samples {
		durations[sample.method] = append(durations[sample.method], sample.elapsed)
	}
	for method, values := range durations {
		require.NotEmpty(t, values, methods[method].name)
		slices.Sort(values)
		// Nearest-rank p99: ceil(0.99*n), converted to a zero-based index.
		p99 := values[(99*len(values)+99)/100-1]
		t.Logf("%s: calls=%d p99=%s max=%s", methods[method].name, len(values), p99, values[len(values)-1])
		assert.Less(t, p99, callerP99Budget, "%s caller p99", methods[method].name)
	}
}

func (f *stressStore) mutations(prefix string) []stressCall {
	return []stressCall{
		// Overflow may be signaled by an error or silently dropped. The
		// contract constrains caller latency and telemetry in either case.
		{"ReportIssue", func(i int) { _ = f.h.ReportIssue(stressIssue(fmt.Sprintf("%s-%d", prefix, i))) }},
		{"ResolveIssue", func(int) { f.h.ResolveIssue("seed") }},
	}
}

// Match the exported health_platform drop counter without coupling the contract
// to the async worker's private fields, event types, or counter label layout.
func (f *stressStore) drops(t *testing.T) float64 {
	t.Helper()
	families, err := f.tel.Gather(func(m *telemetry.MetricFamily) bool {
		return strings.HasPrefix(m.GetName(), "health_platform") && strings.Contains(m.GetName(), "drop")
	})
	require.NoError(t, err)
	var total float64
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			total += metric.GetCounter().GetValue()
		}
	}
	return total
}

func TestStoreCallerLatencyWithWedgedDependencies(t *testing.T) {
	for _, scenario := range []string{"deployment", "cluster", "slow-disk", "slow-failing-disk"} {
		t.Run(scenario, func(t *testing.T) {
			f := newStressStore(t, flavor.DefaultAgent)
			require.NoError(t, f.h.ReportIssue(stressIssue("seed")))
			require.Eventually(t, func() bool { return f.h.GetIssue("seed") != nil }, burstDeadline, time.Millisecond)
			gate := f.disk.gate
			switch scenario {
			case "deployment":
				gate = f.identity.deployment
			case "cluster":
				gate = f.identity.cluster
			case "slow-failing-disk":
				f.disk.fail.Store(true)
			}
			f.wedge(t, gate)
			before := f.drops(t)
			// A fixed 64 producers submit 16,384 events while the consumer is
			// wedged. Overflow must be counted instead of waiting for queue space.
			f.burst(t, 64, 256, f.mutations("fill")...)
			after := f.drops(t)
			require.Greater(t, after, before, "saturated event queue must count dropped issues")
			// With the queue now full and the consumer still wedged, every
			// subsequent report AND resolve must be accounted for as a drop.
			overflow := append(f.mutations("overflow"), stressCall{
				"ResolveAllIssues", func(int) { f.h.ResolveAllIssues() },
			})
			for _, mutation := range overflow {
				before = f.drops(t)
				f.burst(t, 32, 32, mutation)
				assert.Equal(t, float64(32*32), f.drops(t)-before, "%s overflow accounting", mutation.name)
			}
		})
	}
}

func TestStoreExportedMethodsCallerLatency(t *testing.T) {
	for _, dependency := range []string{"deployment", "cluster", "disk"} {
		for _, agentFlavor := range []string{flavor.DefaultAgent, flavor.ClusterAgent} {
			t.Run(dependency+"/"+agentFlavor, func(t *testing.T) {
				f := newStressStore(t, agentFlavor)
				require.NoError(t, f.h.ReportIssue(stressIssue("seed")))
				require.Eventually(t, func() bool { return f.h.GetIssue("seed") != nil }, burstDeadline, time.Millisecond)
				gate := f.disk.gate
				if dependency == "deployment" {
					gate = f.identity.deployment
				} else if dependency == "cluster" {
					gate = f.identity.cluster
				}
				f.wedge(t, gate)
				// Keep methods in separate subtests so one blocking API cannot
				// hide failures in another method's percentile distribution.
				methods := append(f.mutations("api"), []stressCall{
					{"ResolveAllIssues", func(int) { f.h.ResolveAllIssues() }},
					{"GetAllIssues", func(int) { f.h.GetAllIssues() }},
					{"GetIssue", func(int) { f.h.GetIssue("seed") }},
					{"GetActiveIssueIDsByIssueName", func(int) { f.h.GetActiveIssueIDsByIssueName(stressIssueName) }},
					{"RegisterIssuesObserver", func(int) { f.h.RegisterIssuesObserver(storedef.IssuesObserver{}) }},
					{"IssueDiscriminator", func(int) { f.h.IssueDiscriminator("stress-host") }},
					{"ResourceIdentity", func(int) { f.h.ResourceIdentity("stress-host") }},
					{"ResourceID", func(int) { f.h.ResourceID() }},
				}...)
				// Adding an exported API requires adding its latency case too.
				api := reflect.TypeFor[storedef.Component]()
				require.Len(t, methods, api.NumMethod())
				for i := range api.NumMethod() {
					name := api.Method(i).Name
					require.True(t, slices.ContainsFunc(methods, func(c stressCall) bool { return c.name == name }),
						"missing caller-latency coverage for %s", name)
				}
				for _, method := range methods {
					t.Run(method.name, func(t *testing.T) { f.burst(t, 16, 8, method) })
				}
			})
		}
	}
}

func TestStoreCallerLatencyWithFailingDisk(t *testing.T) {
	f := newStressStore(t, flavor.DefaultAgent)
	f.disk.fail.Store(true)
	before := f.disk.calls.Load()
	f.burst(t, 64, 64, f.mutations("failing-disk")...)
	require.Eventually(t, func() bool { return f.disk.calls.Load() > before }, 35*time.Second, 10*time.Millisecond,
		"the persistence failure must actually be exercised")
}
