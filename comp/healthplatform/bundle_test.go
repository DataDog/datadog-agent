// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package healthplatform

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	schedulerdef "github.com/DataDog/datadog-agent/comp/healthplatform/scheduler/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: fleet-remediation

func TestBundleDependencies(t *testing.T) {
	fxutil.TestBundle(t, Bundle(),
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		fx.Provide(func(t testing.TB) config.Component { return config.NewMock(t) }),
		telemetrymock.Module(),
		hostnameinterface.MockModule(),
	)
}

// TestBundleStartLifecycle exercises the full bundle through fx start/stop and
// verifies: (a) a scheduled check fires, (b) its issue reaches the in-memory
// store, and (c) the issue is forwarded to the intake via the forwarder.
func TestBundleStartLifecycle(t *testing.T) {
	var receivedRequests atomic.Int32
	var (
		mu              sync.Mutex
		receivedReports []*healthplatformpayload.HealthReport
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if body, err := io.ReadAll(r.Body); err == nil {
			var rep healthplatformpayload.HealthReport
			if json.Unmarshal(body, &rep) == nil {
				mu.Lock()
				receivedReports = append(receivedReports, &rep)
				mu.Unlock()
			}
		}
		receivedRequests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	t.Setenv("KUBERNETES", "")

	type appDeps struct {
		fx.In
		HP        storedef.Component
		Scheduler schedulerdef.Component
	}

	const tickInterval = 50 * time.Millisecond

	deps := fxutil.Test[appDeps](t,
		Bundle(),
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		fx.Provide(func(t testing.TB) config.Component {
			cfg := config.NewMock(t)
			cfg.SetInTest("api_key", "test-api-key")
			cfg.SetInTest("dd_url", server.URL)
			cfg.SetInTest("health_platform.enabled", true)
			cfg.SetInTest("health_platform.persist_on_kubernetes", true)
			cfg.SetInTest("health_platform.forwarder.interval", tickInterval)
			cfg.SetInTest("run_path", t.TempDir())
			return cfg
		}),
		telemetrymock.Module(),
		hostnameinterface.MockModule(),
	)

	var checkRunCount atomic.Int32
	const (
		testSource  = "test-bundle-lifecycle"
		testIssueID = "test-bundle-lifecycle-issue"
		// Reuse a real issue name registered by the bundle's side-effect imports
		// so the registry's BuildIssue lookup succeeds.
		testIssueName = "Docker File Tailing Disabled"
	)
	require.NoError(t, deps.Scheduler.Schedule(testSource, func() ([]runnerdef.IssueReport, error) {
		checkRunCount.Add(1)
		return []runnerdef.IssueReport{
			{
				IssueID:   testIssueID,
				IssueName: testIssueName,
				Source:    testSource,
				Context: map[string]string{
					"dockerDir": "/var/lib/docker",
					"os":        "linux",
				},
			},
		}, nil
	}, tickInterval, nil))

	require.Eventually(t, func() bool { return checkRunCount.Load() > 0 },
		2*time.Second, 10*time.Millisecond,
		"check function never fired")

	require.Eventually(t, func() bool {
		return deps.HP.GetIssue(testIssueID) != nil
	}, 2*time.Second, 10*time.Millisecond,
		"store never recorded the issue")

	require.Eventually(t, func() bool { return receivedRequests.Load() > 0 },
		2*time.Second, 10*time.Millisecond,
		"forwarder never POSTed")

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, receivedReports, "expected at least one health report payload")
	found := false
	for _, rep := range receivedReports {
		if _, ok := rep.Issues[testIssueID]; ok {
			found = true
			break
		}
	}
	assert.True(t, found, "no received report contained the test issue (id=%s)", testIssueID)
}

