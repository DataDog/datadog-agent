// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package phonehome

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
)

// All RC/PAR grants here are INJECTED STATE, not end-to-end discovery coverage.
type injectedSource struct {
	state  Prerequisites
	wanted bool
	onRead func()
}

func (s *injectedSource) Read(time.Time) Prerequisites {
	if s.onRead != nil {
		s.onRead()
	}
	return s.state
}
func (s *injectedSource) Demand(wanted bool) { s.wanted = wanted }

type supervisor struct {
	pb.ProcessManagerClient
	control, executor *pb.ProcessDetail
	starts, stops     int
	onStart           func()
}

func (p *supervisor) Describe(_ context.Context, r *pb.DescribeRequest, _ ...grpc.CallOption) (*pb.DescribeResponse, error) {
	if r.NameOrUuid == controlProcess {
		return &pb.DescribeResponse{Detail: p.control}, nil
	}
	return &pb.DescribeResponse{Detail: p.executor}, nil
}
func (p *supervisor) Start(ctx context.Context, _ *pb.StartRequest, _ ...grpc.CallOption) (*pb.StartResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p.starts++
	p.control.State = pb.ProcessState_RUNNING
	if p.onStart != nil {
		p.onStart()
	}
	return &pb.StartResponse{}, nil
}
func (p *supervisor) Stop(_ context.Context, r *pb.StopRequest, _ ...grpc.CallOption) (*pb.StopResponse, error) {
	p.stops++
	if r.NameOrUuid == p.control.Uuid {
		p.control.State = pb.ProcessState_STOPPED
	} else {
		p.executor.State = pb.ProcessState_STOPPED
	}
	return &pb.StopResponse{}, nil
}

