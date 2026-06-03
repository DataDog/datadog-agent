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
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	schedulerdef "github.com/DataDog/datadog-agent/comp/healthplatform/scheduler/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: agent-health

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
			cfg.SetWithoutSource("api_key", "test-api-key")
			cfg.SetWithoutSource("dd_url", server.URL)
			cfg.SetWithoutSource("health_platform.enabled", true)
			cfg.SetWithoutSource("health_platform.persist_on_kubernetes", true)
			cfg.SetWithoutSource("health_platform.forwarder.interval", tickInterval)
			cfg.SetWithoutSource("run_path", t.TempDir())
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
		testIssueName = "docker_file_tailing_disabled"
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

// TestAllModulesIssueNameMatchesBuiltIssueName guards the invariant that
// store.storeIssue keys issuesByName by issue.IssueName, while
// GetActiveIssueIDsByIssueName is called with module.IssueName(). They must
// match or restart-based issue resolution silently breaks.
func TestAllModulesIssueNameMatchesBuiltIssueName(t *testing.T) {
	cfg := config.NewMock(t)
	mods := issues.GetAllModules(cfg)
	require.NotEmpty(t, mods, "no modules registered")
	for _, mod := range mods {
		issue, err := mod.BuildIssue(map[string]string{})
		require.NoError(t, err, "module %s: BuildIssue failed", mod.IssueName())
		assert.Equal(t, mod.IssueName(), issue.IssueName,
			"module IssueName() %q must equal BuildIssue().IssueName %q",
			mod.IssueName(), issue.IssueName)
	}
}
