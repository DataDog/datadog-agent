// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package intakeroutingimpl

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	intakerouting "github.com/DataDog/datadog-agent/comp/intakerouting/def"
)

const (
	defaultHost       = "agent-http-intake.logs.datadoghq.com"
	enhancedHost      = "acme.agent-http-intake.logs.datadoghq.com"
	otherEnhancedHost = "shard2.agent-http-intake.logs.datadoghq.com"
	testKey           = "0123456789abcdef0123456789abcdef"
	otherKey          = "fedcba9876543210fedcba9876543210"

	testGrace   = time.Second
	testRefresh = time.Hour
	// testKeyRecheck is longer than testRefresh, so only the tests that ask for a shorter recheck
	// get passes that do nothing but read the API keys.
	testKeyRecheck = 2 * testRefresh
)

var (
	defaultEndpoint  = intakerouting.Endpoint{Host: defaultHost}
	enhancedEndpoint = intakerouting.Endpoint{Host: enhancedHost}
	otherEnhanced    = intakerouting.Endpoint{Host: otherEnhancedHost}
)

type queryCall struct {
	site   string
	apiKey string
	at     time.Time
}

type preflightCall struct {
	endpoint intakerouting.Endpoint
	apiKey   string
}

// queryTarget is the site and API key a query is sent with.
type queryTarget struct {
	site   string
	apiKey string
}

// fakeClient is a routingClient whose answers are set by the test.
type fakeClient struct {
	clock clock.Clock

	mu     sync.Mutex
	result queryResult
	// resultFor, when it has an entry for a query's site and key, answers it instead of result.
	resultFor    map[queryTarget]queryResult
	preflightErr map[string]error
	// gate, when set, holds queries until it is closed or their context is done.
	gate chan struct{}
	// inFlight receives a value when a query starts waiting on gate.
	inFlight   chan struct{}
	queries    []queryCall
	preflights []preflightCall
	ctxErrs    []error
}

func (c *fakeClient) query(ctx context.Context, site, apiKey string) queryResult {
	c.mu.Lock()
	c.queries = append(c.queries, queryCall{site: site, apiKey: apiKey, at: c.clock.Now()})
	gate, inFlight := c.gate, c.inFlight
	c.mu.Unlock()

	if gate != nil {
		if inFlight != nil {
			inFlight <- struct{}{}
		}
		select {
		case <-gate:
		case <-ctx.Done():
			c.mu.Lock()
			c.ctxErrs = append(c.ctxErrs, ctx.Err())
			c.mu.Unlock()
			return queryResult{status: statusTimeout, err: ctx.Err()}
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if res, ok := c.resultFor[queryTarget{site: site, apiKey: apiKey}]; ok {
		return res
	}
	return c.result
}

func (c *fakeClient) preflight(_ context.Context, ep intakerouting.Endpoint, apiKey string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.preflights = append(c.preflights, preflightCall{endpoint: ep, apiKey: apiKey})
	return c.preflightErr[ep.Host]
}

func (c *fakeClient) setResult(res queryResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.result = res
}

// setResultFor sets the answer to the queries sent to site with apiKey.
func (c *fakeClient) setResultFor(site, apiKey string, res queryResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resultFor == nil {
		c.resultFor = make(map[queryTarget]queryResult)
	}
	c.resultFor[queryTarget{site: site, apiKey: apiKey}] = res
}

func (c *fakeClient) failPreflight(host string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.preflightErr == nil {
		c.preflightErr = make(map[string]error)
	}
	c.preflightErr[host] = err
}

func (c *fakeClient) queryCalls() []queryCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.queries)
}

func (c *fakeClient) queryTargets() []queryTarget {
	c.mu.Lock()
	defer c.mu.Unlock()
	targets := make([]queryTarget, len(c.queries))
	for i, q := range c.queries {
		targets[i] = queryTarget{site: q.site, apiKey: q.apiKey}
	}
	return targets
}

func (c *fakeClient) preflightCalls() []preflightCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.preflights)
}

// fakePaths is a pathChecker whose answers are set by the test. Hosts are reached directly unless
// set otherwise.
type fakePaths struct {
	mu      sync.Mutex
	reasons map[string]string
	checks  []intakerouting.Endpoint
}

func (p *fakePaths) check(_ context.Context, ep intakerouting.Endpoint) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.checks = append(p.checks, ep)
	if reason := p.reasons[ep.Host]; reason != "" {
		return reason, errors.New("not reached directly")
	}
	return "", nil
}

// set makes host reached the way reason says ("" means directly).
func (p *fakePaths) set(host, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.reasons == nil {
		p.reasons = make(map[string]string)
	}
	p.reasons[host] = reason
}

func (p *fakePaths) checkCalls() []intakerouting.Endpoint {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.checks)
}