type fixture struct {
	c                    *controller
	pm                   *supervisor
	source               *injectedSource
	clock                *clock.Mock
	state                State
	posts, registrations int
	behavior             string
	requestDone          chan struct{}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	t.Setenv(app.PhoneHomePOCEnvVar, "true")
	t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
	f := &fixture{clock: clock.NewMock(), requestDone: make(chan struct{}, 1)}
	f.clock.Set(time.Now().UTC())
	cfg := configmock.New(t)
	for k, v := range map[string]interface{}{par.Enabled: true, par.SelfEnroll: true, par.APIKeyOnlyEnrollment: true, par.IdentityFilePath: filepath.Join(t.TempDir(), "identity.json"), "api_key": "test-key"} {
		cfg.SetInTest(k, v)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { f.requestDone <- struct{}{} }()
		if r.Method != "POST" {
			t.Error("PAR controller performed discovery HTTP")
			w.WriteHeader(500)
			return
		}
		f.posts++
		switch f.behavior {
		case "quota":
			reject(w, 403, "your organization has reached the maximum number of private action runners")
			return
		case "scope":
			reject(w, 403, "required scope missing")
			return
		case "unknown":
			w.WriteHeader(403)
			return
		case "throttle":
			w.Header().Set("Retry-After", "3600")
			w.WriteHeader(429)
			return
		}
		f.registrations++
		if f.behavior == "lost" {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
			return
		}
		if f.behavior == "storage" {
			if err := os.Mkdir(cfg.GetString(par.IdentityFilePath), 0700); err != nil {
				t.Error(err)
			}
		}
		fmt.Fprint(w, `{"data":{"type":"createRunnerResponse","id":"runner","attributes":{"org_id":42,"runner_id":"runner"}}}`)
	}))
	t.Cleanup(server.Close)
	cfg.SetInTest("dd_url", server.URL)
	f.pm = &supervisor{control: &pb.ProcessDetail{Uuid: "control", State: pb.ProcessState_CREATED, RestartPolicy: "never"}, executor: &pb.ProcessDetail{Uuid: "executor", State: pb.ProcessState_CREATED, RestartPolicy: "never", Env: map[string]string{app.PhoneHomePOCEnvVar: "true"}}}
	f.source = &injectedSource{state: Prerequisites{RCReady: true, RCReason: "rc_ready", ScopeReady: true, ScopeReason: "scope_present", ScopeCheckedAt: f.clock.Now()}}
	f.c = &controller{cfg: cfg, hostname: "host", source: f.source, now: f.clock.Now, pm: f.pm, emit: func(s State) { f.state = s }, jitter: func(time.Duration) time.Duration { return 0 }}
	return f
}
func reject(w http.ResponseWriter, code int, title string) {
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"errors":[{"status":"%d","title":%q}]}`, code, title)
}
func (f *fixture) tick(after time.Duration) time.Duration {
	f.clock.Add(after)
	return f.c.step(context.Background(), f.clock.Now())
}
func (f *fixture) restart() {
	old := f.c
	f.c = &controller{cfg: old.cfg, hostname: old.hostname, source: f.source, now: f.clock.Now, pm: f.pm, emit: old.emit, jitter: old.jitter}
}
func (f *fixture) enroll(t *testing.T) {
	t.Helper()
	f.pm.onStart = func() {
		_, err := enrollment.Enroll(context.Background(), f.c.cfg, &enrollment.AgentIdentifier{Hostname: f.c.hostname})
		// A hijacked connection does not synchronize the test with handler writes.
		select {
		case <-f.requestDone:
		case <-time.After(time.Second):
			t.Fatal("enrollment did not reach the fake backend")
		}
		if f.behavior == "" {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
}
func (f *fixture) attempt(t *testing.T) *enrollment.PhoneHomeAttempt {
	t.Helper()
	a, err := enrollment.ReadPhoneHomeAttempt(f.c.cfg)
	require.NoError(t, err)
	require.NotNil(t, a)
	return a
}
func (f *fixture) identity(t *testing.T) {
	t.Helper()
	key, _, err := util.GenerateKeys()
	require.NoError(t, err)
	require.NoError(t, enrollment.PersistIdentity(context.Background(), f.c.cfg, &enrollment.Result{PrivateKey: key.Key.(*ecdsa.PrivateKey), URN: util.MakeRunnerURN("us1", 42, "existing")}))
}

func TestClosedRCGateNeverStartsEvenWithIdentityOrScope(t *testing.T) {
	for _, existing := range []bool{false, true} {
		for _, reason := range []string{"rc_org_disabled", "rc_key_unauthorized", "rc_both_false", "rc_unchecked", "rc_credential_or_endpoint_mismatch", "rc_locally_disabled", "rc_service_unavailable"} {
			t.Run(fmt.Sprintf("%t_%s", existing, reason), func(t *testing.T) {
				f := newFixture(t)
				if existing {
					f.identity(t)
				}
				f.source.state.RCReady = false
				f.source.state.RCReason = reason
				f.tick(0)
				require.Zero(t, f.pm.starts)
				require.Zero(t, f.posts)
				require.False(t, f.source.wanted)
				require.Equal(t, reason, f.state.Reason)
			})
		}
	}
}

func TestRCOnlyAndThenInjectedScope(t *testing.T) {
	f := newFixture(t)
	f.source.state.ScopeReady = false
	f.source.state.ScopeCheckedAt = time.Time{}
	f.source.state.ScopeReason = "par_scope_unchecked"
	for i := 0; i < 3; i++ {
		f.tick(time.Minute)
	}
	require.Zero(t, f.pm.starts)
	require.Zero(t, f.posts)
	require.True(t, f.source.wanted)
	f.source.state.ScopeReady = true
	f.source.state.ScopeCheckedAt = f.clock.Now()
	f.enroll(t)
	f.tick(time.Minute)
	require.Equal(t, 1, f.pm.starts)
	require.Equal(t, 1, f.posts)
	require.Equal(t, 1, f.registrations)
	require.Zero(t, f.tick(time.Second))
	require.Equal(t, "enrolled", f.state.State)
	require.False(t, f.source.wanted)
}

func TestEnrolledStartupWaitsForRCButRunningPARIsNotStopped(t *testing.T) {
	f := newFixture(t)
	f.identity(t)
	f.source.state.ScopeReady = false
	f.source.state.RCReady = false
	f.tick(0)
	require.Zero(t, f.pm.starts)
	f.source.state.RCReady = true
	f.tick(time.Minute)
	require.Equal(t, 1, f.pm.starts)
	f.source.state.RCReady = false
	require.Zero(t, f.tick(time.Second))
	require.Zero(t, f.pm.stops)
	require.Zero(t, f.posts)
	require.False(t, f.source.wanted)
}

func TestDispatchRechecksRCAndUnstartedReservationRecovers(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(strconv.FormatBool(existing), func(t *testing.T) {
			f := newFixture(t)
			if existing {
				f.identity(t)
			}
			reads := 0
			f.source.onRead = func() {
				reads++
				if reads == 2 {
					f.source.state.RCReady = false
					f.source.state.RCReason = "rc_org_disabled"
				}
			}
			f.tick(0)
			require.Zero(t, f.pm.starts)
			require.Zero(t, f.posts)
			f.source.onRead = nil
			f.source.state.RCReady = true
			if !existing {
				f.tick(time.Second)
				f.tick(5 * time.Minute)
			} else {
				f.tick(time.Minute)
			}
			require.Equal(t, 1, f.pm.starts)
		})
	}
}

func TestSafeRejectionsKeepCooldownAndAuthNeedsNewObservation(t *testing.T) {
	for _, behavior := range []string{"quota", "scope"} {
		t.Run(behavior, func(t *testing.T) {
			f := newFixture(t)
			f.behavior = behavior
			f.enroll(t)
			f.tick(0)
			f.tick(time.Second)
			a := f.attempt(t)
			deadline := a.NextAttemptAt
			f.source.state.RCReady = false
			f.restart()
			f.tick(time.Minute)
			require.True(t, deadline.Equal(f.attempt(t).NextAttemptAt))
			require.Equal(t, 1, f.posts)
			f.behavior = ""
			f.source.state.RCReady = true
			f.tick(5 * time.Minute)
			if behavior == "scope" {
				require.Equal(t, 1, f.posts, "old positive must not revalidate a rejected POST")
				require.Equal(t, "par_scope_revalidation_required", f.state.Reason)
				f.source.state.ScopeCheckedAt = f.clock.Now()
				f.tick(time.Second)
			}
			require.Equal(t, 2, f.posts)
			require.Equal(t, 1, f.registrations)
			require.Zero(t, f.tick(time.Second))
		})
	}
}

func TestAmbiguityAndUnknownDenialsNeverReplay(t *testing.T) {
	for _, behavior := range []string{"unknown", "throttle", "lost"} {
		t.Run(behavior, func(t *testing.T) {
			f := newFixture(t)
			f.behavior = behavior
			f.enroll(t)
			f.tick(0)
			f.tick(time.Second)
			a := f.attempt(t)
			path := f.c.cfg.GetString(par.IdentityFilePath) + ".pending-" + a.ID
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			for i := 0; i < 3; i++ {
				f.restart()
				f.tick(time.Hour)
			}
			require.Equal(t, 1, f.pm.starts)
			require.Equal(t, 1, f.posts)
			require.True(t, f.state.ReconciliationRequired)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
			if behavior == "lost" {
				require.Equal(t, 1, f.registrations)
			} else {
				require.Zero(t, f.registrations)
			}
		})
	}
}

func TestStorageRecoveryWaitsForRCAndBypassesScope(t *testing.T) {
	f := newFixture(t)
	f.behavior = "storage"
	f.enroll(t)
	f.tick(0)
	f.tick(time.Second)
	require.Equal(t, 1, f.posts)
	f.source.state.RCReady = false
	f.source.state.ScopeReady = false
	f.restart()
	f.tick(2 * time.Minute)
	require.Equal(t, 1, f.pm.starts)
	require.NoError(t, os.Remove(f.c.cfg.GetString(par.IdentityFilePath)))
	f.pm.onStart = func() {
		require.NoError(t, enrollment.RecoverPhoneHomeIdentity(context.Background(), f.c.cfg, f.c.hostname))
	}
	f.source.state.RCReady = true
	f.tick(time.Minute)
	require.Equal(t, 2, f.pm.starts)
	require.Zero(t, f.tick(time.Second))
	require.Equal(t, 1, f.posts)
}

func TestRunningCredentialHolderSurvivesClosedRC(t *testing.T) {
	f := newFixture(t)
	f.tick(0)
	a := f.attempt(t)
	require.NoError(t, enrollment.RecordPhoneHomeOutcome(f.c.cfg, enrollment.PhoneHomeOutcome{AttemptID: a.ID, EnrollmentFailure: opms.EnrollmentFailure{Category: "persistence_memory"}, PersistencePending: true, HelperRetained: true}))
	f.pm.executor.State = pb.ProcessState_RUNNING
	f.source.state.RCReady = false
	f.tick(time.Hour)
	require.True(t, f.state.HelperRetained)
	require.Zero(t, f.pm.stops)
	require.Equal(t, 1, f.pm.starts)
	require.False(t, f.source.wanted)
}

func TestConcurrencyPoliciesDisableAndCancellation(t *testing.T) {
	f := newFixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); f.c.step(context.Background(), f.clock.Now()) }()
	}
	wg.Wait()
	require.Equal(t, 1, f.pm.starts)
	f.c.cfg.(model.ReaderWriter).Set(par.Enabled, false, model.SourceAgentRuntime)
	require.Zero(t, f.tick(0))
	require.False(t, f.source.wanted)
	f = newFixture(t)
	f.pm.control.RestartPolicy = "always"
	f.tick(0)
	require.Zero(t, f.pm.starts)
	f = newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Zero(t, f.c.step(ctx, f.clock.Now()))
	require.Zero(t, f.pm.starts)
	f.identity(t)
	t.Setenv(app.PhoneHomePOCEnvVar, "false")
	require.Zero(t, f.tick(0))
	require.Zero(t, f.pm.starts)
	Run(context.Background(), f.c.cfg, "host", f.source)
	require.Zero(t, f.pm.starts)
}
