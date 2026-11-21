// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package service

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	delegatedauthcommon "github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type fakeWorkloadTokenProvider struct {
	mu      sync.Mutex
	managed bool
	fn      func(context.Context, string, delegatedauthcommon.WorkloadTokenRequest) (delegatedauthcommon.WorkloadToken, error)
	calls   int
}

func (f *fakeWorkloadTokenProvider) IsInstanceManaged(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return key == delegatedAuthPrimaryAPIKey && f.managed
}

func (f *fakeWorkloadTokenProvider) setManaged(v bool) {
	f.mu.Lock()
	f.managed = v
	f.mu.Unlock()
}

func (f *fakeWorkloadTokenProvider) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeWorkloadTokenProvider) GetWorkloadTokenWithExpiry(ctx context.Context, key string, req delegatedauthcommon.WorkloadTokenRequest) (delegatedauthcommon.WorkloadToken, error) {
	f.mu.Lock()
	f.calls++
	fn := f.fn
	f.mu.Unlock()
	return fn(ctx, key, req)
}

type recordingTokenAPI struct {
	mu     sync.Mutex
	tokens []string
	notify chan string
}

func newRecordingTokenAPI() *recordingTokenAPI {
	return &recordingTokenAPI{notify: make(chan string, 32)}
}

func (a *recordingTokenAPI) Fetch(context.Context, *pbgo.LatestConfigsRequest) (*pbgo.LatestConfigsResponse, error) {
	return &pbgo.LatestConfigsResponse{}, nil
}

func (a *recordingTokenAPI) FetchOrgData(context.Context) (*pbgo.OrgDataResponse, error) {
	return &pbgo.OrgDataResponse{}, nil
}

func (a *recordingTokenAPI) FetchOrgStatus(context.Context) (*pbgo.OrgStatusResponse, error) {
	return &pbgo.OrgStatusResponse{}, nil
}

func (a *recordingTokenAPI) UpdatePARJWT(string) {}
func (a *recordingTokenAPI) UpdateAPIKey(string) {}

func (a *recordingTokenAPI) UpdateWorkloadToken(token string) {
	a.mu.Lock()
	a.tokens = append(a.tokens, token)
	a.mu.Unlock()
	a.notify <- token
}

func (a *recordingTokenAPI) waitToken(t *testing.T) string {
	t.Helper()
	select {
	case tok := <-a.notify:
		return tok
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for UpdateWorkloadToken")
		return ""
	}
}

