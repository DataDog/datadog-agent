// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

const grantedScopeJSON = `{"data":{"attributes":{"valid":true,"api_key_scopes":["private_action_runner_enroll"]}}}`

type scopeFixture struct {
	clock               *clock.Mock
	s                   *CoreAgentService
	cfg                 model.BuildableConfig
	mu                  sync.Mutex
	enabled, authorized bool
	rcCode, aaaCode     int
	body, retryAfter    string
	rcGETs, aaaGETs     atomic.Int32
}

func newScopeFixture(t *testing.T) *scopeFixture {
	t.Helper()
	t.Setenv("DD_PAR_PHONE_HOME_POC", "true")
	t.Setenv("DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS", "true")
	f := &scopeFixture{clock: clock.NewMock(), rcCode: 200, aaaCode: 200, body: grantedScopeJSON}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "GET", r.Method)
		require.Equal(t, "scope-test-key", r.Header.Get("DD-API-KEY"))
		require.Empty(t, r.Header.Get("DD-APPLICATION-KEY"))
		require.Empty(t, r.Header.Get("DD-PAR-JWT"))
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case "/api/v0.1/status":
			f.rcGETs.Add(1)
			w.WriteHeader(f.rcCode)
			body, err := proto.Marshal(&pbgo.OrgStatusResponse{Enabled: f.enabled, Authorized: f.authorized})
			require.NoError(t, err)
			_, _ = w.Write(body)
		case "/api/v2/validate":
			f.aaaGETs.Add(1)
			w.Header().Set("Retry-After", f.retryAfter)
			w.WriteHeader(f.aaaCode)
			_, _ = io.WriteString(w, f.body)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(server.Close)
	cfg := configmock.New(t)
	cfg.SetInTest("remote_configuration.no_tls", true)
	f.cfg = cfg
	for k, v := range map[string]any{"api_key": "scope-test-key", "site": "datadoghq.com", "remote_configuration.enabled": true, "private_action_runner.enabled": true, "dd_url": server.URL, "remote_configuration.rc_dd_url": server.URL, "run_path": t.TempDir()} {
		cfg.SetInTest(k, v)
	}
	store := &mockCoreAgentUptane{}
	store.On("StoredOrgUUID").Return("", errors.New("test stops before org UUID request"))
	s, err := NewService(cfg, "test", server.URL, "host", getHostTags, &telemetryReporter{}, agentVersion, WithAPIKey("scope-test-key"), WithPARScopeDiscovery(), WithOrgStatusRefreshInterval(time.Minute, "test"), uptaneFactoryOption(store))
	require.NoError(t, err)
	require.NotNil(t, s.parScopePoller)
	f.s = s
	s.clock = f.clock
	s.parScopePoller.clock = f.clock
	s.parScopePoller.jitter = func(time.Duration) time.Duration { return 0 }
	s.parScopePoller.next = f.clock.Now()
	t.Cleanup(func() { require.NoError(t, s.Stop()) })
	return f
}
func (f *scopeFixture) rc(enabled, authorized bool) {
	f.mu.Lock()
	f.enabled, f.authorized = enabled, authorized
	f.mu.Unlock()
	f.s.orgStatusPoller.poll(f.s.api, f.s.rcType)
}
func (f *scopeFixture) step()                   { f.s.parScopePoller.step(context.Background()) }
func (f *scopeFixture) advance(d time.Duration) { f.clock.Add(d); f.rc(true, true) }