func overrideResult(host string, port int) queryResult {
	return queryResult{status: statusSuccess, code: 200, routes: map[intakerouting.TelemetryType]routeAnswer{
		intakerouting.Logs: {override: true, host: host, port: port},
	}}
}

func noOverrideResult() queryResult {
	return queryResult{status: statusSuccess, code: 200, routes: map[intakerouting.TelemetryType]routeAnswer{
		intakerouting.Logs: {override: false},
	}}
}

func notOfferedResult() queryResult {
	return queryResult{status: statusNotOffered, code: 404}
}

func failedResult(status string) queryResult {
	return queryResult{status: status, err: errors.New("query failed")}
}

func logsRequest(host, apiKey string) intakerouting.Request {
	return intakerouting.Request{
		Type:    intakerouting.Logs,
		Default: intakerouting.Endpoint{Host: host},
		APIKey:  func() (string, bool) { return apiKey, true },
	}
}

type fixture struct {
	t      *testing.T
	r      *router
	clock  *clock.Mock
	client *fakeClient
	paths  *fakePaths
	tel    telemetry.Mock
	idle   chan struct{}
}

func newFixture(t *testing.T, opts ...func(*routerOptions)) *fixture {
	clk := clock.NewMock()
	client := &fakeClient{clock: clk, result: notOfferedResult()}
	paths := &fakePaths{}
	tel := telemetrymock.New(t)
	ropts := routerOptions{
		log:        logmock.New(t),
		client:     client,
		paths:      paths,
		clock:      clk,
		tlm:        newRoutingTelemetry(tel),
		grace:      testGrace,
		refresh:    testRefresh,
		keyRecheck: testKeyRecheck,
		jitter:     func(d time.Duration) time.Duration { return d },
	}
	for _, opt := range opts {
		opt(&ropts)
	}
	r := newRouter(ropts)
	idle := make(chan struct{}, 64)
	r.afterPass = func() {
		// Only report idleness once no pass is requested, so settle matches the last action.
		if len(r.wake) == 0 {
			idle <- struct{}{}
		}
	}
	t.Cleanup(func() { require.NoError(t, r.stop(context.Background())) })
	return &fixture{t: t, r: r, clock: clk, client: client, paths: paths, tel: tel, idle: idle}
}

// settle waits for the loop to go idle after the last action.
func (f *fixture) settle() {
	f.t.Helper()
	select {
	case <-f.idle:
	case <-time.After(10 * time.Second):
		f.t.Fatal("the routing loop did not go idle")
	}
}

// advance moves the clock forward and waits for the pass it triggers.
func (f *fixture) advance(d time.Duration) {
	f.t.Helper()
	f.clock.Add(d)
	f.settle()
}

func (f *fixture) register(reqs ...intakerouting.Request) intakerouting.Registration {
	f.t.Helper()
	reg := f.r.Register(reqs)
	f.settle()
	return reg
}

func (f *fixture) counter(name string) map[string]float64 {
	f.t.Helper()
	metrics, err := f.tel.GetCountMetric(telemetrySubsystem, name)
	if err != nil {
		return nil
	}
	values := make(map[string]float64)
	for _, m := range metrics {
		tags := m.Tags()
		key := ""
		for _, tag := range []string{"status", "telemetry_type", "direction", "reason"} {
			if v, ok := tags[tag]; ok {
				if key != "" {
					key += ","
				}
				key += v
			}
		}
		values[key] = m.Value()
	}
	return values
}

func (f *fixture) activeOverrides() float64 {
	f.t.Helper()
	metrics, err := f.tel.GetGaugeMetric(telemetrySubsystem, "active_overrides")
	require.NoError(f.t, err)
	for _, m := range metrics {
		if m.Tags()["telemetry_type"] == string(intakerouting.Logs) {
			return m.Value()
		}
	}
	return 0
}

func (f *fixture) nextAttempt(apiKey string) time.Duration {
	f.t.Helper()
	f.r.mu.Lock()
	defer f.r.mu.Unlock()
	g, ok := f.r.groups[groupKey{site: "datadoghq.com", apiKey: apiKey}]
	require.True(f.t, ok, "no group for this key")
	return g.nextAttempt.Sub(f.clock.Now())
}

func assertNotified(t *testing.T, reg intakerouting.Registration) {
	t.Helper()
	select {
	case <-reg.Changes():
	default:
		t.Fatal("expected a change notification")
	}
}

func assertNotNotified(t *testing.T, reg intakerouting.Registration) {
	t.Helper()
	select {
	case <-reg.Changes():
		t.Fatal("unexpected change notification")
	default:
	}
}