func TestWorkloadTokenRefresherProactiveRefreshAndExpiry(t *testing.T) {
	clk := clock.NewMock()
	clk.Set(time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC))

	var seq atomic.Int32
	provider := &fakeWorkloadTokenProvider{
		managed: true,
		fn: func(_ context.Context, key string, req delegatedauthcommon.WorkloadTokenRequest) (delegatedauthcommon.WorkloadToken, error) {
			require.Equal(t, "api_key", key)
			require.Equal(t, []string{"remote_config"}, req.Grants)
			require.Empty(t, req.JWKThumbprint)
			switch seq.Add(1) {
			case 1:
				// 60m server TTL -> refreshes at 45m (75%) with 0 jitter.
				return delegatedauthcommon.WorkloadToken{
					Token:     "wt-60m",
					ExpiresAt: clk.Now().Add(60 * time.Minute),
				}, nil
			case 2:
				// Server dynamically shortens TTL to 5m -> refreshes at 3m45s without Agent restart.
				return delegatedauthcommon.WorkloadToken{
					Token:     "wt-5m",
					ExpiresAt: clk.Now().Add(5 * time.Minute),
				}, nil
			default:
				// Missing expiry -> uses fallback 4m refresh interval.
				return delegatedauthcommon.WorkloadToken{
					Token: "wt-fallback",
				}, nil
			}
		},
	}

	apiClient := newRecordingTokenAPI()
	var presenceChanges atomic.Int32
	scheduled := make(chan time.Duration, 16)
	r := newWorkloadTokenRefresher(provider)
	r.jitter = func(time.Duration) time.Duration { return 0 }
	r.onTimerScheduled = func(d time.Duration) { scheduled <- d }

	r.start(clk, apiClient, "Remote Config", func() { presenceChanges.Add(1) })
	defer r.stop()

	// Initial acquisition triggers presence change and schedules 45m refresh (75% of 60m).
	require.Equal(t, "wt-60m", apiClient.waitToken(t))
	require.Equal(t, 45*time.Minute, <-scheduled)
	require.Equal(t, int32(1), presenceChanges.Load())

	// Advance just shy of 45m: no refresh yet.
	clk.Add(44*time.Minute + 59*time.Second)
	require.Equal(t, 1, provider.callCount())

	// At 45m: proactive refresh fires and adapts to new 5m server TTL (3m45s).
	clk.Add(time.Second)
	require.Equal(t, "wt-5m", apiClient.waitToken(t))
	require.Equal(t, 3*time.Minute+45*time.Second, <-scheduled)
	// Token stayed present, so no spurious presence-change trigger.
	require.Equal(t, int32(1), presenceChanges.Load())

	// Advance to 3m45s (75% of 5m): proactive refresh fires; missing expiry uses 4m fallback.
	clk.Add(3*time.Minute + 44*time.Second)
	require.Equal(t, 2, provider.callCount())
	clk.Add(time.Second)
	require.Equal(t, "wt-fallback", apiClient.waitToken(t))
	require.Equal(t, 4*time.Minute, <-scheduled)

	// Fallback interval is 4m.
	clk.Add(3*time.Minute + 59*time.Second)
	require.Equal(t, 3, provider.callCount())
	clk.Add(time.Second)
	require.Equal(t, "wt-fallback", apiClient.waitToken(t))
	require.Equal(t, 4*time.Minute, <-scheduled)
}

func TestWorkloadTokenRefresherDeniedClearsAndRecoversWithoutRestart(t *testing.T) {
	clk := clock.NewMock()
	clk.Set(time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC))

	var seq atomic.Int32
	provider := &fakeWorkloadTokenProvider{
		managed: true,
		fn: func(context.Context, string, delegatedauthcommon.WorkloadTokenRequest) (delegatedauthcommon.WorkloadToken, error) {
			switch seq.Add(1) {
			case 1:
				return delegatedauthcommon.WorkloadToken{
					Token:     "wt-initial",
					ExpiresAt: clk.Now().Add(60 * time.Minute),
				}, nil
			case 2, 3:
				// Mapping has remote_config disabled -> 403.
				return delegatedauthcommon.WorkloadToken{}, &delegatedauthcommon.WorkloadTokenHTTPError{
					StatusCode: http.StatusForbidden,
					Detail:     "Workload grant denied",
				}
			default:
				// Mapping later re-enables remote_config -> recovers without restart.
				return delegatedauthcommon.WorkloadToken{
					Token:     "wt-recovered",
					ExpiresAt: clk.Now().Add(60 * time.Minute),
				}, nil
			}
		},
	}

	apiClient := newRecordingTokenAPI()
	var presenceChanges atomic.Int32
	scheduled := make(chan time.Duration, 16)
	r := newWorkloadTokenRefresher(provider)
	r.jitter = func(time.Duration) time.Duration { return 0 }
	r.onTimerScheduled = func(d time.Duration) { scheduled <- d }

	r.start(clk, apiClient, "Remote Config", func() { presenceChanges.Add(1) })
	defer r.stop()

	// 1. Initial token acquired.
	require.Equal(t, "wt-initial", apiClient.waitToken(t))
	require.Equal(t, 45*time.Minute, <-scheduled)
	require.Equal(t, int32(1), presenceChanges.Load())

	// 2. Proactive refresh at 45m gets 403 -> clears header immediately and triggers status poll.
	clk.Add(45 * time.Minute)
	require.Equal(t, "", apiClient.waitToken(t))
	require.Equal(t, time.Minute, <-scheduled)
	require.Equal(t, int32(2), presenceChanges.Load())

	// 3. First denied backoff is 1m -> still denied (403) -> stays cleared, no extra presence change.
	clk.Add(59 * time.Second)
	require.Equal(t, 2, provider.callCount())
	clk.Add(time.Second)
	require.Equal(t, "", apiClient.waitToken(t))
	require.Equal(t, 2*time.Minute, <-scheduled)
	require.Equal(t, int32(2), presenceChanges.Load())

	// 4. Second denied backoff doubles to 2m -> now enabled -> recovers token and triggers status poll.
	clk.Add(2*time.Minute - time.Second)
	require.Equal(t, 3, provider.callCount())
	clk.Add(time.Second)
	require.Equal(t, "wt-recovered", apiClient.waitToken(t))
	require.Equal(t, 45*time.Minute, <-scheduled)
	require.Equal(t, int32(3), presenceChanges.Load())
}

