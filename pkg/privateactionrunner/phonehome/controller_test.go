// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package phonehome

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/setup"
	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type supervisor struct {
	pb.ProcessManagerClient
	control, executor *pb.ProcessDetail
	starts, stops     int
	onStart           func()
}

func (s *supervisor) Describe(_ context.Context, r *pb.DescribeRequest, _ ...grpc.CallOption) (*pb.DescribeResponse, error) {
	p := s.control
	if r.NameOrUuid == executorProcess {
		p = s.executor
	}
	return &pb.DescribeResponse{Detail: p}, nil
}
func (s *supervisor) Start(ctx context.Context, r *pb.StartRequest, _ ...grpc.CallOption) (*pb.StartResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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
func (s *supervisor) Stop(_ context.Context, r *pb.StopRequest, _ ...grpc.CallOption) (*pb.StopResponse, error) {
	p := s.control
	if r.NameOrUuid == s.executor.Uuid {
		p = s.executor
	}
	s.stops++
	p.State = pb.ProcessState_STOPPED
	return &pb.StopResponse{}, nil
}
func fixture(t *testing.T, handler http.HandlerFunc) (*controller, *supervisor, *State) {
	t.Helper()
	t.Setenv(app.PhoneHomePOCEnvVar, "true")
	t.Setenv(app.InternalUseDDURLForOPMSEnvVar, "true")
	cfg := configmock.New(t)
	for key, value := range map[string]interface{}{setup.PAREnabled: true, setup.PARSelfEnroll: true, setup.PARApiKeyOnlyEnrollment: true, setup.PARIdentityFilePath: filepath.Join(t.TempDir(), "identity.json"), "api_key": "test-key"} {
		cfg.SetInTest(key, value)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cfg.SetInTest("dd_url", srv.URL)
	pm := &supervisor{control: &pb.ProcessDetail{Uuid: "control-id", State: pb.ProcessState_CREATED, RestartPolicy: "never"}, executor: &pb.ProcessDetail{Uuid: "executor-id", State: pb.ProcessState_CREATED, RestartPolicy: "never", Env: map[string]string{app.PhoneHomePOCEnvVar: "true"}}}
	state := &State{}
	return &controller{cfg: cfg, hostname: "test-host", client: srv.Client(), endpoint: srv.URL, pm: pm, emit: func(s State) { *state = s }, jitter: func(time.Duration) time.Duration { return 0 }}, pm, state
}
func restart(c *controller) *controller {
	return &controller{cfg: c.cfg, hostname: c.hostname, client: c.client, endpoint: c.endpoint, pm: c.pm, emit: c.emit, jitter: c.jitter}
}
func scope(w http.ResponseWriter, ready bool) {
	scopes := `[]`
	if ready {
		scopes = `["private_action_runner_enroll"]`
	}
	fmt.Fprintf(w, `{"data":{"attributes":{"valid":true,"api_key_scopes":%s}}}`, scopes)
}
func reject(w http.ResponseWriter, code int, title string) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"errors": []map[string]string{{"status": fmt.Sprint(code), "title": title}}})
}
func created(w http.ResponseWriter) {
	fmt.Fprint(w, `{"data":{"type":"createRunnerResponse","id":"runner","attributes":{"org_id":42,"runner_id":"runner"}}}`)
}
func attempt(t *testing.T, c *controller) *enrollment.PhoneHomeAttempt {
	t.Helper()
	a, e := enrollment.ReadPhoneHomeAttempt(c.cfg)
	require.NoError(t, e)
	require.NotNil(t, a)
	return a
}
func logResult(t *testing.T, s *State, pm *supervisor, posts, registrations int) {
	t.Helper()
	t.Logf("state=%s reason=%s outcome=%s Start=%d Stop=%d POST=%d registrations=%d next_discovery=%s next_enrollment=%s retry_safe=%t reconcile=%t", s.State, s.Reason, s.EnrollmentOutcome, pm.starts, pm.stops, posts, registrations, s.NextDiscovery, s.NextEnrollmentAttempt, s.RetrySafe, s.ReconciliationRequired)
}