// TestTelemetryNames pins the metric names and labels. Agent telemetry ships them through the
// intake-routing profile of comp/core/agenttelemetry/impl/defaultProfiles.yaml, whose test lists
// the same names.
func TestTelemetryNames(t *testing.T) {
	tel := telemetrymock.New(t)
	tlm := newRoutingTelemetry(tel)
	tlm.queries.Inc(statusSuccess)
	tlm.preflights.Inc(preflightSuccess)
	tlm.switches.Inc(string(intakerouting.Logs), directionToEnhanced)
	tlm.rejectedAnswers.Inc(string(intakerouting.Logs), rejectInvalidHost)
	tlm.skippedEndpoints.Inc(string(intakerouting.Logs), pathPrivateAddress)
	tlm.activeOverrides.Set(1, string(intakerouting.Logs))

	families, err := tel.GetRegistry().Gather()
	require.NoError(t, err)
	labels := make(map[string][]string)
	for _, family := range families {
		for _, label := range family.GetMetric()[0].GetLabel() {
			labels[family.GetName()] = append(labels[family.GetName()], label.GetName())
		}
	}
	assert.Equal(t, map[string][]string{
		"intake_routing__queries":           {"status"},
		"intake_routing__preflights":        {"status"},
		"intake_routing__switches":          {"direction", "telemetry_type"},
		"intake_routing__rejected_answers":  {"reason", "telemetry_type"},
		"intake_routing__skipped_endpoints": {"reason", "telemetry_type"},
		"intake_routing__active_overrides":  {"telemetry_type"},
	}, labels)
}

func TestDisabled(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	provides := NewComponent(Requires{
		Lc:        lc,
		Config:    config.NewMock(t),
		Log:       logmock.New(t),
		Telemetry: telemetrymock.New(t),
	})
	lc.AssertHooksNumber(0)

	reg := provides.Comp.Register([]intakerouting.Request{logsRequest(defaultHost, testKey)})
	assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, reg.WaitInitial(context.Background()))
	assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, reg.Current())
	assertNotNotified(t, reg)
	reg.Close()
	reg.Close()
}

func TestEnabledRegistersStopHook(t *testing.T) {
	lc := compdef.NewTestLifecycle(t)
	provides := NewComponent(Requires{
		Lc:        lc,
		Config:    config.NewMockWithOverrides(t, map[string]interface{}{"intake_routing.enabled": true}),
		Log:       logmock.New(t),
		Telemetry: telemetrymock.New(t),
	})
	lc.AssertHooksNumber(1)
	assert.IsType(t, &router{}, provides.Comp)
	require.NoError(t, lc.Stop(context.Background()))
}

func TestLoadSettings(t *testing.T) {
	tests := []struct {
		name            string
		overrides       map[string]interface{}
		expectedGrace   time.Duration
		expectedRefresh time.Duration
	}{
		{"defaults", nil, time.Second, 6 * time.Hour},
		{"custom", map[string]interface{}{startupGraceKey: "3s", refreshKey: "10m"}, 3 * time.Second, 10 * time.Minute},
		{"grace too long", map[string]interface{}{startupGraceKey: "45s"}, 30 * time.Second, 6 * time.Hour},
		{"negative grace", map[string]interface{}{startupGraceKey: "-1s"}, 0, 6 * time.Hour},
		{"refresh too short", map[string]interface{}{refreshKey: "10s"}, time.Second, time.Minute},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			grace, refresh := loadSettings(config.NewMockWithOverrides(t, tc.overrides), logmock.New(t))
			assert.Equal(t, tc.expectedGrace, grace)
			assert.Equal(t, tc.expectedRefresh, refresh)
		})
	}
}

func TestAnswerWithinGrace(t *testing.T) {
	f := newFixture(t)
	f.client.setResult(overrideResult(enhancedHost, 443))

	reg := f.register(logsRequest(defaultHost, testKey))

	// The decision was made before the grace period elapsed: no need to wait for it.
	assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint}, reg.WaitInitial(context.Background()))
	assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint}, reg.Current())

	queries := f.client.queryCalls()
	require.Len(t, queries, 1)
	assert.Equal(t, "datadoghq.com", queries[0].site)
	assert.Equal(t, testKey, queries[0].apiKey)
	assert.Equal(t, []preflightCall{{endpoint: enhancedEndpoint, apiKey: testKey}}, f.client.preflightCalls())

	assert.Equal(t, map[string]float64{statusSuccess: 1}, f.counter("queries"))
	assert.Equal(t, map[string]float64{preflightSuccess: 1}, f.counter("preflights"))
	assert.Equal(t, map[string]float64{"logs," + directionToEnhanced: 1}, f.counter("switches"))
	assert.Equal(t, 1.0, f.activeOverrides())
}

