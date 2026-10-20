// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && !windows

package phonehome

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	remote "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	par "github.com/DataDog/datadog-agent/pkg/privateactionrunner"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/procmgr"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// Real RC and AAA clients -> typed reader -> controller -> real enrollment and
// persistence. Only the supervisor and remote endpoints are fakes; no OS process
// count, RSS, or wall-clock startup latency is established by this fixture.
type httpStack struct {
	*fixture
	service                    *remote.CoreAgentService
	discovery                  *Discovery
	server                     *httptest.Server
	pollRC                     func()
	pollAAA                    func(context.Context)
	mu                         sync.Mutex
	enabled, authorized, scope bool
	rcCode, aaaCode            int
	retry                      string
	rcGETs, aaaGETs            atomic.Int32
}

func newHTTPStack(t *testing.T) *httpStack { t.Helper(); return newHTTPStackWithRCKey(t, "test-key") }
func newHTTPStackWithRCKey(t *testing.T, rcKey string) *httpStack {
	t.Helper()
	h := &httpStack{fixture: newFixture(t), rcCode: 200, aaaCode: 200}
	cfg := h.c.cfg.(model.BuildableConfig)
	postURL, err := url.Parse(cfg.GetString("dd_url"))
	require.NoError(t, err)
	proxy := httputil.NewSingleHostReverseProxy(postURL)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			proxy.ServeHTTP(w, r)
			return
		}
		expectedKey := "test-key"
		if r.URL.Path == "/api/v0.1/status" {
			expectedKey = rcKey
		}
		require.Equal(t, expectedKey, r.Header.Get("DD-API-KEY"))
		require.Empty(t, r.Header.Get("DD-APPLICATION-KEY"))
		h.mu.Lock()
		defer h.mu.Unlock()
		switch r.URL.Path {
		case "/api/v0.1/status":
			h.rcGETs.Add(1)
			w.WriteHeader(h.rcCode)
			data, err := proto.Marshal(&pbgo.OrgStatusResponse{Enabled: h.enabled, Authorized: h.authorized})
			require.NoError(t, err)
			_, _ = w.Write(data)
		case "/api/v2/validate":
			h.aaaGETs.Add(1)
			w.Header().Set("Retry-After", h.retry)
			w.WriteHeader(h.aaaCode)
			scope := "remote_config_read"
			if h.scope {
				scope = "private_action_runner_enroll"
			}
			fmt.Fprintf(w, `{"data":{"attributes":{"valid":true,"api_key_scopes":[%q]}}}`, scope)
		default:
			t.Errorf("unexpected discovery path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	h.server = server
	for k, v := range map[string]any{"dd_url": server.URL, "remote_configuration.rc_dd_url": server.URL, "remote_configuration.enabled": true, "remote_configuration.no_tls": true, "run_path": t.TempDir()} {
		cfg.SetInTest(k, v)
	}
	if rcKey != "test-key" {
		cfg.SetInTest("remote_configuration.api_key", rcKey)
	}
	service, err := remote.NewService(cfg, "test", server.URL, "host", func() []string { return nil }, nil, "test", remote.WithAPIKey(rcKey), remote.WithOrgStatusRefreshInterval(time.Minute, "test"))
	require.NoError(t, err)
	h.service = service
	t.Cleanup(func() { require.NoError(t, service.Stop()) })
	h.pollRC = func() { remote.MockPollOrgStatus(service) }
	h.discovery = NewDiscovery(cfg, service)
	if h.discovery != nil {
		h.discovery.clock = h.clock
		h.discovery.jitter = func(time.Duration) time.Duration { return 0 }
		h.discovery.next = h.clock.Now()
	}
	h.pollAAA = func(ctx context.Context) {
		if h.discovery != nil {
			h.discovery.step(ctx)
		}
	}
	h.c.source = NewSource(cfg, service, h.discovery)
	h.enroll(t)
	return h
}
func (h *httpStack) rc(enabled, authorized bool) {
	h.mu.Lock()
	h.enabled, h.authorized = enabled, authorized
	h.mu.Unlock()
	h.pollRC()
}
func (h *httpStack) grant() { h.mu.Lock(); h.scope = true; h.mu.Unlock() }
func (h *httpStack) cycle(after time.Duration) {
	h.clock.Add(after)
	h.pollRC()
	h.tick(0)
	h.pollAAA(context.Background())
	h.tick(0)
}
func (h *httpStack) evidence(t *testing.T) {
	t.Helper()
	t.Logf("status=%s reason=%s RC-owner GETs=%d AAA-owner GETs=%d Start RPCs=%d enrollment POSTs=%d registrations=%d next_discovery=%s next_enrollment=%s retained=%t reconciliation=%t coverage=loopback-HTTP+fake-supervisor", h.state.State, h.state.Reason, h.rcGETs.Load(), h.aaaGETs.Load(), h.pm.starts, h.posts, h.registrations, h.state.NextDiscovery, h.state.NextEnrollmentAttempt, h.state.HelperRetained, h.state.ReconciliationRequired)
}

func TestHTTPStackClosedRCGate(t *testing.T) {
	for _, existing := range []bool{false, true} {
		for _, condition := range []string{"false_false", "false_true", "true_false", "unknown", "failed", "absent", "local_off", "endpoint", "off", "poc_off"} {
			t.Run(fmt.Sprintf("%t_%s", existing, condition), func(t *testing.T) {
				h := newHTTPStack(t)
				h.grant()
				if existing {
					h.identity(t)
				}
				switch condition {
				case "false_false":
					h.rc(false, false)
				case "false_true":
					h.rc(false, true)
				case "true_false":
					h.rc(true, false)
				case "failed":
					h.mu.Lock()
					h.rcCode = 503
					h.mu.Unlock()
					h.rc(true, true)
				case "absent":
					h.c.source = NewSource(h.c.cfg, nil, nil)
				case "local_off":
					h.c.cfg.(model.BuildableConfig).SetInTest("remote_configuration.enabled", false)
				case "endpoint":
					h.rc(true, true)
					h.c.cfg.(model.BuildableConfig).SetInTest("dd_url", "http://127.0.0.1:1")
				case "off":
					h.c.cfg.(model.BuildableConfig).SetInTest(par.Enabled, false)
				case "poc_off":
					h.rc(true, true)
					t.Setenv("DD_PAR_PHONE_HOME_POC", "false")
				}
				h.tick(0)
				h.pollAAA(context.Background())
				h.tick(0)
				require.Zero(t, h.aaaGETs.Load())
				require.Zero(t, h.pm.starts)
				require.Zero(t, h.posts)
				h.evidence(t)
			})
		}
	}
}

func TestHTTPStackUsesLastSuccessfulRCResponse(t *testing.T) {
	h := newHTTPStack(t)
	h.grant()
	h.rc(true, true)
	h.mu.Lock()
	h.rcCode = 503
	h.mu.Unlock()
	h.cycle(time.Hour)
	require.EqualValues(t, 1, h.aaaGETs.Load())
	require.Equal(t, 1, h.pm.starts)
	require.Equal(t, 1, h.posts)
	require.Zero(t, h.tick(time.Second))
	h.evidence(t)
}

func TestHTTPStackDifferentRCCredentialAndCachedGrant(t *testing.T) {
	t.Run("different-key", func(t *testing.T) {
		h := newHTTPStackWithRCKey(t, "other-rc-key")
		h.grant()
		h.rc(true, true)
		h.cycle(0)
		require.Zero(t, h.aaaGETs.Load())
		require.Zero(t, h.pm.starts)
		require.Zero(t, h.posts)
		h.evidence(t)
	})
	t.Run("cached-grant", func(t *testing.T) {
		h := newHTTPStack(t)
		h.grant()
		h.rc(true, true)
		h.discovery.SetPARDiscoveryDemand(true)
		h.pollAAA(context.Background())
		require.EqualValues(t, 1, h.aaaGETs.Load())
		h.rc(false, true)
		h.tick(0)
		h.pollAAA(context.Background())
		require.EqualValues(t, 1, h.aaaGETs.Load())
		require.Zero(t, h.pm.starts)
		require.Zero(t, h.posts)
		h.evidence(t)
	})
}

func TestHTTPStackGrantThenReleaseDemand(t *testing.T) {
	h := newHTTPStack(t)
	h.rc(false, true)
	h.cycle(0)
	require.Zero(t, h.aaaGETs.Load())
	h.rc(true, true)
	h.cycle(0)
	require.EqualValues(t, 1, h.aaaGETs.Load())
	require.Zero(t, h.pm.starts)
	h.grant()
	h.cycle(time.Minute)
	require.Equal(t, 1, h.pm.starts)
	require.Equal(t, 1, h.posts)
	require.Equal(t, 1, h.registrations)
	require.Zero(t, h.tick(time.Second))
	beforeRC, beforeAAA := h.rcGETs.Load(), h.aaaGETs.Load()
	h.cycle(time.Hour)
	require.Greater(t, h.rcGETs.Load(), beforeRC)
	require.Equal(t, beforeAAA, h.aaaGETs.Load())
	require.Equal(t, "enrolled", h.state.State)
	h.evidence(t)
}

func TestHTTPStackRejectionCooldownAndFreshRevalidation(t *testing.T) {
	for _, kind := range []string{"quota", "scope", "auth"} {
		t.Run(kind, func(t *testing.T) {
			h := newHTTPStack(t)
			h.behavior = kind
			h.grant()
			h.rc(true, true)
			h.cycle(0)
			h.tick(time.Second)
			deadline := h.attempt(t).NextAttemptAt
			h.restart()
			for range 3 {
				h.cycle(time.Minute)
				require.Equal(t, 1, h.pm.starts)
				require.Equal(t, 1, h.posts)
				require.True(t, deadline.Equal(h.attempt(t).NextAttemptAt))
			}
			h.rc(false, true)
			h.cycle(time.Minute)
			require.Equal(t, 1, h.posts)
			h.behavior = ""
			h.rc(true, true)
			h.cycle(deadline.Sub(h.clock.Now()) + time.Second)
			require.Equal(t, 2, h.posts)
			require.Equal(t, 1, h.registrations)
			require.Zero(t, h.tick(time.Second))
			h.evidence(t)
		})
	}
}

func TestHTTPStack429AndTransientDiscovery(t *testing.T) {
	for _, hint := range []string{"3600", "date", "", "malformed"} {
		t.Run(hint, func(t *testing.T) {
			h := newHTTPStack(t)
			h.grant()
			h.rc(true, true)
			h.mu.Lock()
			h.aaaCode = 429
			h.retry = hint
			if hint == "date" {
				h.retry = h.clock.Now().Add(time.Hour).UTC().Format(http.TimeFormat)
			}
			h.mu.Unlock()
			h.cycle(0)
			deadline := h.discovery.GetPARScope().NextCheck
			require.Zero(t, h.posts)
			h.rc(false, true)
			h.rc(true, true)
			h.cycle(10 * time.Second)
			require.EqualValues(t, 1, h.aaaGETs.Load())
			require.True(t, deadline.Equal(h.discovery.GetPARScope().NextCheck))
			h.mu.Lock()
			h.aaaCode = 503
			h.mu.Unlock()
			h.cycle(deadline.Sub(h.clock.Now()))
			require.Zero(t, h.posts)
			deadline = h.discovery.GetPARScope().NextCheck
			h.mu.Lock()
			h.aaaCode = 200
			h.mu.Unlock()
			h.cycle(deadline.Sub(h.clock.Now()))
			require.Equal(t, 1, h.posts)
			h.tick(time.Second)
			h.evidence(t)
		})
	}
}

func TestHTTPStackRefusedEnrollmentConnectionRecovers(t *testing.T) {
	h := newHTTPStack(t)
	h.grant()
	h.rc(true, true)
	h.tick(0)
	h.pollAAA(context.Background())
	addr, handler := h.server.Listener.Addr().String(), h.server.Config.Handler
	h.server.Close()
	h.pm.onStart = func() {
		_, err := enrollment.Enroll(context.Background(), h.c.cfg, &enrollment.AgentIdentifier{Hostname: h.c.hostname})
		require.Error(t, err)
	}
	h.tick(0)
	h.tick(time.Second)
	require.Zero(t, h.posts)
	require.Equal(t, "not_submitted", h.state.EnrollmentOutcome)
	listener, err := net.Listen("tcp", addr)
	require.NoError(t, err)
	restored := httptest.NewUnstartedServer(handler)
	restored.Listener = listener
	restored.Start()
	t.Cleanup(restored.Close)
	h.enroll(t)
	h.restart()
	h.cycle(6 * time.Minute)
	require.Equal(t, 1, h.posts)
	require.Equal(t, 1, h.registrations)
	h.tick(time.Second)
	h.evidence(t)
}

func TestHTTPStackDenialLostResponseAndRestart(t *testing.T) {
	for _, kind := range []string{"unknown", "bad_input", "throttle", "lost"} {
		t.Run(kind, func(t *testing.T) {
			h := newHTTPStack(t)
			h.behavior = kind
			h.grant()
			h.rc(true, true)
			h.cycle(0)
			h.tick(time.Second)
			a := h.attempt(t)
			path := h.c.cfg.GetString(par.IdentityFilePath) + ".pending-" + a.ID
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			for range 3 {
				h.restart()
				h.cycle(time.Hour)
			}
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.Equal(t, 1, h.posts)
			require.Equal(t, 1, h.pm.starts)
			if kind == "lost" {
				require.Equal(t, 1, h.registrations)
				require.True(t, h.state.ReconciliationRequired)
			} else {
				require.Zero(t, h.registrations)
			}
			h.evidence(t)
			if kind == "lost" {
				// Manual, out-of-band reconciliation using the backend's recorded key;
				// this is not an invented idempotency or lookup endpoint.
				h.pm.control.State = pb.ProcessState_STOPPED
				h.pm.executor.State = pb.ProcessState_STOPPED
				var pending enrollment.PersistedIdentity
				require.NoError(t, json.Unmarshal(before, &pending))
				key, err := util.Base64ToJWK(pending.PrivateKey)
				require.NoError(t, err)
				pub := key.Public()
				pem, err := util.JWKToPEM(&pub)
				require.NoError(t, err)
				require.Equal(t, h.publicPEM, pem)
				require.NoError(t, enrollment.PersistIdentity(context.Background(), h.c.cfg, &enrollment.Result{PrivateKey: key.Key.(*ecdsa.PrivateKey), URN: util.MakeRunnerURN("us1", 42, "runner"), Hostname: h.c.hostname, APIKeyHash: pending.APIKeyHash}))
				require.NoError(t, enrollment.ArchivePhoneHomeAttempt(h.c.cfg, a.ID))
				h.pm.onStart = nil
				h.restart()
				h.cycle(time.Minute)
				require.Zero(t, h.tick(time.Second))
				require.Equal(t, 1, h.posts)
			}
		})
	}
}

func TestHTTPStackStorageRecoveryAndEnrolledRestart(t *testing.T) {
	t.Run("storage", func(t *testing.T) {
		h := newHTTPStack(t)
		h.behavior = "storage"
		h.grant()
		h.rc(true, true)
		h.cycle(0)
		h.tick(time.Second)
		h.rc(false, true)
		h.restart()
		h.cycle(2 * time.Minute)
		require.Equal(t, 1, h.pm.starts)
		require.Equal(t, 1, h.posts)
		require.NoError(t, os.Remove(h.c.cfg.GetString(par.IdentityFilePath)))
		h.pm.onStart = func() {
			require.NoError(t, enrollment.RecoverPhoneHomeIdentity(context.Background(), h.c.cfg, h.c.hostname))
		}
		h.mu.Lock()
		h.scope = false
		h.mu.Unlock()
		h.rc(true, true)
		h.cycle(time.Minute)
		require.Equal(t, 2, h.pm.starts)
		require.Zero(t, h.tick(time.Second))
		require.Equal(t, 1, h.posts)
		h.evidence(t)
	})
	t.Run("enrolled", func(t *testing.T) {
		h := newHTTPStack(t)
		h.identity(t)
		h.pm.onStart = nil
		h.rc(false, true)
		h.cycle(0)
		require.Zero(t, h.pm.starts)
		h.rc(true, true)
		h.cycle(time.Minute)
		require.Equal(t, 1, h.pm.starts)
		require.Zero(t, h.aaaGETs.Load())
		require.Zero(t, h.posts)
		h.rc(false, true)
		h.cycle(time.Minute)
		require.Zero(t, h.pm.stops)
		h.evidence(t)
	})
}

func TestHTTPStackConcurrentLaunches(t *testing.T) {
	h := newHTTPStack(t)
	h.grant()
	h.rc(true, true)
	h.tick(0)
	h.pollAAA(context.Background())
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); h.c.step(context.Background(), h.clock.Now()) }()
	}
	wg.Wait()
	require.Equal(t, 1, h.pm.starts)
	require.Equal(t, 1, h.posts)
	require.Equal(t, 1, h.registrations)
	h.evidence(t)
}