func TestPhoneHomeScenario(t *testing.T) {
	ready := false
	gets, posts := 0, 0
	c, pm, s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "test-key", r.Header.Get("DD-API-KEY"))
		require.Empty(t, r.Header.Get("DD-APPLICATION-KEY"))
		if r.Method == "GET" {
			gets++
			scope(w, ready)
		} else {
			posts++
			created(w)
		}
	})
	now := time.Now()
	for i := 0; i < 3; i++ {
		require.Equal(t, time.Minute, c.step(context.Background(), now))
		require.Equal(t, "waiting", s.State)
		now = now.Add(time.Minute)
	}
	require.Zero(t, pm.starts)
	require.Zero(t, posts)
	ready = true
	c.step(context.Background(), now)
	require.Equal(t, 1, pm.starts)
	require.Equal(t, "launching", s.State)
	c.step(context.Background(), now.Add(time.Minute))
	require.Equal(t, 5, gets)
	_, err := enrollment.Enroll(context.Background(), c.cfg, &enrollment.AgentIdentifier{Hostname: c.hostname})
	require.NoError(t, err)
	require.Zero(t, c.step(context.Background(), now.Add(time.Minute+time.Second)))
	require.Equal(t, "enrolled", s.State)
	require.NoDirExists(t, enrollment.PhoneHomeAttemptPath(c.cfg))
	ready = false
	require.Zero(t, restart(c).step(context.Background(), now.Add(time.Hour)))
	require.Equal(t, 1, pm.starts)
	require.Equal(t, 1, posts)
	require.Equal(t, 5, gets)
	logResult(t, s, pm, posts, 1)
}

func TestKnownRejectionsRecoverWithUnchangedScopeAndDurableCooldown(t *testing.T) {
	for _, tc := range []struct {
		name  string
		code  int
		title string
	}{{"quota", 403, "your organization has reached the maximum number of private action runners"}, {"scope_race", 403, "required scope missing"}, {"auth_race", 401, "invalid auth context"}} {
		t.Run(tc.name, func(t *testing.T) {
			blocked := true
			permission := true
			posts, registrations, gets := 0, 0, 0
			c, pm, s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					gets++
					scope(w, permission)
					return
				}
				posts++
				if blocked {
					reject(w, tc.code, tc.title)
				} else {
					registrations++
					created(w)
				}
			})
			pm.onStart = func() {
				_, err := enrollment.Enroll(context.Background(), c.cfg, &enrollment.AgentIdentifier{Hostname: c.hostname})
				if blocked {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			}
			now := time.Now()
			c.step(context.Background(), now)
			require.Equal(t, 1, posts)
			c.step(context.Background(), now)
			first := attempt(t, c)
			deadline := first.NextAttemptAt
			require.True(t, deadline.Equal(now.Add(5*time.Minute)))
			// Failure quiesces startup; repeated successful scope checks and a
			// new controller do not move the durable enrollment deadline.
			for i := 1; i < 5; i++ {
				c = restart(c)
				c.step(context.Background(), now.Add(time.Duration(i)*time.Minute))
				require.Equal(t, deadline, attempt(t, c).NextAttemptAt)
				require.Equal(t, 1, pm.starts)
				require.Equal(t, 1, posts)
			}
			require.Greater(t, gets, 2)
			require.Equal(t, pb.ProcessState_STOPPED, pm.control.State)
			if tc.name != "quota" {
				permission = false
				c.step(context.Background(), deadline)
				require.Equal(t, 1, posts)
				permission = true
				deadline = deadline.Add(time.Minute)
			}
			blocked = false
			c.step(context.Background(), deadline)
			require.Equal(t, 2, posts)
			require.Equal(t, 1, registrations)
			old := enrollment.PhoneHomeOutcome{AttemptID: first.ID, EnrollmentFailure: opms.EnrollmentFailure{Category: "quota", RetrySafe: true}}
			require.ErrorIs(t, enrollment.RecordPhoneHomeOutcome(c.cfg, old), enrollment.ErrStalePhoneHomeAttempt)
			require.Zero(t, c.step(context.Background(), deadline.Add(time.Second)))
			require.Equal(t, "enrolled", s.State)
			require.Equal(t, 2, pm.starts)
			logResult(t, s, pm, posts, registrations)
		})
	}
}