func TestAnswerAfterGrace(t *testing.T) {
	f := newFixture(t)
	f.client.setResult(overrideResult(enhancedHost, 0))
	f.client.gate = make(chan struct{})

	reg := f.r.Register([]intakerouting.Request{logsRequest(defaultHost, testKey)})

	// The query is held: once the grace period elapses, the default endpoint is used.
	f.clock.Add(testGrace)
	assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, reg.WaitInitial(context.Background()))
	assertNotNotified(t, reg)

	close(f.client.gate)
	f.settle()
	assertNotified(t, reg)
	assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint}, reg.Current())
}

func TestWaitInitialHonorsContext(t *testing.T) {
	f := newFixture(t)
	f.client.gate = make(chan struct{})
	defer close(f.client.gate)

	reg := f.r.Register([]intakerouting.Request{logsRequest(defaultHost, testKey)})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, reg.WaitInitial(ctx))
}

func TestStartupBackoff(t *testing.T) {
	f := newFixture(t)
	f.client.setResult(failedResult(statusNetworkError))

	reg := f.register(logsRequest(defaultHost, testKey))
	// A failed query decides the route for WaitInitial: it keeps its default.
	assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, reg.WaitInitial(context.Background()))

	expected := []time.Duration{1, 2, 4, 8, 16, 32, 64, 128, 256, 300, 300}
	for i, seconds := range expected {
		delay := seconds * time.Second
		assert.Equal(t, delay, f.nextAttempt(testKey), "retry %d", i+1)
		f.advance(delay)
		assert.Len(t, f.client.queryCalls(), i+2)
	}
	assert.Equal(t, map[string]float64{statusNetworkError: float64(len(expected) + 1)}, f.counter("queries"))

	// The first answer switches the endpoint, then queries follow the refresh cadence.
	f.client.setResult(overrideResult(enhancedHost, 0))
	f.advance(300 * time.Second)
	assertNotified(t, reg)
	assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint}, reg.Current())
	assert.Equal(t, testRefresh, f.nextAttempt(testKey))
}

func TestRefreshCadence(t *testing.T) {
	f := newFixture(t)
	f.client.setResult(noOverrideResult())

	f.register(logsRequest(defaultHost, testKey))
	require.Len(t, f.client.queryCalls(), 1)

	f.clock.Add(testRefresh - time.Minute)
	f.advance(time.Minute)
	queries := f.client.queryCalls()
	require.Len(t, queries, 2)
	assert.Equal(t, testRefresh, queries[1].at.Sub(queries[0].at))
}

func TestFailureAfterAnswerKeepsCurrentAndWaitsForRefresh(t *testing.T) {
	f := newFixture(t)
	f.client.setResult(overrideResult(enhancedHost, 0))
	reg := f.register(logsRequest(defaultHost, testKey))
	assertNotified(t, reg)

	for _, status := range []string{statusNetworkError, statusTimeout, statusHTTPError, statusInvalidResponse} {
		f.client.setResult(failedResult(status))
		f.advance(testRefresh)
		assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint}, reg.Current(), status)
		assertNotNotified(t, reg)
		// No fast retry once the group got an answer.
		assert.Equal(t, testRefresh, f.nextAttempt(testKey), status)
	}
	assert.Equal(t, 1.0, f.activeOverrides())
}

func TestNoOverrideRevertsWithoutPreflight(t *testing.T) {
	f := newFixture(t)
	f.client.setResult(overrideResult(enhancedHost, 0))
	reg := f.register(logsRequest(defaultHost, testKey))
	assertNotified(t, reg)
	require.Len(t, f.client.preflightCalls(), 1)

	f.client.setResult(noOverrideResult())
	f.advance(testRefresh)
	assertNotified(t, reg)
	assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, reg.Current())
	assert.Len(t, f.client.preflightCalls(), 1)
	assert.Equal(t, map[string]float64{"logs," + directionToEnhanced: 1, "logs," + directionToDefault: 1}, f.counter("switches"))
	assert.Equal(t, 0.0, f.activeOverrides())
}

func TestOverrideToDefaultHostReverts(t *testing.T) {
	f := newFixture(t)
	f.client.setResult(overrideResult(enhancedHost, 0))
	reg := f.register(logsRequest(defaultHost, testKey))
	assertNotified(t, reg)

	f.client.setResult(overrideResult(defaultHost+".", 443))
	f.advance(testRefresh)
	assertNotified(t, reg)
	assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, reg.Current())
	assert.Len(t, f.client.preflightCalls(), 1)
}