func TestPARDiscoveryRequiresBothRealRCBooleans(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, authorized := range []bool{false, true} {
			t.Run(fmt.Sprintf("%t_%t", enabled, authorized), func(t *testing.T) {
				f := newScopeFixture(t)
				f.s.SetPARDiscoveryDemand(true)
				f.step()
				require.Zero(t, f.aaaGETs.Load(), "unknown RC is not readiness")
				f.rc(enabled, authorized)
				f.step()
				if enabled && authorized {
					require.EqualValues(t, 1, f.aaaGETs.Load())
					require.True(t, f.s.GetPARScope().Granted(f.clock.Now()))
				} else {
					require.Zero(t, f.aaaGETs.Load())
					f.rc(true, true)
					f.step()
					require.EqualValues(t, 1, f.aaaGETs.Load())
				}
				f.s.SetPARDiscoveryDemand(false)
				f.advance(time.Hour)
				f.step()
				require.EqualValues(t, 1, f.aaaGETs.Load(), "released/enrolled demand cannot keep polling")
			})
		}
	}
}

func TestPARDiscoveryCadenceAndBackoffSurviveGatePause(t *testing.T) {
	f := newScopeFixture(t)
	f.s.SetPARDiscoveryDemand(true)
	f.rc(true, true)
	f.mu.Lock()
	f.body = `{"data":{"attributes":{"valid":true,"api_key_scopes":["remote_config_read"]}}}`
	f.mu.Unlock()
	f.step()
	require.Equal(t, "missing_enrollment_scope", f.s.GetPARScope().Reason)
	for range 20 {
		f.step()
	}
	require.EqualValues(t, 1, f.aaaGETs.Load())
	f.mu.Lock()
	f.aaaCode = 503
	f.mu.Unlock()
	for _, delay := range []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 15 * time.Minute, 15 * time.Minute} {
		f.advance(f.s.GetPARScope().NextCheck.Sub(f.clock.Now()))
		f.step()
		next := f.s.GetPARScope().NextCheck
		require.Equal(t, delay, next.Sub(f.clock.Now()))
		count := f.aaaGETs.Load()
		f.rc(false, true)
		f.s.SetPARDiscoveryDemand(false)
		f.s.SetPARDiscoveryDemand(true)
		f.rc(true, true)
		f.step()
		require.Equal(t, next, f.s.GetPARScope().NextCheck)
		require.Equal(t, count, f.aaaGETs.Load())
	}
	f.mu.Lock()
	f.aaaCode = 429
	f.retryAfter = "3600"
	f.mu.Unlock()
	f.advance(f.s.GetPARScope().NextCheck.Sub(f.clock.Now()))
	f.step()
	deadline := f.s.GetPARScope().NextCheck
	require.Equal(t, time.Hour, deadline.Sub(f.clock.Now()))
	for range 5 {
		f.rc(false, false)
		f.rc(true, true)
		f.step()
	}
	require.Equal(t, deadline, f.s.GetPARScope().NextCheck)
	f.mu.Lock()
	f.aaaCode = 200
	f.body = grantedScopeJSON
	f.mu.Unlock()
	f.advance(time.Hour)
	f.step()
	require.True(t, f.s.GetPARScope().Granted(f.clock.Now()))
	require.Zero(t, f.s.parScopePoller.backoff)
}

func TestPARDiscoveryRejectsConfigMismatchAndLocalDisable(t *testing.T) {
	for _, scenario := range []string{"key", "endpoint", "local-off", "poc-off", "par-off"} {
		t.Run(scenario, func(t *testing.T) {
			f := newScopeFixture(t)
			f.s.SetPARDiscoveryDemand(true)
			f.rc(true, true)
			f.step()
			switch scenario {
			case "key":
				f.cfg.SetInTest("api_key", "different")
			case "endpoint":
				f.cfg.SetInTest("dd_url", "http://127.0.0.1:1")
			case "local-off":
				f.cfg.SetInTest("remote_configuration.enabled", false)
			case "poc-off":
				t.Setenv("DD_PAR_PHONE_HOME_POC", "false")
			case "par-off":
				f.cfg.SetInTest("private_action_runner.enabled", false)
			}
			require.False(t, f.s.GetPARScope().Granted(f.clock.Now()))
			f.clock.Add(time.Hour)
			f.step()
			require.EqualValues(t, 1, f.aaaGETs.Load())
		})
	}
}

type scopeTransport func(*http.Request) (*http.Response, error)