func TestWorkloadTokenRefresherClearsExpiredTokenOnTransientRenewalFailure(t *testing.T) {
	clk := clock.NewMock()
	clk.Set(time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC))

	var seq atomic.Int32
	errCh := make(chan struct{}, 16)
	provider := &fakeWorkloadTokenProvider{
		managed: true,
		fn: func(context.Context, string, delegatedauthcommon.WorkloadTokenRequest) (delegatedauthcommon.WorkloadToken, error) {
			if seq.Add(1) == 1 {
				// Short 1m TTL -> proactive refresh at 45s, expires at 60s.
				return delegatedauthcommon.WorkloadToken{
					Token:     "wt-short",
					ExpiresAt: clk.Now().Add(time.Minute),
				}, nil
			}
			errCh <- struct{}{}
			return delegatedauthcommon.WorkloadToken{}, &delegatedauthcommon.WorkloadTokenHTTPError{
				StatusCode: http.StatusServiceUnavailable,
				Detail:     "Signing unavailable",
			}
		},
	}

	apiClient := newRecordingTokenAPI()
	var presenceChanges atomic.Int32
	scheduled := make(chan time.Duration, 16)
	r := newWorkloadTokenRefresher(provider)
	r.jitter = func(time.Duration) time.Duration { return 0 }
	r.onTimerScheduled = func(d time.Duration) { scheduled <- d }

	r.start(clk, apiClient, "Remote Config", func() { presenceChanges.Add(1) })
	defer r.stop()

	require.Equal(t, "wt-short", apiClient.waitToken(t))
	require.Equal(t, 45*time.Second, <-scheduled)
	require.Equal(t, int32(1), presenceChanges.Load())

	// At 45s: transient 503 during proactive refresh; token still has 15s left so it is NOT cleared yet.
	clk.Add(45 * time.Second)
	<-errCh
	require.Equal(t, 15*time.Second, <-scheduled)
	select {
	case unexpected := <-apiClient.notify:
		t.Fatalf("expected valid token to remain intact before expiry, got update %q", unexpected)
	default:
	}

	// At 60s (expiresAt): retry fails after expiry -> DD-Workload-Token MUST be cleared.
	clk.Add(15 * time.Second)
	<-errCh
	require.Equal(t, "", apiClient.waitToken(t))
	require.Equal(t, 30*time.Second, <-scheduled)
	require.Equal(t, int32(2), presenceChanges.Load())
}

func TestWorkloadTokenRefresherStopsWhenDelegatedAuthStopsManaging(t *testing.T) {
	clk := clock.NewMock()
	clk.Set(time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC))

	started := make(chan struct{}, 1)
	var provider *fakeWorkloadTokenProvider
	provider = &fakeWorkloadTokenProvider{
		managed: true,
		fn: func(context.Context, string, delegatedauthcommon.WorkloadTokenRequest) (delegatedauthcommon.WorkloadToken, error) {
			provider.setManaged(false)
			started <- struct{}{}
			return delegatedauthcommon.WorkloadToken{}, errors.New("provider detection failed")
		},
	}

	apiClient := newRecordingTokenAPI()
	r := newWorkloadTokenRefresher(provider)
	r.start(clk, apiClient, "Remote Config", nil)

	<-started
	// Goroutine exits on its own without hanging stop().
	r.stop()
	require.Equal(t, 1, provider.callCount())
}