func TestPreflight(t *testing.T) {
	t.Run("failure keeps the default", func(t *testing.T) {
		f := newFixture(t)
		f.client.setResult(overrideResult(enhancedHost, 0))
		f.client.failPreflight(enhancedHost, errors.New("HTTP 503"))

		reg := f.register(logsRequest(defaultHost, testKey))
		assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, reg.WaitInitial(context.Background()))
		assertNotNotified(t, reg)
		assert.Equal(t, map[string]float64{preflightFailure: 1}, f.counter("preflights"))
		assert.Empty(t, f.counter("switches"))
		// A failed pre-flight check still settles the query: no fast retry.
		assert.Equal(t, testRefresh, f.nextAttempt(testKey))
	})

	t.Run("enhanced to enhanced", func(t *testing.T) {
		f := newFixture(t)
		f.client.setResult(overrideResult(enhancedHost, 0))
		reg := f.register(logsRequest(defaultHost, testKey))
		assertNotified(t, reg)

		// The same answer again is a no-op: no new pre-flight check.
		f.advance(testRefresh)
		assertNotNotified(t, reg)
		assert.Len(t, f.client.preflightCalls(), 1)

		// A failed pre-flight check of another host keeps the current override.
		f.client.setResult(overrideResult(otherEnhancedHost, 0))
		f.client.failPreflight(otherEnhancedHost, errors.New("connection refused"))
		f.advance(testRefresh)
		assertNotNotified(t, reg)
		assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint}, reg.Current())

		f.client.failPreflight(otherEnhancedHost, nil)
		f.advance(testRefresh)
		assertNotified(t, reg)
		assert.Equal(t, []intakerouting.Endpoint{otherEnhanced}, reg.Current())
		assert.Equal(t, map[string]float64{"logs," + directionToEnhanced: 1, "logs," + directionEnhancedToEnhanced: 1}, f.counter("switches"))
		assert.Equal(t, 1.0, f.activeOverrides())
	})

	t.Run("custom port", func(t *testing.T) {
		f := newFixture(t)
		f.client.setResult(overrideResult(enhancedHost, 8443))
		reg := f.register(logsRequest(defaultHost, testKey))
		target := intakerouting.Endpoint{Host: enhancedHost, Port: 8443}
		assert.Equal(t, []intakerouting.Endpoint{target}, reg.Current())
		assert.Equal(t, []preflightCall{{endpoint: target, apiKey: testKey}}, f.client.preflightCalls())
	})
}

func TestInvalidHostsAreRejected(t *testing.T) {
	for _, host := range []string{
		"",
		"10.0.0.1",
		"[::1]",
		"evil.example.com",
		"datadoghq.com",
		"us3.datadoghq.com",
		"acme.agent-http-intake.logs.datadoghq.com/path",
		"user@acme.agent-http-intake.logs.datadoghq.com",
		"acme.agent-http-intake.logs.datadoghq.com:8443",
		"acme..datadoghq.com",
		"datadoghq.com.evil.example",
	} {
		t.Run(host, func(t *testing.T) {
			f := newFixture(t)
			f.client.setResult(overrideResult(host, 0))
			reg := f.register(logsRequest(defaultHost, testKey))

			assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, reg.Current())
			assert.Empty(t, f.client.preflightCalls())
			assert.Equal(t, map[string]float64{"logs," + rejectInvalidHost: 1}, f.counter("rejected_answers"))
			// An unusable answer still ends the fast retry.
			assert.Equal(t, testRefresh, f.nextAttempt(testKey))
		})
	}

	t.Run("invalid port", func(t *testing.T) {
		f := newFixture(t)
		f.client.setResult(overrideResult(enhancedHost, 70000))
		reg := f.register(logsRequest(defaultHost, testKey))
		assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, reg.Current())
		assert.Equal(t, map[string]float64{"logs," + rejectInvalidHost: 1}, f.counter("rejected_answers"))
	})

	t.Run("invalid host keeps the current override", func(t *testing.T) {
		f := newFixture(t)
		f.client.setResult(overrideResult(enhancedHost, 0))
		reg := f.register(logsRequest(defaultHost, testKey))
		assertNotified(t, reg)

		f.client.setResult(overrideResult("evil.example.com", 0))
		f.advance(testRefresh)
		assertNotNotified(t, reg)
		assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint}, reg.Current())
	})
}

func TestAnswerEntriesOfOtherTypesAreIgnored(t *testing.T) {
	f := newFixture(t)
	// The answer holds every type the backend knows of, not only the types that are registered.
	f.client.setResult(queryResult{status: statusSuccess, code: 200, routes: map[intakerouting.TelemetryType]routeAnswer{
		intakerouting.Logs: {override: true, host: enhancedHost},
		"metrics":          {override: true, host: otherEnhancedHost},
		"traces":           {override: true, host: "evil.example.com"},
	}})

	reg := f.register(logsRequest(defaultHost, testKey))
	assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint}, reg.Current())
	// Only the logs override is checked, and the entries of other types are not validated.
	assert.Equal(t, []preflightCall{{endpoint: enhancedEndpoint, apiKey: testKey}}, f.client.preflightCalls())
	assert.Empty(t, f.counter("rejected_answers"))
	assert.Equal(t, map[string]float64{"logs," + directionToEnhanced: 1}, f.counter("switches"))
}