func TestSafeRejectionRetryAfterSurvivesRestart(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	deadline := now.Add(time.Hour)
	posts := 0
	c, pm, s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			scope(w, true)
			return
		}
		posts++
		if posts == 1 {
			w.Header().Set("Retry-After", deadline.Format(http.TimeFormat))
			reject(w, 403, "your organization has reached the maximum number of private action runners")
		} else {
			created(w)
		}
	})
	pm.onStart = func() {
		_, err := enrollment.Enroll(context.Background(), c.cfg, &enrollment.AgentIdentifier{Hostname: c.hostname})
		if posts == 1 {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}
	c.step(context.Background(), now)
	c.step(context.Background(), now) // durable cooldown + quiescence
	require.True(t, deadline.Equal(attempt(t, c).NextAttemptAt))
	c = restart(c)
	c.step(context.Background(), deadline.Add(-time.Second))
	require.Equal(t, 1, pm.starts)
	require.Equal(t, 1, posts)
	c.step(context.Background(), deadline)
	require.Zero(t, c.step(context.Background(), deadline.Add(time.Second)))
	require.Equal(t, 2, posts)
	logResult(t, s, pm, posts, 1)
}

func TestRepeatedRejectionBackoffIsBounded(t *testing.T) {
	posts := 0
	c, pm, _ := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			scope(w, true)
			return
		}
		posts++
		reject(w, 403, "your organization has reached the maximum number of private action runners")
	})
	pm.onStart = func() {
		_, err := enrollment.Enroll(context.Background(), c.cfg, &enrollment.AgentIdentifier{Hostname: c.hostname})
		require.Error(t, err)
	}
	now := time.Now()
	c.step(context.Background(), now)
	for _, delay := range []time.Duration{5 * time.Minute, 10 * time.Minute, 20 * time.Minute, 30 * time.Minute, 30 * time.Minute} {
		c.step(context.Background(), now) // persist cooldown and quiesce
		deadline := attempt(t, c).NextAttemptAt
		require.True(t, deadline.Equal(now.Add(delay)))
		before := posts
		c = restart(c)
		c.step(context.Background(), deadline.Add(-time.Second))
		require.Equal(t, before, posts)
		c.step(context.Background(), deadline)
		require.Equal(t, before+1, posts)
		now = deadline
	}
}

func TestDialFailureAutomaticallyRecoversWithoutSubmittingFirstPOST(t *testing.T) {
	posts := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			scope(w, true)
			return
		}
		posts++
		created(w)
	})
	c, pm, s := fixture(t, handler)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	address := server.Listener.Addr().String()
	c.cfg.(model.ReaderWriter).Set("dd_url", server.URL, model.SourceAgentRuntime)
	c.endpoint = server.URL
	first := true
	pm.onStart = func() {
		if first {
			server.Close()
		}
		_, err := enrollment.Enroll(context.Background(), c.cfg, &enrollment.AgentIdentifier{Hostname: c.hostname})
		if first {
			var failure *opms.EnrollmentFailure
			require.ErrorAs(t, err, &failure)
			require.Equal(t, "not_submitted", failure.Category)
			first = false
		} else {
			require.NoError(t, err)
		}
	}
	now := time.Now()
	c.step(context.Background(), now)
	require.Zero(t, posts)
	c.step(context.Background(), now)
	deadline := attempt(t, c).NextAttemptAt
	// Restore the exact same endpoint/credential, not a config rotation.
	replacement := httptest.NewUnstartedServer(handler)
	require.NoError(t, replacement.Listener.Close())
	listener, err := net.Listen("tcp", address)
	require.NoError(t, err)
	replacement.Listener = listener
	replacement.Start()
	t.Cleanup(replacement.Close)
	c = restart(c)
	c.step(context.Background(), deadline)
	require.Equal(t, 1, posts)
	require.Zero(t, c.step(context.Background(), deadline.Add(time.Second)))
	logResult(t, s, pm, posts, 1)
}