// TestIssueStateLifecycleForwarded exercises the full issue state machine end-to-end.
func TestIssueStateLifecycleForwarded(t *testing.T) {
	ready := make(chan bool, 1)
	fi := fakeintakeserver.NewServer(
		fakeintakeserver.WithAddress("127.0.0.1:0"),
		fakeintakeserver.WithReadyChannel(ready),
	)
	fi.Start()
	require.True(t, <-ready, "fakeintake server did not become ready")
	t.Cleanup(func() { _ = fi.Stop() })

	fiClient := fakeintakeclient.NewClient(fi.URL())

	type appDeps struct {
		fx.In
		HP storedef.Component
	}

	const tickInterval = 500 * time.Millisecond

	deps := fxutil.Test[appDeps](t,
		Bundle(),
		fx.Provide(func(t testing.TB) log.Component { return logmock.New(t) }),
		fx.Provide(func(t testing.TB) config.Component {
			cfg := config.NewMock(t)
			cfg.SetInTest("api_key", "test-api-key")
			cfg.SetInTest("dd_url", fi.URL())
			cfg.SetInTest("health_platform.enabled", true)
			cfg.SetInTest("health_platform.persist_on_kubernetes", true)
			cfg.SetInTest("health_platform.forwarder.interval", tickInterval)
			cfg.SetInTest("run_path", t.TempDir())
			return cfg
		}),
		telemetrymock.Module(),
		hostnameinterface.MockModule(),
	)

	const (
		issueAID      = "test-lifecycle-A"
		issueBID      = "test-lifecycle-B"
		testIssueName = "Docker File Tailing Disabled"
		testIssueType = "docker_file_tailing_disabled"
		testSource    = "test-lifecycle"
	)

	const (
		waitTimeout  = 2 * time.Second
		waitInterval = 10 * time.Millisecond
	)

	// latestIssue uses collectedTime (ns precision) rather than EmittedAt (RFC3339, s precision).
	latestIssue := func(issueID string) *healthplatformpayload.Issue {
		payloads, err := fiClient.GetAgentHealth()
		if err != nil || len(payloads) == 0 {
			return nil
		}
		latest := payloads[0]
		for _, p := range payloads[1:] {
			if p.GetCollectedTime().After(latest.GetCollectedTime()) {
				latest = p
			}
		}
		return latest.Issues[issueID]
	}

	latestHasIssueState := func(issueID string, state healthplatformpayload.IssueState) bool {
		iss := latestIssue(issueID)
		return iss != nil && iss.PersistedIssue != nil && iss.PersistedIssue.State == state
	}

	issueA := &healthplatformpayload.Issue{
		Id:        issueAID,
		IssueName: testIssueName,
		IssueType: testIssueType,
		Source:    testSource,
	}

	issueB := &healthplatformpayload.Issue{
		Id:        issueBID,
		IssueName: testIssueName,
		IssueType: testIssueType,
		Source:    testSource,
	}

	deps.HP.ReportIssue(issueA)
	deps.HP.ReportIssue(issueB)
	require.Eventually(t, func() bool {
		return latestHasIssueState(issueAID, healthplatformpayload.IssueState_ISSUE_STATE_ACTIVE) &&
			latestHasIssueState(issueBID, healthplatformpayload.IssueState_ISSUE_STATE_ACTIVE)
	}, waitTimeout, waitInterval, "issueA and issueB never appeared as ACTIVE in forwarded reports")

	// IssueType is caller-set (never overwritten by the store) and must survive
	// the full store -> egress -> forwarder -> fakeintake round-trip unchanged.
	if iss := latestIssue(issueAID); assert.NotNil(t, iss) {
		assert.Equal(t, testIssueType, iss.IssueType)
	}

	deps.HP.ReportIssue(issueA)
	require.Eventually(t, func() bool {
		return latestHasIssueState(issueAID, healthplatformpayload.IssueState_ISSUE_STATE_ACTIVE)
	}, waitTimeout, waitInterval, "issueA not seen as ACTIVE after second report")

	deps.HP.ResolveIssue(issueAID)
	deps.HP.ResolveIssue(issueBID)

	// The egress component may split issues resolved back-to-back across separate
	// ticks/reports, so issueA and issueB reaching RESOLVED is not guaranteed to land in
	// the same latest payload. Latch each as seen once observed, and stop once both have.
	var seenAResolved, seenBResolved bool
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		if latestHasIssueState(issueAID, healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED) {
			seenAResolved = true
		}
		if latestHasIssueState(issueBID, healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED) {
			seenBResolved = true
		}
		assert.True(c, seenAResolved, "issueA never observed as RESOLVED")
		assert.True(c, seenBResolved, "issueB never observed as RESOLVED")
	}, waitTimeout, waitInterval, "expected issueA and issueB to each be forwarded as RESOLVED")

	deps.HP.ReportIssue(issueA)
	require.Eventually(t, func() bool {
		return latestHasIssueState(issueAID, healthplatformpayload.IssueState_ISSUE_STATE_ACTIVE)
	}, waitTimeout, waitInterval, "issueA never re-appeared as ACTIVE after resolve+re-report")

	// RESOLVED must appear exactly once: tombstones are removed after a successful send.
	allPayloads, err := fiClient.GetAgentHealth()
	require.NoError(t, err)
	resolvedCountA, resolvedCountB := 0, 0
	for _, p := range allPayloads {
		if p == nil || p.HealthReport == nil {
			continue
		}
		if iss, ok := p.Issues[issueAID]; ok && iss != nil && iss.PersistedIssue != nil &&
			iss.PersistedIssue.State == healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED {
			resolvedCountA++
		}
		if iss, ok := p.Issues[issueBID]; ok && iss != nil && iss.PersistedIssue != nil &&
			iss.PersistedIssue.State == healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED {
			resolvedCountB++
		}
	}
	require.Equal(t, 1, resolvedCountA, "issueA RESOLVED forwarded more than once")
	require.Equal(t, 1, resolvedCountB, "issueB RESOLVED forwarded more than once")
}

// TestAllModulesIssueNameMatchesBuiltIssueName guards the invariant that
// store.storeIssue keys issuesByName by issue.IssueName, while
// GetActiveIssueIDsByIssueName is called with module.IssueName(). They must
// match or restart-based issue resolution silently breaks.
func TestAllModulesIssueNameMatchesBuiltIssueName(t *testing.T) {
	cfg := config.NewMock(t)
	hn, _ := hostnameinterface.NewMock("test-host")
	mods := issues.GetAllModules(issues.ModuleDeps{Config: cfg, Hostname: hn})
	require.NotEmpty(t, mods, "no modules registered")
	for _, mod := range mods {
		issue, err := mod.BuildIssue(map[string]string{})
		require.NoError(t, err, "module %s: BuildIssue failed", mod.IssueName())
		assert.Equal(t, mod.IssueName(), issue.IssueName,
			"module IssueName() %q must equal BuildIssue().IssueName %q",
			mod.IssueName(), issue.IssueName)
		assert.Equal(t, mod.IssueType(), issue.IssueType,
			"module IssueType() %q must equal BuildIssue().IssueType %q",
			mod.IssueType(), issue.IssueType)
	}
}
