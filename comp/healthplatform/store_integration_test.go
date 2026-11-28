// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package healthplatform

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	schemanoopfx "github.com/DataDog/datadog-agent/comp/core/schema/fx-noop"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	forwarderimpl "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/impl"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	storeimpl "github.com/DataDog/datadog-agent/comp/healthplatform/store/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	fakeintakeserver "github.com/DataDog/datadog-agent/test/fakeintake/server"
)

// team: fleet-remediation

// These tests cover the asynchronous store through the whole pipeline, as a direct reporter such as Autodiscovery
// uses it: ReportIssue and ResolveIssue only queue the change, the store's worker applies it, the egress sends it, and
// the persister saves it. They run in a testing/synctest bubble against an in-memory fakeintake, like
// TestInvalidConfigPayload, so the test knows when the queued changes were applied and sent rather than polling.

// storeTestIssueName and storeTestIssueType identify the issues these tests report directly to the store, apart
// from those the bundle's own checks report
const (
	storeTestIssueName = "Store integration test issue"
	storeTestIssueType = "store_integration_test"
)

// storeTestEnv starts an in-memory fakeintake the Agent forwarder reaches, and returns its client and the options of
// a health platform bundle sending to it, persisting under runPath
func storeTestEnv(t *testing.T, runPath string) (*fakeintakeclient.Client, fx.Option) {
	t.Helper()
	listener := newPipeListener()
	ready := make(chan bool, 1)
	fi := fakeintakeserver.NewServer(
		fakeintakeserver.WithListener(listener, "http://"+pipeListenerAddress),
		fakeintakeserver.WithReadyChannel(ready),
	)
	fi.Start()
	require.True(t, <-ready, "fakeintake server did not become ready")
	t.Cleanup(func() { _ = fi.Stop() })
	clientTransport := listener.transport()
	t.Cleanup(clientTransport.CloseIdleConnections)
	fiClient := fakeintakeclient.NewClient(fi.URL(), fakeintakeclient.WithHTTPTransport(clientTransport))
	agentTransport := listener.transport()
	t.Cleanup(agentTransport.CloseIdleConnections)
	forwarderimpl.SetTransport(t, agentTransport)

	// clear the inherited DD_* and proxy variables, which the configuration would read, and point DOCKER_HOST away from
	// the host sockets: see setScenarioEnv
	setScenarioEnv(t, "")

	return fiClient, fx.Options(
		Bundle(),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component {
			cfg := config.NewMock(t)
			cfg.SetInTest("api_key", "0000001")
			cfg.SetInTest("run_path", runPath)
			cfg.SetInTest("health_platform.enabled", true)
			// the issues are persisted under run_path, even when the test runs on Kubernetes
			cfg.SetInTest("health_platform.persist_on_kubernetes", true)
			cfg.SetInTest("health_platform.forwarder.dd_url", fi.URL())
			cfg.SetInTest("health_platform.forwarder.interval", "100ms")
			return cfg
		}),
		telemetrymock.Module(),
		schemanoopfx.Module(),
		hostnameinterface.MockModule(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	)
}

func storeTestIssue(id string) *healthplatformpayload.Issue {
	return &healthplatformpayload.Issue{
		Id:        id,
		IssueName: storeTestIssueName,
		IssueType: storeTestIssueType,
		Title:     "Store integration test issue " + id,
		Source:    "store-integration-test",
	}
}

// receivedIssues returns, in arrival order, every version of the issue with the given ID the intake received
func receivedIssues(t *testing.T, fiClient *fakeintakeclient.Client, id string) []*healthplatformpayload.Issue {
	t.Helper()
	received, err := fiClient.GetAgentHealth()
	require.NoError(t, err)
	var versions []*healthplatformpayload.Issue
	for _, payload := range received {
		if issue, found := payload.Issues[id]; found {
			versions = append(versions, issue)
		}
	}
	return versions
}

func issueState(issue *healthplatformpayload.Issue) healthplatformpayload.IssueState {
	return issue.GetPersistedIssue().GetState()
}

// TestStoreReportAndResolveReachIntake checks that an issue reported directly to the store, without waiting for it
// to apply, reaches the intake, and that its resolution follows as a resolved issue.
func TestStoreReportAndResolveReachIntake(t *testing.T) {
	initProcessGlobals()
	synctest.Test(t, func(t *testing.T) {
		fiClient, opts := storeTestEnv(t, t.TempDir())
		store := fxutil.Test[storedef.Component](t, opts)

		require.NoError(t, store.ReportIssue(storeTestIssue("store-test:active")))
		waitForReports()
		versions := receivedIssues(t, fiClient, "store-test:active")
		require.NotEmpty(t, versions, "the reported issue never reached the intake")
		latest := versions[len(versions)-1]
		assert.Equal(t, storeTestIssueType, latest.GetIssueType())
		assert.Equal(t, healthplatformpayload.IssueState_ISSUE_STATE_ACTIVE, issueState(latest))

		store.ResolveIssue("store-test:active")
		waitForReports()
		versions = receivedIssues(t, fiClient, "store-test:active")
		latest = versions[len(versions)-1]
		assert.Equal(t, healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED, issueState(latest), "the resolution never reached the intake")
		assert.NotEmpty(t, latest.GetPersistedIssue().GetResolvedAt())
	})
}