func TestWorkloadTokenRefresherStopCancelsInFlightAndDoesNotLeak(t *testing.T) {
	clk := clock.NewMock()
	inFlight := make(chan struct{})
	canceled := make(chan struct{})

	provider := &fakeWorkloadTokenProvider{
		managed: true,
		fn: func(ctx context.Context, _ string, _ delegatedauthcommon.WorkloadTokenRequest) (delegatedauthcommon.WorkloadToken, error) {
			close(inFlight)
			<-ctx.Done()
			close(canceled)
			return delegatedauthcommon.WorkloadToken{}, ctx.Err()
		},
	}

	apiClient := newRecordingTokenAPI()
	r := newWorkloadTokenRefresher(provider)
	r.start(clk, apiClient, "Remote Config", nil)

	<-inFlight
	r.stop()

	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("expected in-flight workload token request context to be canceled by stop()")
	}
	// Idempotent stop.
	r.stop()
}

func TestCoreAgentServiceWorkloadTokenGuards(t *testing.T) {
	newServiceWithCfg := func(t *testing.T, configure func(cfg model.BuildableConfig), opts ...Option) *CoreAgentService {
		t.Helper()
		cfg := configmock.New(t)
		cfg.SetInTest("hostname", "test-hostname")
		cfg.SetInTest("run_path", t.TempDir())
		if configure != nil {
			configure(cfg)
		}
		uptaneClient := &mockCoreAgentUptane{}
		allOpts := []Option{
			uptaneFactoryOption(uptaneClient),
			WithDatabaseFileName("test.db"),
			WithAPIKey("managed-key"),
		}
		allOpts = append(allOpts, opts...)
		svc, err := NewService(cfg, "Remote Config", "https://localhost", "localhost", getHostTags, &telemetryReporter{}, agentVersion, allOpts...)
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Stop() })
		return svc
	}

	t.Run("enabled when api_key is managed and no static override or PAR JWT", func(t *testing.T) {
		provider := &fakeWorkloadTokenProvider{
			managed: true,
			fn: func(context.Context, string, delegatedauthcommon.WorkloadTokenRequest) (delegatedauthcommon.WorkloadToken, error) {
				return delegatedauthcommon.WorkloadToken{Token: "wt-ok", ExpiresAt: time.Now().Add(time.Hour)}, nil
			},
		}
		svc := newServiceWithCfg(t, nil, WithWorkloadTokenProvider(provider))
		require.NotNil(t, svc.workloadTokenRefresher)
	})

	t.Run("skipped when remote_configuration.api_key is explicitly configured", func(t *testing.T) {
		provider := &fakeWorkloadTokenProvider{managed: true}
		svc := newServiceWithCfg(t, func(cfg model.BuildableConfig) {
			cfg.SetInTest("remote_configuration.api_key", "static-rc-key")
		}, WithWorkloadTokenProvider(provider))
		require.Nil(t, svc.workloadTokenRefresher)
	})

	t.Run("skipped when WithPARJWT is set", func(t *testing.T) {
		provider := &fakeWorkloadTokenProvider{managed: true}
		svc := newServiceWithCfg(t, nil, WithWorkloadTokenProvider(provider), WithPARJWT("par-jwt-token"))
		require.Nil(t, svc.workloadTokenRefresher)
	})

	t.Run("skipped when delegatedauth is not managing api_key", func(t *testing.T) {
		provider := &fakeWorkloadTokenProvider{managed: false}
		svc := newServiceWithCfg(t, nil, WithWorkloadTokenProvider(provider))
		require.Nil(t, svc.workloadTokenRefresher)
	})

	t.Run("UpdatePARJWT stops refresher and clears workload token", func(t *testing.T) {
		tokenSet := make(chan string, 4)
		provider := &fakeWorkloadTokenProvider{
			managed: true,
			fn: func(context.Context, string, delegatedauthcommon.WorkloadTokenRequest) (delegatedauthcommon.WorkloadToken, error) {
				return delegatedauthcommon.WorkloadToken{Token: "wt-before-par", ExpiresAt: time.Now().Add(time.Hour)}, nil
			},
		}
		svc := newServiceWithCfg(t, nil, WithWorkloadTokenProvider(provider), WithAgentPollLoopDisabled())
		apiMock := &mockAPI{}
		apiMock.On("FetchOrgStatus", mock.Anything).Return(&pbgo.OrgStatusResponse{Enabled: true, Authorized: true}, nil).Maybe()
		apiMock.On("UpdateWorkloadToken", mock.Anything).Run(func(args mock.Arguments) {
			tokenSet <- args.String(0)
		}).Maybe()
		apiMock.On("UpdatePARJWT", "par-jwt").Return().Once()
		clk := clock.NewMock()
		clk.Set(time.Now())
		svc.api = apiMock
		svc.clock = clk

		svc.Start()
		require.Equal(t, "wt-before-par", <-tokenSet)

		svc.UpdatePARJWT("par-jwt")
		require.Equal(t, "", <-tokenSet)
		apiMock.AssertCalled(t, "UpdatePARJWT", "par-jwt")
	})

	t.Run("Start and Stop lifecycle with CoreAgentService", func(t *testing.T) {
		tokenSet := make(chan string, 4)
		provider := &fakeWorkloadTokenProvider{
			managed: true,
			fn: func(context.Context, string, delegatedauthcommon.WorkloadTokenRequest) (delegatedauthcommon.WorkloadToken, error) {
				return delegatedauthcommon.WorkloadToken{Token: "wt-live", ExpiresAt: time.Now().Add(time.Hour)}, nil
			},
		}
		svc := newServiceWithCfg(t, nil, WithWorkloadTokenProvider(provider), WithAgentPollLoopDisabled())
		apiMock := &mockAPI{}
		apiMock.On("FetchOrgStatus", mock.Anything).Return(&pbgo.OrgStatusResponse{Enabled: true, Authorized: true}, nil).Maybe()
		apiMock.On("UpdateWorkloadToken", mock.Anything).Run(func(args mock.Arguments) {
			tokenSet <- args.String(0)
		}).Maybe()
		clk := clock.NewMock()
		clk.Set(time.Now())
		svc.api = apiMock
		svc.clock = clk

		svc.Start()
		require.Equal(t, "wt-live", <-tokenSet)
		require.NoError(t, svc.Stop())
		// Verify uptane and refresher stopped cleanly without hanging.
		require.NoError(t, svc.Stop())
	})
}

