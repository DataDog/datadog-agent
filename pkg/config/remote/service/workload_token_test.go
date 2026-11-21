// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/require"

	delegatedauthcommon "github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

type fakeWorkloadTokenProvider struct {
	mu      sync.Mutex
	managed bool
	calls   int
	fn      func(context.Context, string, delegatedauthcommon.WorkloadTokenRequest) (string, error)
}

func (f *fakeWorkloadTokenProvider) IsInstanceManaged(key string) bool {
	return key == delegatedAuthPrimaryAPIKey && f.managed
}

func (f *fakeWorkloadTokenProvider) GetWorkloadToken(ctx context.Context, key string, req delegatedauthcommon.WorkloadTokenRequest) (string, error) {
	f.mu.Lock()
	f.calls++
	fn := f.fn
	f.mu.Unlock()
	return fn(ctx, key, req)
}

type stubRCAPI struct {
	mu          sync.Mutex
	token       string
	parJWT      string
	seenOnFetch []string
}

func (s *stubRCAPI) recordToken() {
	s.mu.Lock()
	s.seenOnFetch = append(s.seenOnFetch, s.token)
	s.mu.Unlock()
}

func (s *stubRCAPI) Fetch(context.Context, *pbgo.LatestConfigsRequest) (*pbgo.LatestConfigsResponse, error) {
	s.recordToken()
	return &pbgo.LatestConfigsResponse{}, nil
}

func (s *stubRCAPI) FetchOrgData(context.Context) (*pbgo.OrgDataResponse, error) {
	s.recordToken()
	return &pbgo.OrgDataResponse{}, nil
}

func (s *stubRCAPI) FetchOrgStatus(context.Context) (*pbgo.OrgStatusResponse, error) {
	s.recordToken()
	return &pbgo.OrgStatusResponse{}, nil
}

func (s *stubRCAPI) UpdateAPIKey(string) {}

func (s *stubRCAPI) UpdatePARJWT(jwt string) {
	s.mu.Lock()
	s.parJWT = jwt
	s.mu.Unlock()
}

func (s *stubRCAPI) UpdateWorkloadToken(token string) {
	s.mu.Lock()
	s.token = token
	s.mu.Unlock()
}

func TestWorkloadTokenClientFetchCacheAndProactiveRefresh(t *testing.T) {
	clk := clock.NewMock()
	clk.Set(time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC))

	tokens := []string{"wt-1", "wt-2"}
	idx := 0
	provider := &fakeWorkloadTokenProvider{
		managed: true,
		fn: func(_ context.Context, key string, req delegatedauthcommon.WorkloadTokenRequest) (string, error) {
			require.Equal(t, "api_key", key)
			require.Equal(t, []string{"remote_config"}, req.Grants)
			require.Empty(t, req.JWKThumbprint)
			tok := tokens[idx]
			idx++
			return tok, nil
		},
	}

	inner := &stubRCAPI{}
	client := newWorkloadTokenClient(inner, provider, clk, "Remote Config")
	client.jitter = func(time.Duration) time.Duration { return 0 }

	// 1. Initial fetch attaches token BEFORE first RC request (no startup race).
	_, err := client.FetchOrgStatus(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, provider.calls)
	require.Equal(t, []string{"wt-1"}, inner.seenOnFetch)

	// 2. Cache hit reuses token without calling ETS before 3m45s (75% of 5m).
	clk.Add(3*time.Minute + 44*time.Second)
	_, err = client.Fetch(context.Background(), &pbgo.LatestConfigsRequest{})
	require.NoError(t, err)
	require.Equal(t, 1, provider.calls)
	require.Equal(t, "wt-1", inner.seenOnFetch[1])

	// 3. At 3m45s, proactively refreshes token before 5m expiry.
	clk.Add(time.Second)
	_, err = client.FetchOrgData(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, provider.calls)
	require.Equal(t, "wt-2", inner.seenOnFetch[2])
}

