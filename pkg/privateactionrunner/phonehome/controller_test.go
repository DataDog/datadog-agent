// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package phonehome

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
)

type supervisor struct {
	pb.ProcessManagerClient
	control  *pb.ProcessDetail
	executor *pb.ProcessDetail
	starts   int
	onStart  func()
}

func (s *supervisor) Describe(_ context.Context, r *pb.DescribeRequest, _ ...grpc.CallOption) (*pb.DescribeResponse, error) {
	p := s.control
	if r.NameOrUuid == executorProcess {
		p = s.executor
	}
	return &pb.DescribeResponse{Detail: p}, nil
}
func (s *supervisor) Start(_ context.Context, r *pb.StartRequest, _ ...grpc.CallOption) (*pb.StartResponse, error) {
	if r.NameOrUuid != "control-id" {
		return nil, fmt.Errorf("unexpected target")
	}
	s.starts++
	s.control.State = pb.ProcessState_RUNNING
	if s.onStart != nil {
		s.onStart()
	}
	return &pb.StartResponse{}, nil
}

func fixture(t *testing.T, handler http.HandlerFunc) (*controller, *supervisor, *State) {
	t.Helper()
	cfg := configmock.New(t)
	cfg.SetInTest(setup.PAREnabled, true)
	cfg.SetInTest(setup.PARSelfEnroll, true)
	cfg.SetInTest(setup.PARApiKeyOnlyEnrollment, true)
	cfg.SetInTest(setup.PARIdentityFilePath, filepath.Join(t.TempDir(), "identity.json"))
	cfg.SetInTest("api_key", "test-key")
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cfg.SetInTest("dd_url", srv.URL)
	pm := &supervisor{
		control:  &pb.ProcessDetail{Uuid: "control-id", State: pb.ProcessState_CREATED, RestartPolicy: "never"},
		executor: &pb.ProcessDetail{Uuid: "executor-id", State: pb.ProcessState_CREATED, RestartPolicy: "never", Env: map[string]string{app.PhoneHomePOCEnvVar: "true"}},
	}
	state := &State{}
	return &controller{cfg: cfg, hostname: "test-host", client: srv.Client(), endpoint: srv.URL, pm: pm, emit: func(s State) { *state = s }}, pm, state
}

// TestPhoneHomeScenario is the reproducible local demo: real enrollment client,
// generated keys, actual identity persistence, mocked backend, supervisor fake.
func TestPhoneHomeScenario(t *testing.T) {
	t.Setenv(app.PhoneHomePOCEnvVar, "true")
	t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
	var scoped atomic.Bool
	var gets, posts atomic.Int32
	c, pm, state := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "test-key", r.Header.Get("DD-API-KEY"))
		require.Empty(t, r.Header.Get("DD-APPLICATION-KEY"))
		switch r.URL.Path {
		case "/api/v2/validate":
			gets.Add(1)
			scopes := `[]`
			if scoped.Load() {
				scopes = `["private_action_runner_enroll"]`
			}
			fmt.Fprintf(w, `{"data":{"attributes":{"valid":true,"api_key_scopes":%s}}}`, scopes)
		case "/api/unstable/on_prem_runners/api_key_only":
			posts.Add(1)
			fmt.Fprint(w, `{"data":{"type":"createRunnerResponse","id":"runner","attributes":{"org_id":42,"runner_id":"runner"}}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	})
	now := time.Now()
	for i := 0; i < 3; i++ {
		delay := c.step(context.Background(), now)
		require.Equal(t, "waiting", state.State)
		require.Equal(t, "missing_enrollment_scope", state.Reason)
		require.GreaterOrEqual(t, delay, time.Minute)
		require.Less(t, delay, 72*time.Second)
		now = now.Add(delay)
	}
	require.Equal(t, 0, pm.starts)
	require.EqualValues(t, 0, posts.Load())
	scoped.Store(true)
	c.step(context.Background(), now)
	require.Equal(t, 1, pm.starts)
	require.Equal(t, "launching", state.State)
	// Launch is NOT enrollment: core keeps checking discovery and the identity.
	now = now.Add(75 * time.Second)
	c.step(context.Background(), now)
	require.EqualValues(t, 5, gets.Load())
	require.Equal(t, "launching", state.State)
	result, err := enrollment.Enroll(context.Background(), c.cfg, &enrollment.AgentIdentifier{Hostname: "test-host"})
	require.NoError(t, err)
	require.NoError(t, enrollment.PersistIdentity(context.Background(), c.cfg, result))
	require.Zero(t, c.step(context.Background(), now.Add(time.Second)))
	require.Equal(t, "enrolled", state.State)
	require.NoDirExists(t, enrollment.PhoneHomeAttemptPath(c.cfg), "confirmed identity clears the latch for future key/hostname re-enrollment")
	require.EqualValues(t, 1, posts.Load())
	// A new core controller adopts the same running PAR without scope/POST/start.
	restarted := &controller{cfg: c.cfg, hostname: c.hostname, client: c.client, endpoint: c.endpoint, pm: pm, emit: c.emit}
	scoped.Store(false)
	require.Zero(t, restarted.step(context.Background(), now.Add(time.Hour)))
	require.Equal(t, 1, pm.starts)
	require.EqualValues(t, 5, gets.Load())
	t.Log("3 unscoped checks, zero PAR starts; grant -> one launch; pending still polls; persisted -> polling stops; core restart reuses identity")
}

func TestLegacyIdentityNeedsNoScopeAndReenrollmentBypassesDiscovery(t *testing.T) {
	for _, hostname := range []string{"", "old-host"} {
		t.Run("hostname="+hostname, func(t *testing.T) {
			c, pm, state := fixture(t, func(_ http.ResponseWriter, _ *http.Request) { t.Error("existing identity must not require discovery") })
			key, _, err := util.GenerateKeys()
			require.NoError(t, err)
			require.NoError(t, enrollment.PersistIdentity(context.Background(), c.cfg, &enrollment.Result{URN: util.MakeRunnerURN("us1", 42, "legacy"), PrivateKey: key.Key.(*ecdsa.PrivateKey), Hostname: hostname}))
			c.step(context.Background(), time.Now())
			require.Equal(t, 1, pm.starts)
			if hostname == "" {
				require.Zero(t, c.step(context.Background(), time.Now()))
				require.Equal(t, "enrolled", state.State)
			} else {
				require.Positive(t, c.step(context.Background(), time.Now()))
				require.NotEqual(t, "enrolled", state.State)
			}
		})
	}
}

func TestConcurrentTicksAndRejectedEnrollmentNeverRelaunch(t *testing.T) {
	c, pm, state := fixture(t, func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"data":{"attributes":{"valid":true,"api_key_scopes":["private_action_runner_enroll"]}}}`)
	})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.step(context.Background(), time.Now()) }()
	}
	wg.Wait()
	require.Equal(t, 1, pm.starts)
	require.NoError(t, enrollment.RecordPhoneHomeFailure(c.cfg, "enrollment_rejected_not_necessarily_scope"))
	pm.control.State = pb.ProcessState_EXITED
	for i := 0; i < 3; i++ {
		c = &controller{cfg: c.cfg, hostname: c.hostname, client: c.client, endpoint: c.endpoint, pm: pm, emit: c.emit}
		delay := c.step(context.Background(), time.Now().Add(time.Duration(i)*time.Hour))
		require.Equal(t, slowRetry, delay)
		require.Equal(t, "blocked", state.State)
		require.Equal(t, "enrollment_rejected_not_necessarily_scope", state.Reason)
	}
	require.Equal(t, 1, pm.starts)
}