func TestSettledAnswersEndFastRetry(t *testing.T) {
	tests := map[string]queryResult{
		"not offered": notOfferedResult(),
		"absent":      {status: statusSuccess, routes: map[intakerouting.TelemetryType]routeAnswer{"metrics": {override: true, host: enhancedHost}}},
	}
	for name, res := range tests {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			f.client.setResult(res)
			reg := f.register(logsRequest(defaultHost, testKey))
			assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, reg.Current())
			assert.Equal(t, testRefresh, f.nextAttempt(testKey))
		})
	}

	t.Run("absent keeps the current override", func(t *testing.T) {
		f := newFixture(t)
		f.client.setResult(overrideResult(enhancedHost, 0))
		reg := f.register(logsRequest(defaultHost, testKey))
		assertNotified(t, reg)

		f.client.setResult(queryResult{status: statusSuccess, routes: map[intakerouting.TelemetryType]routeAnswer{}})
		f.advance(testRefresh)
		assertNotNotified(t, reg)
		assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint}, reg.Current())
		assert.Equal(t, map[string]float64{"logs," + rejectAbsent: 1}, f.counter("rejected_answers"))
	})

	t.Run("not offered keeps the current override", func(t *testing.T) {
		f := newFixture(t)
		f.client.setResult(overrideResult(enhancedHost, 0))
		reg := f.register(logsRequest(defaultHost, testKey))
		assertNotified(t, reg)

		f.client.setResult(notOfferedResult())
		f.advance(testRefresh)
		assertNotNotified(t, reg)
		assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint}, reg.Current())
		assert.Equal(t, map[string]float64{statusSuccess: 1, statusNotOffered: 1}, f.counter("queries"))
	})
}

func TestDeduplication(t *testing.T) {
	f := newFixture(t)
	f.client.setResult(overrideResult(enhancedHost, 0))

	// The main endpoint and an additional endpoint sharing its key need one query and one
	// pre-flight check. An endpoint with another key needs its own.
	reg := f.register(
		logsRequest(defaultHost, testKey),
		logsRequest(defaultHost, testKey),
		logsRequest(defaultHost, otherKey),
	)
	assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint, enhancedEndpoint, enhancedEndpoint}, reg.Current())
	assert.Len(t, f.client.queryCalls(), 2)
	assert.ElementsMatch(t, []preflightCall{
		{endpoint: enhancedEndpoint, apiKey: testKey},
		{endpoint: enhancedEndpoint, apiKey: otherKey},
	}, f.client.preflightCalls())
	assert.Equal(t, 3.0, f.activeOverrides())

	// A later registration joining a group gets its cached answer without a new query.
	late := f.register(logsRequest(defaultHost, testKey))
	assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint}, late.WaitInitial(context.Background()))
	assertNotified(t, late)
	assert.Len(t, f.client.queryCalls(), 2)
	assert.Equal(t, 4.0, f.activeOverrides())

	// Both registrations are refreshed by one query.
	f.client.setResult(noOverrideResult())
	f.advance(testRefresh)
	assert.Len(t, f.client.queryCalls(), 4)
	assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, late.Current())
	assert.Equal(t, 0.0, f.activeOverrides())
}

func TestEachSiteAndKeyQueriedSeparately(t *testing.T) {
	const (
		euHost         = "agent-http-intake.logs.datadoghq.eu"
		euEnhancedHost = "globex.agent-http-intake.logs.datadoghq.eu"
		euKey          = "00112233445566778899aabbccddeeff"
	)
	euDefault := intakerouting.Endpoint{Host: euHost}
	euEnhanced := intakerouting.Endpoint{Host: euEnhancedHost}

	f := newFixture(t)
	// Each organization gets its own answer, from the site of its endpoint.
	f.client.setResultFor("datadoghq.com", testKey, overrideResult(enhancedHost, 0))
	f.client.setResultFor("datadoghq.com", otherKey, noOverrideResult())
	f.client.setResultFor("datadoghq.eu", euKey, overrideResult(euEnhancedHost, 0))
	f.client.setResultFor("datadoghq.eu", testKey, notOfferedResult())

	// A main endpoint and additional endpoints: another organization on the same site, an
	// organization on another site, and the main key on another site.
	reg := f.register(
		logsRequest(defaultHost, testKey),
		logsRequest(defaultHost, otherKey),
		logsRequest(euHost, euKey),
		logsRequest(euHost, testKey),
	)
	assert.ElementsMatch(t, []queryTarget{
		{site: "datadoghq.com", apiKey: testKey},
		{site: "datadoghq.com", apiKey: otherKey},
		{site: "datadoghq.eu", apiKey: euKey},
		{site: "datadoghq.eu", apiKey: testKey},
	}, f.client.queryTargets())
	assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint, defaultEndpoint, euEnhanced, euDefault}, reg.Current())
	// Each new host is checked with the key of the organization routed to it.
	assert.ElementsMatch(t, []preflightCall{
		{endpoint: enhancedEndpoint, apiKey: testKey},
		{endpoint: euEnhanced, apiKey: euKey},
	}, f.client.preflightCalls())
	assert.Equal(t, 2.0, f.activeOverrides())

	// Every group is refreshed, and a new answer only changes the routes of its own group.
	f.client.setResultFor("datadoghq.eu", euKey, noOverrideResult())
	f.advance(testRefresh)
	assert.Len(t, f.client.queryCalls(), 8)
	assertNotified(t, reg)
	assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint, defaultEndpoint, euDefault, euDefault}, reg.Current())
	assert.Equal(t, 1.0, f.activeOverrides())
}