// TestStorePersistsAcrossRestart checks that stopping the bundle saves the changes queued right before, and that a
// new bundle with the same run_path restores them (draining the queue on stop is covered by the store unit tests): the still-active issue is active again, and resolving it sends the
// FirstSeen of the first run.
func TestStorePersistsAcrossRestart(t *testing.T) {
	initProcessGlobals()
	synctest.Test(t, func(t *testing.T) {
		runPath := t.TempDir()

		// First run: queue the changes right before stopping, so the final save on stop is what persists them.
		_, firstOpts := storeTestEnv(t, runPath)
		app, store, err := fxutil.TestApp[storedef.Component](firstOpts)
		require.NoError(t, err)
		// TestApp does not stop the app: stop it on a failure before the explicit stop below, so the bubble exits with
		// the failure rather than a deadlock
		stopped := false
		t.Cleanup(func() {
			if !stopped {
				_ = app.Stop(context.Background())
			}
		})
		waitForReports() // the bundle's startup checks finished
		require.NoError(t, store.ReportIssue(storeTestIssue("store-test:ongoing")))
		require.NoError(t, store.ReportIssue(storeTestIssue("store-test:fixed")))
		store.ResolveIssue("store-test:fixed")
		stopped = true
		require.NoError(t, app.Stop(context.Background()))

		data, err := os.ReadFile(filepath.Join(runPath, "health-platform", "issues.json"))
		require.NoError(t, err, "stop must save the issues")
		var saved struct {
			Issues map[string]struct {
				State     string `json:"state"`
				FirstSeen string `json:"first_seen"`
			} `json:"issues"`
		}
		require.NoError(t, json.Unmarshal(data, &saved))
		require.Contains(t, saved.Issues, "store-test:ongoing", "a change queued right before stop must be in the final save")
		require.Contains(t, saved.Issues, "store-test:fixed")
		firstSeen := saved.Issues["store-test:ongoing"].FirstSeen
		require.NotEmpty(t, firstSeen)
		assert.Equal(t, "active", saved.Issues["store-test:ongoing"].State)
		assert.Equal(t, "resolved", saved.Issues["store-test:fixed"].State, "a resolution queued right before stop must be in the final save")

		// Second run: the store restores the issues saved by the first one.
		fiClient, secondOpts := storeTestEnv(t, runPath)
		store = fxutil.Test[storedef.Component](t, secondOpts)
		active := store.GetActiveIssueIDsByIssueName(storeTestIssueName)
		assert.Contains(t, active, "store-test:ongoing", "the active issue must be restored")
		assert.NotContains(t, active, "store-test:fixed", "the resolved issue must stay resolved")

		store.ResolveIssue("store-test:ongoing")
		waitForReports()
		versions := receivedIssues(t, fiClient, "store-test:ongoing")
		require.NotEmpty(t, versions, "the resolution of the restored issue never reached the intake")
		latest := versions[len(versions)-1]
		assert.Equal(t, healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED, issueState(latest))
		assert.Equal(t, firstSeen, latest.GetPersistedIssue().GetFirstSeen(), "the restored issue keeps the FirstSeen of the first run")
	})
}

// slowSaveDelay is how long every save of the store takes in TestSlowIssueProcessingDoesNotBlockReporters: longer
// than the 10 s persist interval, so a save is still in progress when the next one is due
const slowSaveDelay = 30 * time.Second

// TestSlowIssueProcessingDoesNotBlockReporters reproduces the incident this store follows up on: Autodiscovery called
// ReportIssue while holding its config lock, and ReportIssue saved the issue state before returning, so a slow save
// stalled the caller. Here every save takes slowSaveDelay. A reporter, standing in for Autodiscovery, reports an issue
// every second while saves are stuck: each call must return without waiting, and the health payloads must keep
// reaching the intake. A regression that makes ReportIssue wait on a timer or a channel fails the test; one that makes
// it wait on a mutex held during the save hangs it instead, since a goroutine blocked on a mutex keeps the bubble
// clock, and so the save, from advancing.
func TestSlowIssueProcessingDoesNotBlockReporters(t *testing.T) {
	initProcessGlobals()
	synctest.Test(t, func(t *testing.T) {
		saves := storeimpl.SetSaveDelay(t, slowSaveDelay)
		runPath := t.TempDir()
		fiClient, opts := storeTestEnv(t, runPath)
		store := fxutil.Test[storedef.Component](t, opts)
		// Registered after fxutil.Test, so it runs before the stop, also on a failure: let the stuck save finish and make
		// the final one fast, so the stop meets its deadline.
		t.Cleanup(func() {
			saves.SpeedUp()
			time.Sleep(slowSaveDelay)
		})

		// Report for longer than the persist interval plus one save, so reports keep coming while a save is stuck.
		const reports = 35
		for i := range reports {
			start := time.Now()
			require.NoError(t, store.ReportIssue(storeTestIssue(fmt.Sprintf("store-test:slow:%d", i))))
			// In the bubble, time only moves while every goroutine is blocked: a call waiting on the save would
			// advance it.
			require.Zero(t, time.Since(start), "ReportIssue %d waited for the issue to be processed", i)
			time.Sleep(time.Second)
		}

		// The first save started at the first persist tick and is still in progress: nothing is written yet.
		require.Positive(t, saves.Started(), "no slow save started while the reports came in")
		_, err := os.Stat(filepath.Join(runPath, "health-platform", "issues.json"))
		require.ErrorIs(t, err, os.ErrNotExist, "the slow save should still be in progress")

		// Every issue, including those reported while a save was stuck, reached the intake.
		waitForReports()
		for i := range reports {
			id := fmt.Sprintf("store-test:slow:%d", i)
			assert.NotEmpty(t, receivedIssues(t, fiClient, id), "issue %s never reached the intake while saves were slow", id)
		}
	})
}
