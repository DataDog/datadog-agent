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
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
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

// TestBundleStartLifecycle exercises the full bundle (core + checkrunner + forwarder)
// through fx start/stop and locks in the start-order invariant: a check registered
// via the core component must (a) actually fire, (b) reach the in-memory store
// (proves the reporter is wired before the first tick), and (c) be POSTed to the
// intake (proves the provider is wired before the forwarder ticks). If a future
// refactor flips SetReporter / SetProvider with RegisterCheck — or moves built-in
// check registration back into New — the first tick is silently dropped and this
// test fails.
func TestBundleStartLifecycle(t *testing.T) {
	// Mock intake server to capture forwarded reports.
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

	// Force the persistence selector off the Kubernetes branch so the test is
	// not sensitive to the CI runner's environment.
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	t.Setenv("KUBERNETES", "")

	type appDeps struct {
		fx.In
		HP healthplatformdef.Component
	}

	// Intervals well below the test timeout so the lifecycle work completes quickly.
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

	// Register a custom check after fx.Start has returned. We use a unique
	// checkID so platform-specific built-in checks (docker / rofs) do not
	// confuse the assertions.
	var checkRunCount atomic.Int32
	const (
		testCheckID   = "test-bundle-lifecycle-check"
		testCheckName = "Test Bundle Lifecycle Check"
		// Reuse a real issue ID registered by the bundle's side-effect imports
		// so the registry's BuildIssue lookup succeeds.
		testIssueID = "docker-file-tailing-disabled"
	)
	require.NoError(t, deps.HP.RegisterCheck(testCheckID, testCheckName, func() (*healthplatformpayload.IssueReport, error) {
		checkRunCount.Add(1)
		return &healthplatformpayload.IssueReport{
			IssueId: testIssueID,
			Context: map[string]string{
				"dockerDir": "/var/lib/docker",
				"os":        "linux",
			},
		}, nil
	}, tickInterval))

	require.Eventually(t, func() bool { return checkRunCount.Load() > 0 },
		2*time.Second, 10*time.Millisecond,
		"check function never fired — checkrunner did not spawn its goroutine")

	require.Eventually(t, func() bool {
		return deps.HP.GetIssueForCheck(testCheckID) != nil
	}, 2*time.Second, 10*time.Millisecond,
		"core never recorded the issue — reporter not wired before first check fired")

	require.Eventually(t, func() bool { return receivedRequests.Load() > 0 },
		2*time.Second, 10*time.Millisecond,
		"forwarder never POSTed — provider not wired before forwarder ticked")

	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, receivedReports, "expected at least one health report payload")
	found := false
	for _, rep := range receivedReports {
		if _, ok := rep.Issues[testCheckID]; ok {
			found = true
			break
		}
	}
	assert.True(t, found, "no received report contained the test check's issue (checkID=%s)", testCheckID)
}