func TestIndirectDefaultIsNotQueried(t *testing.T) {
	for _, reason := range []string{pathPrivateAddress, pathProxied} {
		t.Run(reason, func(t *testing.T) {
			f := newFixture(t)
			f.client.setResult(overrideResult(enhancedHost, 0))
			f.paths.set(defaultHost, reason)

			reg := f.register(logsRequest(defaultHost, testKey))
			assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, reg.WaitInitial(context.Background()))
			assertNotNotified(t, reg)
			assert.Empty(t, f.client.queryCalls())
			assert.Empty(t, f.client.preflightCalls())
			assert.Nil(t, f.counter("queries"))
			assert.Equal(t, map[string]float64{"logs," + reason: 1}, f.counter("skipped_endpoints"))

			// The skip settles the group: the default host is checked again one refresh later.
			assert.Equal(t, testRefresh, f.nextAttempt(testKey))
			f.advance(testRefresh)
			assert.Empty(t, f.client.queryCalls())
			assert.Equal(t, map[string]float64{"logs," + reason: 2}, f.counter("skipped_endpoints"))

			// Once the default host is reached directly, the group is queried again.
			f.paths.set(defaultHost, "")
			f.advance(testRefresh)
			assert.Len(t, f.client.queryCalls(), 1)
			assertNotified(t, reg)
			assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint}, reg.Current())
		})
	}
}

func TestIndirectDefaultRevertsOverride(t *testing.T) {
	f := newFixture(t)
	f.client.setResult(overrideResult(enhancedHost, 0))
	reg := f.register(logsRequest(defaultHost, testKey))
	assertNotified(t, reg)

	// PrivateLink is set up while the override is active.
	f.paths.set(defaultHost, pathPrivateAddress)
	f.advance(testRefresh)
	assertNotified(t, reg)
	assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, reg.Current())
	assert.Len(t, f.client.queryCalls(), 1)
	assert.Equal(t, map[string]float64{"logs," + directionToEnhanced: 1, "logs," + directionToDefault: 1}, f.counter("switches"))
	assert.Equal(t, 0.0, f.activeOverrides())
}

func TestDefaultLookupFailureKeepsCurrent(t *testing.T) {
	f := newFixture(t)
	f.client.setResult(overrideResult(enhancedHost, 0))
	f.paths.set(defaultHost, pathLookupFailed)

	// The query is still sent, but its answer is not applied.
	reg := f.register(logsRequest(defaultHost, testKey))
	assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, reg.WaitInitial(context.Background()))
	assert.Len(t, f.client.queryCalls(), 1)
	assert.Empty(t, f.client.preflightCalls())
	assert.Equal(t, map[string]float64{"logs," + pathLookupFailed: 1}, f.counter("skipped_endpoints"))

	f.paths.set(defaultHost, "")
	f.advance(testRefresh)
	assertNotified(t, reg)
	assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint}, reg.Current())

	// A lookup failure does not revert an active override either.
	f.paths.set(defaultHost, pathLookupFailed)
	f.advance(testRefresh)
	assertNotNotified(t, reg)
	assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint}, reg.Current())
}