func (f scopeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPARDiscoveryFencesLateResponseAndCancels(t *testing.T) {
	for _, change := range []string{"closed", "credential", "release", "disable", "shutdown"} {
		t.Run(change, func(t *testing.T) {
			f := newScopeFixture(t)
			f.s.SetPARDiscoveryDemand(true)
			f.rc(true, true)
			entered, release, done, canceled := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
			f.s.parScopePoller.client = &http.Client{Transport: scopeTransport(func(r *http.Request) (*http.Response, error) {
				close(entered)
				<-r.Context().Done()
				close(canceled)
				<-release
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(grantedScopeJSON)), Request: r}, nil
			})}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() { f.s.parScopePoller.step(ctx); close(done) }()
			<-entered
			switch change {
			case "closed":
				f.rc(false, true)
			case "credential":
				f.cfg.SetInTest("remote_configuration.api_key", "other")
			case "release":
				f.s.SetPARDiscoveryDemand(false)
			case "disable":
				f.cfg.SetInTest("private_action_runner.enabled", false)
			case "shutdown":
				cancel()
			}
			f.clock.Add(time.Second)
			select {
			case <-canceled:
			case <-time.After(time.Second):
				t.Fatal("in-flight AAA work not canceled")
			}
			if change == "release" {
				f.s.SetPARDiscoveryDemand(true)
			}
			f.rc(true, true)
			close(release)
			<-done
			require.False(t, f.s.GetPARScope().Granted(f.clock.Now()), "even a transport that ignores cancellation cannot publish a late grant")
		})
	}
}

func TestPARDiscoveryRechecksAtHTTPDispatch(t *testing.T) {
	f := newScopeFixture(t)
	f.s.SetPARDiscoveryDemand(true)
	f.rc(true, true)
	var reads atomic.Int32
	f.s.parScopePoller.status = func() *pbgo.OrgStatusResponse {
		if reads.Add(1) == 2 {
			f.rc(false, true)
		}
		return f.s.GetOrgStatus()
	}
	f.step()
	require.Zero(t, f.aaaGETs.Load())
	require.False(t, f.s.GetPARScope().Granted(f.clock.Now()))
}

func TestPARDiscoveryLifecycleAndScopeInvalidation(t *testing.T) {
	f := newScopeFixture(t)
	f.s.SetPARDiscoveryDemand(true)
	f.rc(true, true)
	f.step()
	previous := f.s.GetPARScope()
	require.True(t, previous.Granted(f.clock.Now()))
	f.rc(false, true)
	require.False(t, f.s.GetPARScope().Granted(f.clock.Now()))
	f.rc(true, true)
	require.False(t, f.s.GetPARScope().Granted(f.clock.Now()))
	f.s.parScopePoller.start()
	f.s.parScopePoller.stop()
	f.s.SetPARDiscoveryDemand(true)
	require.False(t, f.s.GetPARScope().Granted(f.clock.Now()))
	require.EqualValues(t, 1, f.aaaGETs.Load())
}

func TestPARDiscoveryHugeHintAndStrictPayload(t *testing.T) {
	f := newScopeFixture(t)
	f.s.SetPARDiscoveryDemand(true)
	f.rc(true, true)
	f.mu.Lock()
	f.aaaCode = 429
	f.retryAfter = "9223372036854775807"
	f.mu.Unlock()
	f.s.parScopePoller.jitter = func(d time.Duration) time.Duration { return d }
	f.step()
	require.Greater(t, f.s.GetPARScope().NextCheck.Sub(f.clock.Now()), 200*365*24*time.Hour)
	for _, body := range []string{grantedScopeJSON + "garbage", strings.Repeat(" ", 1<<20) + grantedScopeJSON} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, body) }))
		result := validate(context.Background(), server.Client(), server.URL, "key", time.Now())
		server.Close()
		require.False(t, result.eligible)
		require.Equal(t, "invalid_discovery_response", result.reason)
	}
}