func TestUnknownDenialsGateAnd429DoNotInventRetryGuarantees(t *testing.T) {
	for _, tc := range []struct {
		name  string
		code  int
		title string
	}{{"bare_gate_403", 403, ""}, {"unknown_403", 403, "unknown policy"}, {"bad_request", 400, "error decoding request"}, {"bare_401", 401, ""}, {"post_429", 429, ""}} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			c, pm, s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "GET" {
					scope(w, true)
					return
				}
				posts++
				w.Header().Set("Retry-After", "3600")
				if tc.title != "" {
					reject(w, tc.code, tc.title)
				} else {
					w.WriteHeader(tc.code)
				}
			})
			pm.onStart = func() {
				_, err := enrollment.Enroll(context.Background(), c.cfg, &enrollment.AgentIdentifier{Hostname: c.hostname})
				require.Error(t, err)
			}
			now := time.Now()
			c.step(context.Background(), now)
			c.step(context.Background(), now)
			for i := 1; i <= 3; i++ {
				c = restart(c)
				c.step(context.Background(), now.Add(time.Duration(i)*time.Hour))
			}
			require.Equal(t, 1, posts)
			require.Equal(t, 1, pm.starts)
			require.False(t, s.RetrySafe)
			require.Equal(t, "blocked", s.State)
			if tc.code == 429 {
				outcome, err := enrollment.ReadPhoneHomeOutcome(c.cfg, attempt(t, c))
				require.NoError(t, err)
				require.WithinDuration(t, now.Add(time.Hour), outcome.RetryAt, 3*time.Second)
			}
			// Deliberate reconciliation is a supported recovery trigger; process
			// shutdown and backend no-creation/deletion verification are required.
			require.NoError(t, enrollment.ArchivePhoneHomeAttempt(c.cfg, attempt(t, c).ID))
			require.Equal(t, 1, posts)
			logResult(t, s, pm, posts, 0)
		})
	}
}

func TestInvalidRequestNeedsDeliberateRecovery(t *testing.T) {
	blocked := true
	posts, registrations := 0, 0
	c, pm, s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			scope(w, true)
			return
		}
		posts++
		if blocked {
			reject(w, 400, "value not allowed")
			return
		}
		registrations++
		created(w)
	})
	pm.onStart = func() {
		_, err := enrollment.Enroll(context.Background(), c.cfg, &enrollment.AgentIdentifier{Hostname: c.hostname})
		if blocked {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}
	now := time.Now()
	c.step(context.Background(), now)
	c.step(context.Background(), now)
	c.step(context.Background(), now.Add(time.Hour))
	require.Equal(t, "invalid_request", s.Reason)
	require.Equal(t, 1, posts)
	require.Equal(t, 1, pm.starts)
	// Backend validation is fixed, but an operator must still confirm no
	// creation and archive the stopped attempt; scope success cannot do it.
	blocked = false
	a := attempt(t, c)
	require.NoError(t, enrollment.ArchivePhoneHomeAttempt(c.cfg, a.ID))
	c = restart(c)
	c.step(context.Background(), now.Add(2*time.Hour))
	require.Zero(t, c.step(context.Background(), now.Add(2*time.Hour+time.Second)))
	require.Equal(t, 2, posts)
	require.Equal(t, 1, registrations)
	logResult(t, s, pm, posts, registrations)
}

