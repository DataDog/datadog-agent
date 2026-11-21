// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rcserviceimpl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	delegatedauthmock "github.com/DataDog/datadog-agent/comp/core/delegatedauth/mock"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type noopTelemetryReporter struct{}

func (noopTelemetryReporter) IncTimeout()                                {}
func (noopTelemetryReporter) IncRateLimit()                              {}
func (noopTelemetryReporter) IncConfigSubscriptionsConnectedCounter()    {}
func (noopTelemetryReporter) IncConfigSubscriptionsDisconnectedCounter() {}
func (noopTelemetryReporter) SetConfigSubscriptionsActive(int)           {}
func (noopTelemetryReporter) SetConfigSubscriptionClientsTracked(int)    {}

type testLifecycle struct {
	hooks []compdef.Hook
}

func (l *testLifecycle) Append(h compdef.Hook) {
	l.hooks = append(l.hooks, h)
}

func (l *testLifecycle) start(t *testing.T) {
	t.Helper()
	for _, h := range l.hooks {
		if h.OnStart != nil {
			require.NoError(t, h.OnStart(context.Background()))
		}
	}
}

func (l *testLifecycle) stop(t *testing.T) {
	t.Helper()
	for i := len(l.hooks) - 1; i >= 0; i-- {
		if l.hooks[i].OnStop != nil {
			require.NoError(t, l.hooks[i].OnStop(context.Background()))
		}
	}
}

func TestRCServiceWorkloadTokenWiring(t *testing.T) {
	newDeps := func(t *testing.T, serverURL string, da *delegatedauthmock.Mock, extraCfg func(config.Component), params *rcservice.Params) (Dependencies, *testLifecycle) {
		t.Helper()
		cfg := config.NewMock(t)
		cfg.SetInTest("remote_configuration.enabled", true)
		cfg.SetInTest("remote_configuration.rc_dd_url", serverURL)
		cfg.SetInTest("remote_configuration.no_tls", true)
		cfg.SetInTest("remote_configuration.no_tls_validation", true)
		cfg.SetInTest("api_key", "managed-intake-key")
		cfg.SetInTest("run_path", t.TempDir())
		if extraCfg != nil {
			extraCfg(cfg)
		}
		lc := &testLifecycle{}
		deps := Dependencies{
			Lc:                    lc,
			Params:                params,
			DdRcTelemetryReporter: noopTelemetryReporter{},
			Hostname:              hostnameimpl.NewHostnameService(),
			Cfg:                   cfg,
			Logger:                logmock.New(t),
			Tagger:                option.None[tagger.Component](),
		}
		if da != nil {
			deps.DelegatedAuth = da
		}
		return deps, lc
	}

	t.Run("requests remote_config workload token when api_key is managed by delegatedauth", func(t *testing.T) {
		var mu sync.Mutex
		var sawWorkloadHeader bool
		headerCh := make(chan string, 1)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tok := r.Header.Get("DD-Workload-Token"); tok != "" {
				mu.Lock()
				if !sawWorkloadHeader {
					sawWorkloadHeader = true
					headerCh <- tok
				}
				mu.Unlock()
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		reqCh := make(chan common.WorkloadTokenRequest, 1)
		da := &delegatedauthmock.Mock{
			IsInstanceManagedFunc: func(key string) bool { return key == "api_key" },
			GetWorkloadTokenWithExpiryFunc: func(_ context.Context, key string, req common.WorkloadTokenRequest) (common.WorkloadToken, error) {
				require.Equal(t, "api_key", key)
				select {
				case reqCh <- req:
				default:
				}
				return common.WorkloadToken{
					Token:     "rc-workload-token",
					ExpiresAt: time.Now().Add(time.Hour),
				}, nil
			},
		}

		deps, lc := newDeps(t, ts.URL, da, nil, &rcservice.Params{
			Options: []remoteconfig.Option{remoteconfig.WithAgentPollLoopDisabled()},
		})
		provides := NewComponent(deps)
		_, ok := provides.Comp.Get()
		require.True(t, ok)

		lc.start(t)
		t.Cleanup(func() { lc.stop(t) })

		select {
		case req := <-reqCh:
			require.Equal(t, []string{"remote_config"}, req.Grants)
			require.Empty(t, req.JWKThumbprint)
		case <-time.After(5 * time.Second):
			t.Fatal("expected rcservice to request a remote_config workload token")
		}

		select {
		case got := <-headerCh:
			require.Equal(t, "rc-workload-token", got)
		case <-time.After(5 * time.Second):
			t.Fatal("expected rcservice HTTP requests to include DD-Workload-Token")
		}
	})

	t.Run("skips workload token when remote_configuration.api_key is configured", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		called := make(chan struct{}, 1)
		da := &delegatedauthmock.Mock{
			IsInstanceManagedFunc: func(string) bool { return true },
			GetWorkloadTokenWithExpiryFunc: func(context.Context, string, common.WorkloadTokenRequest) (common.WorkloadToken, error) {
				called <- struct{}{}
				return common.WorkloadToken{}, nil
			},
		}

		deps, lc := newDeps(t, ts.URL, da, func(cfg config.Component) {
			cfg.SetInTest("remote_configuration.api_key", "static-rc-override")
		}, &rcservice.Params{
			Options: []remoteconfig.Option{remoteconfig.WithAgentPollLoopDisabled()},
		})
		provides := NewComponent(deps)
		_, ok := provides.Comp.Get()
		require.True(t, ok)

		lc.start(t)
		lc.stop(t)

		select {
		case <-called:
			t.Fatal("workload token must not be requested when remote_configuration.api_key is configured")
		default:
		}
	})

	t.Run("skips workload token when PAR JWT is supplied via Params", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		called := make(chan struct{}, 1)
		da := &delegatedauthmock.Mock{
			IsInstanceManagedFunc: func(string) bool { return true },
			GetWorkloadTokenWithExpiryFunc: func(context.Context, string, common.WorkloadTokenRequest) (common.WorkloadToken, error) {
				called <- struct{}{}
				return common.WorkloadToken{}, nil
			},
		}

		deps, lc := newDeps(t, ts.URL, da, nil, &rcservice.Params{
			Options: []remoteconfig.Option{
				remoteconfig.WithAgentPollLoopDisabled(),
				remoteconfig.WithPARJWT("par-jwt-token"),
			},
		})
		provides := NewComponent(deps)
		_, ok := provides.Comp.Get()
		require.True(t, ok)

		lc.start(t)
		lc.stop(t)

		select {
		case <-called:
			t.Fatal("workload token must not be requested when PAR JWT is configured")
		default:
		}
	})
}