func TestDisabledAndUnsafeSupervisorDoNothing(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Setenv(app.PhoneHomePOCEnvVar, "false")
		cfg := configmock.New(t)
		cfg.SetInTest(setup.PAREnabled, enabled)
		Start(context.Background(), cfg, "test-host")
	}
	t.Setenv(app.PhoneHomePOCEnvVar, "true")
	Start(context.Background(), configmock.New(t), "test-host") // disabled, no dependencies
	c, pm, state := fixture(t, func(_ http.ResponseWriter, _ *http.Request) { t.Error("unsafe supervisor must not discover") })
	pm.control.AutoStart = true
	c.step(context.Background(), time.Now())
	require.Equal(t, 0, pm.starts)
	require.Equal(t, "supervisor_must_be_on_demand_no_restart", state.Reason)
	pm.control.AutoStart = false
	pm.executor.Env = nil
	c.step(context.Background(), time.Now())
	require.Zero(t, pm.starts)
	require.Equal(t, "executor_must_opt_into_poc_guard", state.Reason)
}

func TestCancellationAndBackoff(t *testing.T) {
	c, pm, _ := fixture(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) })
	now := time.Now()
	for _, minimum := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, slowRetry} {
		delay := c.step(context.Background(), now)
		require.GreaterOrEqual(t, delay, minimum)
		require.Less(t, delay, minimum+12*time.Second)
		now = now.Add(delay)
	}
	require.Zero(t, pm.starts)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waiting controller did not cancel")
	}

	entered := make(chan struct{})
	c, _, _ = fixture(t, func(_ http.ResponseWriter, r *http.Request) { close(entered); <-r.Context().Done() })
	ctx, cancel = context.WithCancel(context.Background())
	done = make(chan struct{})
	go func() { c.step(ctx, time.Now()); close(done) }()
	<-entered
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("HTTP request did not cancel")
	}
}

func TestUnknownOutcomeSurvivesRestart(t *testing.T) {
	c, pm, state := fixture(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(503) })
	require.NoError(t, os.Mkdir(enrollment.PhoneHomeAttemptPath(c.cfg), 0700))
	c.step(context.Background(), time.Now().Add(3*time.Minute))
	require.Equal(t, "blocked", state.State)
	require.Equal(t, "enrollment_outcome_unknown_operator_recovery_required", state.Reason)
	require.Zero(t, pm.starts)
}