func TestPersistenceRecoveryDoesNotCreateAnotherRunner(t *testing.T) {
	posts := 0
	var c *controller
	c, pm, s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			scope(w, true)
			return
		}
		posts++
		require.NoError(t, os.Mkdir(c.cfg.GetString(setup.PARIdentityFilePath), 0700))
		created(w)
	})
	pm.onStart = func() {
		if posts == 0 {
			_, err := enrollment.Enroll(context.Background(), c.cfg, &enrollment.AgentIdentifier{Hostname: c.hostname})
			var pending *enrollment.PendingPersistence
			require.ErrorAs(t, err, &pending)
			require.True(t, pending.SafeToExit)
		} else {
			require.NoError(t, enrollment.RecoverPhoneHomeIdentity(context.Background(), c.cfg, c.hostname))
		}
	}
	now := time.Now()
	c.step(context.Background(), now)
	a := attempt(t, c)
	pendingBefore, err := os.ReadFile(c.cfg.GetString(setup.PARIdentityFilePath) + ".pending-" + a.ID)
	require.NoError(t, err)
	c.step(context.Background(), now)
	deadline := attempt(t, c).NextAttemptAt
	require.True(t, deadline.Equal(now.Add(time.Minute)))
	c = restart(c)
	c.step(context.Background(), deadline.Add(-time.Second))
	require.Equal(t, 1, posts)
	require.Equal(t, 1, pm.starts)
	require.NoError(t, os.Remove(c.cfg.GetString(setup.PARIdentityFilePath)))
	c.step(context.Background(), deadline)
	require.Equal(t, 2, pm.starts)
	require.Equal(t, 1, posts)
	identity, err := enrollment.GetIdentityFromPreviousEnrollment(context.Background(), c.cfg)
	require.NoError(t, err)
	var pending enrollment.PersistedIdentity
	require.NoError(t, json.Unmarshal(pendingBefore, &pending))
	require.Equal(t, pending.URN, identity.URN)
	require.Equal(t, pending.PrivateKey, identity.PrivateKey)
	require.Zero(t, c.step(context.Background(), deadline.Add(time.Second)))
	require.Equal(t, "enrolled", s.State)
	logResult(t, s, pm, posts, 1)
}