func TestPathsCheckedPerDefaultHost(t *testing.T) {
	f := newFixture(t)
	f.client.setResult(overrideResult(enhancedHost, 0))
	f.paths.set("agent-http-intake.logs.datadoghq.eu", pathPrivateAddress)

	// Each default host is checked once per pass, and only the indirect one keeps its default.
	reg := f.register(
		logsRequest(defaultHost, testKey),
		logsRequest(defaultHost, otherKey),
		logsRequest("agent-http-intake.logs.datadoghq.eu", testKey),
	)
	assert.ElementsMatch(t, []intakerouting.Endpoint{
		{Host: defaultHost},
		{Host: "agent-http-intake.logs.datadoghq.eu"},
	}, f.paths.checkCalls())
	assert.Equal(t, []intakerouting.Endpoint{
		enhancedEndpoint,
		enhancedEndpoint,
		{Host: "agent-http-intake.logs.datadoghq.eu"},
	}, reg.Current())
	// The datadoghq.eu group has no route that may be routed: it is not queried.
	assert.Len(t, f.client.queryCalls(), 2)
}

func TestUnofficialHostIsNotRouted(t *testing.T) {
	f := newFixture(t)
	reg := f.r.Register([]intakerouting.Request{logsRequest("logs.example.com", testKey)})
	custom := intakerouting.Endpoint{Host: "logs.example.com"}
	assert.Equal(t, []intakerouting.Endpoint{custom}, reg.WaitInitial(context.Background()))
	f.settle()
	assert.Empty(t, f.client.queryCalls())
}

func TestAPIKeyNotReady(t *testing.T) {
	f := newFixture(t)
	f.client.setResult(overrideResult(enhancedHost, 0))
	var ready atomic.Bool
	req := logsRequest(defaultHost, testKey)
	req.APIKey = func() (string, bool) { return testKey, ready.Load() }

	reg := f.register(req)
	// A route without a usable key keeps its default, and does not hold WaitInitial.
	assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, reg.WaitInitial(context.Background()))
	assert.Empty(t, f.client.queryCalls())

	f.advance(testKeyRecheck)
	assert.Empty(t, f.client.queryCalls())

	ready.Store(true)
	f.advance(testKeyRecheck)
	assert.Len(t, f.client.queryCalls(), 1)
	assertNotified(t, reg)
	assert.Equal(t, []intakerouting.Endpoint{enhancedEndpoint}, reg.Current())
}

func TestAPIKeyRotation(t *testing.T) {
	f := newFixture(t, func(o *routerOptions) { o.keyRecheck = keyRecheckInterval })
	f.client.setResult(overrideResult(enhancedHost, 0))
	var key atomic.Value
	key.Store(testKey)
	req := logsRequest(defaultHost, testKey)
	req.APIKey = func() (string, bool) { return key.Load().(string), true }

	reg := f.register(req)
	assertNotified(t, reg)

	// Reading an unchanged key sends no query.
	f.advance(keyRecheckInterval)
	assert.Len(t, f.client.queryCalls(), 1)

	key.Store(otherKey)
	f.client.setResult(noOverrideResult())
	// The rotation is picked up at the next key recheck, well before the next refresh.
	f.advance(keyRecheckInterval)

	queries := f.client.queryCalls()
	require.Len(t, queries, 2)
	assert.Equal(t, otherKey, queries[1].apiKey)
	assertNotified(t, reg)
	assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, reg.Current())

	f.r.mu.Lock()
	defer f.r.mu.Unlock()
	assert.Len(t, f.r.groups, 1, "the group of the old key is dropped")
}

func TestClose(t *testing.T) {
	f := newFixture(t)
	f.client.setResult(noOverrideResult())
	closed := f.register(logsRequest(defaultHost, testKey))
	open := f.register(logsRequest(defaultHost, testKey))

	closed.Close()
	closed.Close()

	f.client.setResult(overrideResult(enhancedHost, 0))
	f.advance(testRefresh)
	assertNotified(t, open)
	assertNotNotified(t, closed)
	assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, closed.Current())
	assert.Equal(t, 1.0, f.activeOverrides())

	open.Close()
	assert.Equal(t, 0.0, f.activeOverrides())
	f.r.mu.Lock()
	defer f.r.mu.Unlock()
	assert.Empty(t, f.r.groups)
}

func TestStopCancelsInFlightQueries(t *testing.T) {
	f := newFixture(t)
	f.client.gate = make(chan struct{})
	f.client.inFlight = make(chan struct{}, 1)

	reg := f.r.Register([]intakerouting.Request{logsRequest(defaultHost, testKey)})
	<-f.client.inFlight

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, f.r.stop(ctx))
	assert.Equal(t, []error{context.Canceled}, f.client.ctxErrs)
	assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, reg.Current())

	// Registering after stop returns the defaults right away.
	late := f.r.Register([]intakerouting.Request{logsRequest(defaultHost, otherKey)})
	assert.Equal(t, []intakerouting.Endpoint{defaultEndpoint}, late.WaitInitial(context.Background()))
	assert.Len(t, f.client.queryCalls(), 1)
}

func TestStopWithoutRegistration(t *testing.T) {
	f := newFixture(t)
	require.NoError(t, f.r.stop(context.Background()))
}