func TestWorkloadTokenClientBackoffExpiryClearAndRecovery(t *testing.T) {
	clk := clock.NewMock()
	clk.Set(time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC))

	var returnErr error
	nextTok := "wt-1"
	provider := &fakeWorkloadTokenProvider{
		managed: true,
		fn: func(context.Context, string, delegatedauthcommon.WorkloadTokenRequest) (string, error) {
			if returnErr != nil {
				return "", returnErr
			}
			return nextTok, nil
		},
	}

	inner := &stubRCAPI{}
	client := newWorkloadTokenClient(inner, provider, clk, "Remote Config")
	client.jitter = func(time.Duration) time.Duration { return 0 }

	// Acquire initial token (proactive refresh at 3m45s, expires at 5m).
	_, _ = client.FetchOrgStatus(context.Background())
	require.Equal(t, 1, provider.calls)
	require.Equal(t, "wt-1", inner.token)

	// At 3m45s: refresh fails -> keeps still-valid token and enters 1m backoff.
	returnErr = errors.New("workload token request failed (HTTP 403): Workload grant denied")
	clk.Add(3*time.Minute + 45*time.Second)
	_, _ = client.FetchOrgStatus(context.Background())
	require.Equal(t, 2, provider.calls)
	require.Equal(t, "wt-1", inner.token)

	// Within 1m backoff: does not hammer ETS.
	clk.Add(30 * time.Second)
	_, _ = client.FetchOrgStatus(context.Background())
	require.Equal(t, 2, provider.calls)
	require.Equal(t, "wt-1", inner.token)

	// At 4m45s (after 1m backoff, 15s left before 5m expiry): fails again,
	// next retry is clamped to 5m expiry instead of oversleeping 2m.
	clk.Add(30 * time.Second)
	_, _ = client.FetchOrgStatus(context.Background())
	require.Equal(t, 3, provider.calls)
	require.Equal(t, "wt-1", inner.token)

	// At 5m (token expired): clears DD-Workload-Token and enters 4m backoff.
	clk.Add(15 * time.Second)
	_, _ = client.FetchOrgStatus(context.Background())
	require.Equal(t, 4, provider.calls)
	require.Empty(t, inner.token)
	require.Empty(t, inner.seenOnFetch[len(inner.seenOnFetch)-1])

	// Still inside 4m backoff at +2m: does not call ETS.
	clk.Add(2 * time.Minute)
	_, _ = client.FetchOrgStatus(context.Background())
	require.Equal(t, 4, provider.calls)

	// After 4m backoff expires and mapping is enabled: recovers without restart.
	returnErr = nil
	nextTok = "wt-recovered"
	clk.Add(2 * time.Minute)
	_, _ = client.FetchOrgStatus(context.Background())
	require.Equal(t, 5, provider.calls)
	require.Equal(t, "wt-recovered", inner.token)
	require.Equal(t, "wt-recovered", inner.seenOnFetch[len(inner.seenOnFetch)-1])
}

func TestWorkloadTokenClientUpdatePARJWTDisablesAndClears(t *testing.T) {
	clk := clock.NewMock()
	clk.Set(time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC))

	provider := &fakeWorkloadTokenProvider{
		managed: true,
		fn: func(context.Context, string, delegatedauthcommon.WorkloadTokenRequest) (string, error) {
			return "wt-initial", nil
		},
	}

	inner := &stubRCAPI{}
	client := newWorkloadTokenClient(inner, provider, clk, "Remote Config")

	_, _ = client.FetchOrgStatus(context.Background())
	require.Equal(t, "wt-initial", inner.token)

	client.UpdatePARJWT("par-jwt")
	require.Empty(t, inner.token)
	require.Equal(t, "par-jwt", inner.parJWT)

	// Advancing past expiry must not re-fetch workload tokens once PAR JWT is active.
	clk.Add(10 * time.Minute)
	_, _ = client.FetchOrgStatus(context.Background())
	require.Equal(t, 1, provider.calls)
	require.Empty(t, inner.token)
}

func TestNewServiceWorkloadTokenGuards(t *testing.T) {
	newSvc := func(t *testing.T, configure func(model.BuildableConfig), opts ...Option) *CoreAgentService {
		t.Helper()
		cfg := configmock.New(t)
		cfg.SetInTest("hostname", "test-hostname")
		cfg.SetInTest("run_path", t.TempDir())
		if configure != nil {
			configure(cfg)
		}
		allOpts := append([]Option{
			uptaneFactoryOption(&mockCoreAgentUptane{}),
			WithDatabaseFileName("test.db"),
			WithAPIKey("managed-key"),
		}, opts...)
		svc, err := NewService(cfg, "Remote Config", "https://localhost", "localhost", getHostTags, &telemetryReporter{}, agentVersion, allOpts...)
		require.NoError(t, err)
		t.Cleanup(func() { _ = svc.Stop() })
		return svc
	}

	provider := &fakeWorkloadTokenProvider{managed: true}

	svc := newSvc(t, nil, WithWorkloadTokenProvider(provider))
	_, wrapped := svc.api.(*workloadTokenClient)
	require.True(t, wrapped, "expected workloadTokenClient when api_key is managed")

	svcStatic := newSvc(t, func(cfg model.BuildableConfig) {
		cfg.SetInTest("remote_configuration.api_key", "static-rc-key")
	}, WithWorkloadTokenProvider(provider))
	_, wrapped = svcStatic.api.(*workloadTokenClient)
	require.False(t, wrapped, "must skip workload tokens when remote_configuration.api_key is set")

	svcPAR := newSvc(t, nil, WithWorkloadTokenProvider(provider), WithPARJWT("par-jwt"))
	_, wrapped = svcPAR.api.(*workloadTokenClient)
	require.False(t, wrapped, "must skip workload tokens when WithPARJWT is set")

	svcUnmanaged := newSvc(t, nil, WithWorkloadTokenProvider(&fakeWorkloadTokenProvider{managed: false}))
	_, wrapped = svcUnmanaged.api.(*workloadTokenClient)
	require.False(t, wrapped, "must skip workload tokens when api_key is not managed")
}