func TestLostResponseHasStableIdentityAndManualRecovery(t *testing.T) {
	posts, registrations := 0, 0
	publicPEM := ""
	c, pm, s := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			scope(w, true)
			return
		}
		posts++
		registrations++
		var request struct {
			Data struct {
				Attributes struct {
					PublicKey string `json:"public_key_pem"`
				} `json:"attributes"`
			} `json:"data"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		publicPEM = request.Data.Attributes.PublicKey
		conn, _, err := w.(http.Hijacker).Hijack()
		require.NoError(t, err)
		_ = conn.Close()
	})
	pm.onStart = func() {
		_, err := enrollment.Enroll(context.Background(), c.cfg, &enrollment.AgentIdentifier{Hostname: c.hostname})
		require.Error(t, err)
	}
	now := time.Now()
	c.step(context.Background(), now)
	a := attempt(t, c)
	path := c.cfg.GetString(setup.PARIdentityFilePath) + ".pending-" + a.ID
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		c = restart(c)
		c.step(context.Background(), now.Add(time.Duration(i+1)*time.Hour))
		_, err := enrollment.Enroll(context.Background(), c.cfg, &enrollment.AgentIdentifier{Hostname: c.hostname})
		require.Error(t, err)
	}
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.Equal(t, 1, posts)
	require.Equal(t, 1, registrations)
	require.Equal(t, 1, pm.starts)
	require.True(t, s.ReconciliationRequired)
	// Manual recovery, NOT a fake idempotent endpoint: an administrator must
	// verify the actual registered public key + runner/org ID out of band.
	var pending enrollment.PersistedIdentity
	require.NoError(t, json.Unmarshal(before, &pending))
	key, err := util.Base64ToJWK(pending.PrivateKey)
	require.NoError(t, err)
	pub := key.Public()
	pem, err := util.JWKToPEM(&pub)
	require.NoError(t, err)
	require.Equal(t, publicPEM, pem)
	require.NoError(t, enrollment.PersistIdentity(context.Background(), c.cfg, &enrollment.Result{PrivateKey: key.Key.(*ecdsa.PrivateKey), URN: util.MakeRunnerURN("us1", 42, "runner"), Hostname: c.hostname, APIKeyHash: pending.APIKeyHash}))
	require.NoError(t, enrollment.ArchivePhoneHomeAttempt(c.cfg, a.ID))
	pm.onStart = nil
	c = restart(c)
	c.step(context.Background(), now.Add(4*time.Hour))
	require.Zero(t, c.step(context.Background(), now.Add(4*time.Hour+time.Second)))
	require.Equal(t, 1, posts)
	logResult(t, s, pm, posts, registrations)
}

func TestExistingIdentityOffAndSupervisorPolicy(t *testing.T) {
	c, pm, s := fixture(t, func(http.ResponseWriter, *http.Request) { t.Error("discovery not expected") })
	key, _, err := util.GenerateKeys()
	require.NoError(t, err)
	require.NoError(t, enrollment.PersistIdentity(context.Background(), c.cfg, &enrollment.Result{PrivateKey: key.Key.(*ecdsa.PrivateKey), URN: util.MakeRunnerURN("us1", 42, "legacy")}))
	pm.control.AutoStart = true
	c.step(context.Background(), time.Now())
	require.Equal(t, "supervisor_must_be_on_demand_no_restart", s.Reason)
	require.Zero(t, pm.starts)
	pm.control.AutoStart = false
	pm.executor.Env = nil
	c.step(context.Background(), time.Now())
	require.Equal(t, "executor_must_opt_into_poc_guard", s.Reason)
	pm.executor.Env = map[string]string{app.PhoneHomePOCEnvVar: "true"}
	c.step(context.Background(), time.Now())
	require.Zero(t, restart(c).step(context.Background(), time.Now()))
	require.Equal(t, 1, pm.starts)
	require.Zero(t, pm.stops)
	for _, enabled := range []bool{false, true} {
		t.Setenv(app.PhoneHomePOCEnvVar, "false")
		cfg := configmock.New(t)
		cfg.SetInTest(setup.PAREnabled, enabled)
		Start(context.Background(), cfg, "host")
	}
	t.Setenv(app.PhoneHomePOCEnvVar, "true")
	Start(context.Background(), configmock.New(t), "host")
}

func TestExistingIdentityReenrollmentDoesNotAddScopeGate(t *testing.T) {
	for _, tc := range []struct{ name, hostname, hash string }{{"hostname", "old-host", ""}, {"api_key", "test-host", enrollment.HashAPIKey("old-key")}} {
		t.Run(tc.name, func(t *testing.T) {
			posts := 0
			c, pm, s := fixture(t, func(w http.ResponseWriter, r *http.Request) { require.Equal(t, "POST", r.Method); posts++; created(w) })
			key, _, err := util.GenerateKeys()
			require.NoError(t, err)
			require.NoError(t, enrollment.PersistIdentity(context.Background(), c.cfg, &enrollment.Result{PrivateKey: key.Key.(*ecdsa.PrivateKey), URN: util.MakeRunnerURN("us1", 42, "legacy"), Hostname: tc.hostname, APIKeyHash: tc.hash}))
			pm.onStart = func() {
				_, err := enrollment.Enroll(context.Background(), c.cfg, &enrollment.AgentIdentifier{Hostname: c.hostname})
				require.NoError(t, err)
			}
			now := time.Now()
			c.step(context.Background(), now)
			require.Zero(t, c.step(context.Background(), now.Add(time.Second)))
			require.Equal(t, 1, posts)
			logResult(t, s, pm, posts, 1)
		})
	}
}

func TestConcurrencyCancellationAndConfigurationMismatch(t *testing.T) {
	c, pm, s := fixture(t, func(w http.ResponseWriter, _ *http.Request) { scope(w, true) })
	var wg sync.WaitGroup
	now := time.Now()
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); c.step(context.Background(), now) }()
	}
	wg.Wait()
	require.Equal(t, 1, pm.starts)
	// The executor must not silently use a different enrollment credential.
	c.cfg.(model.ReaderWriter).Set("api_key", "different", model.SourceAgentRuntime)
	_, err := enrollment.Enroll(context.Background(), c.cfg, &enrollment.AgentIdentifier{Hostname: c.hostname})
	require.Error(t, err)
	c.step(context.Background(), now)
	require.Equal(t, "config_changed_restart_required", s.Reason)
	require.Equal(t, 1, pm.starts)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("wait not cancelled")
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
		t.Fatal("request not cancelled")
	}
}

func TestVisibleIdentityWithoutAcknowledgementKeepsDiscoveryAndHelper(t *testing.T) {
	checks := 0
	c, pm, s := fixture(t, func(w http.ResponseWriter, _ *http.Request) { checks++; scope(w, true) })
	now := time.Now()
	c.step(context.Background(), now)
	a := attempt(t, c)
	key, _, err := util.GenerateKeys()
	require.NoError(t, err)
	// Visible identity, but no Go fsync acknowledgement in the outcome journal.
	require.NoError(t, enrollment.PersistIdentity(context.Background(), c.cfg, &enrollment.Result{PrivateKey: key.Key.(*ecdsa.PrivateKey), URN: util.MakeRunnerURN("us1", 42, "runner")}))
	pm.executor.State = pb.ProcessState_RUNNING
	c = restart(c)
	require.Positive(t, c.step(context.Background(), now.Add(3*time.Minute)))
	require.Equal(t, 2, checks)
	require.Equal(t, "unconfirmed_executor_may_hold_identity", s.Reason)
	require.Zero(t, pm.stops)
	require.NoError(t, enrollment.RecordPhoneHomeOutcome(c.cfg, enrollment.PhoneHomeOutcome{AttemptID: a.ID, EnrollmentFailure: opms.EnrollmentFailure{Category: "identity_persisted"}}))
	require.Zero(t, c.step(context.Background(), now.Add(4*time.Minute)))
	require.Equal(t, 2, checks)
	require.Equal(t, "enrolled", s.State)
}

func TestMemoryHolderIsNotStoppedAndLossRequiresReconciliation(t *testing.T) {
	c, pm, s := fixture(t, func(w http.ResponseWriter, _ *http.Request) { scope(w, true) })
	now := time.Now()
	c.step(context.Background(), now)
	a := attempt(t, c)
	pm.executor.State = pb.ProcessState_RUNNING
	require.NoError(t, enrollment.RecordPhoneHomeOutcome(c.cfg, enrollment.PhoneHomeOutcome{AttemptID: a.ID, EnrollmentFailure: opms.EnrollmentFailure{Category: "persistence_memory"}, PersistencePending: true, HelperRetained: true}))
	c.step(context.Background(), now.Add(time.Hour))
	require.True(t, s.HelperRetained)
	require.Zero(t, pm.stops)
	pm.executor.State = pb.ProcessState_CRASHED
	c = restart(c)
	c.step(context.Background(), now.Add(2*time.Hour)) // quiesce Rust only
	c.step(context.Background(), now.Add(2*time.Hour+time.Second))
	require.Equal(t, "pending_identity_holder_lost", s.Reason)
	require.True(t, s.ReconciliationRequired)
	require.False(t, s.HelperRetained)
	require.Equal(t, 1, pm.starts)
}

func TestDiscoveryBackoffAndRetryAfter(t *testing.T) {
	code := 503
	calls := 0
	c, pm, _ := fixture(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if code == 200 {
			scope(w, true)
			return
		}
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(code)
	})
	now := time.Now()
	for _, delay := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 15 * time.Minute} {
		require.Equal(t, delay, c.step(context.Background(), now))
		now = now.Add(delay)
	}
	code = 429
	require.InDelta(t, time.Hour.Seconds(), c.step(context.Background(), now).Seconds(), 1)
	before := calls
	c.step(context.Background(), now.Add(30*time.Minute))
	require.Equal(t, before, calls)
	require.Zero(t, pm.starts)
	code = 200
	c.step(context.Background(), now.Add(time.Hour))
	require.Equal(t, before+1, calls)
	require.Equal(t, 1, pm.starts)
}