func TestNextSuccessDelayBounds(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	r := newWorkloadTokenRefresher(nil)
	r.jitter = func(time.Duration) time.Duration { return 0 }

	require.Equal(t, 45*time.Minute, r.nextSuccessDelay(now, now.Add(60*time.Minute)))
	require.Equal(t, 3*time.Minute+45*time.Second, r.nextSuccessDelay(now, now.Add(5*time.Minute)))
	require.Equal(t, 6*time.Hour, r.nextSuccessDelay(now, now.Add(24*time.Hour)))
	require.Equal(t, 10*time.Second, r.nextSuccessDelay(now, now.Add(12*time.Second)))
	require.Equal(t, 6*time.Second, r.nextSuccessDelay(now, now.Add(8*time.Second)))
	require.Equal(t, time.Second, r.nextSuccessDelay(now, now.Add(500*time.Millisecond)))
	require.Equal(t, workloadTokenFallbackRefreshInterval, r.nextSuccessDelay(now, time.Time{}))
	require.Equal(t, workloadTokenFallbackRefreshInterval, r.nextSuccessDelay(now, now.Add(-time.Minute)))

	// Max negative jitter (10%) keeps delay strictly below 75% of TTL.
	r.jitter = func(max time.Duration) time.Duration { return max }
	require.Equal(t, 40*time.Minute+30*time.Second, r.nextSuccessDelay(now, now.Add(60*time.Minute)))
}
