// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package service

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	delegatedauthcommon "github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// remoteConfigWorkloadGrant is the ETS workload grant for ordinary Agent Remote
	// Configuration (parity with the API key Remote Config scope), distinct from
	// RC Secrets (rc_secrets) and PAR enrollment (par_enrollment).
	remoteConfigWorkloadGrant  = "remote_config"
	delegatedAuthPrimaryAPIKey = "api_key"

	workloadTokenRequestTimeout = 35 * time.Second

	// Proactive refresh targets ~75% of server-returned TTL (with up to 10%
	// negative jitter), falling back to 4m when ExpiresAt is absent/unparseable.
	workloadTokenMinRefreshInterval      = 10 * time.Second
	workloadTokenMaxRefreshInterval      = 6 * time.Hour
	workloadTokenFallbackRefreshInterval = 4 * time.Minute

	workloadTokenInitialBackoff = 1 * time.Minute
	workloadTokenMaxBackoff     = 5 * time.Minute
)

// WorkloadTokenProvider exchanges the primary intake WIF identity ("api_key")
// for an ordinary Remote Configuration workload token ("remote_config" grant).
// delegatedauth.Component satisfies this interface directly.
type WorkloadTokenProvider interface {
	GetWorkloadTokenWithExpiry(ctx context.Context, apiKeyConfigKey string, request delegatedauthcommon.WorkloadTokenRequest) (delegatedauthcommon.WorkloadToken, error)
	IsInstanceManaged(apiKeyConfigKey string) bool
}

// workloadTokenClient wraps api.API to attach and proactively refresh
// DD-Workload-Token on demand before RC requests, with no background goroutine.
type workloadTokenClient struct {
	inner    api.API
	provider WorkloadTokenProvider
	clock    clock.Clock
	rcType   string
	jitter   func(max time.Duration) time.Duration

	// refreshMu serializes ETS exchanges without blocking UpdatePARJWT or cache hits.
	// Lock ordering is always refreshMu -> mu.
	refreshMu sync.Mutex

	mu          sync.Mutex
	disabled    bool
	hasToken    bool
	expiresAt   time.Time
	acquiredAt  time.Time
	refreshAt   time.Time
	backoff     time.Duration
	nextRetryAt time.Time
}

func newWorkloadTokenClient(inner api.API, provider WorkloadTokenProvider, clk clock.Clock, rcType string) *workloadTokenClient {
	return &workloadTokenClient{
		inner:    inner,
		provider: provider,
		clock:    clk,
		rcType:   rcType,
		jitter: func(max time.Duration) time.Duration {
			if max <= 0 {
				return 0
			}
			return time.Duration(rand.Int64N(int64(max) + 1))
		},
	}
}

func (c *workloadTokenClient) Fetch(ctx context.Context, req *pbgo.LatestConfigsRequest) (*pbgo.LatestConfigsResponse, error) {
	c.ensureToken(ctx)
	return c.inner.Fetch(ctx, req)
}

func (c *workloadTokenClient) FetchOrgData(ctx context.Context) (*pbgo.OrgDataResponse, error) {
	c.ensureToken(ctx)
	return c.inner.FetchOrgData(ctx)
}

func (c *workloadTokenClient) FetchOrgStatus(ctx context.Context) (*pbgo.OrgStatusResponse, error) {
	c.ensureToken(ctx)
	return c.inner.FetchOrgStatus(ctx)
}

func (c *workloadTokenClient) UpdateAPIKey(apiKey string) {
	c.inner.UpdateAPIKey(apiKey)
}

func (c *workloadTokenClient) UpdateWorkloadToken(token string) {
	c.inner.UpdateWorkloadToken(token)
}

func (c *workloadTokenClient) UpdatePARJWT(jwt string) {
	if jwt != "" {
		c.mu.Lock()
		c.disabled = true
		c.clearTokenLocked()
		c.mu.Unlock()
	}
	c.inner.UpdatePARJWT(jwt)
}

func (c *workloadTokenClient) ensureToken(ctx context.Context) {
	c.mu.Lock()
	need := c.needsRefreshLocked(c.clock.Now())
	c.mu.Unlock()
	if !need || ctx.Err() != nil {
		return
	}

	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	c.mu.Lock()
	need = c.needsRefreshLocked(c.clock.Now())
	c.mu.Unlock()
	if !need || ctx.Err() != nil {
		return
	}

	attemptCtx, cancel := context.WithTimeout(ctx, workloadTokenRequestTimeout)
	tok, err := c.provider.GetWorkloadTokenWithExpiry(attemptCtx, delegatedAuthPrimaryAPIKey, delegatedauthcommon.WorkloadTokenRequest{
		Grants: []string{remoteConfigWorkloadGrant},
	})
	cancel()
	if ctx.Err() != nil {
		return
	}
	if err == nil && tok.Token == "" {
		err = errors.New("workload token response contained an empty token")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.disabled {
		return
	}

	now := c.clock.Now()
	if err == nil {
		wasMissing := !c.hasToken || c.backoff > 0
		delay := c.nextRefreshDelay(now, tok.ExpiresAt)
		c.inner.UpdateWorkloadToken(tok.Token)
		c.hasToken = true
		c.expiresAt = tok.ExpiresAt
		c.acquiredAt = now
		c.refreshAt = now.Add(delay)
		c.backoff = 0
		c.nextRetryAt = time.Time{}
		if wasMissing {
			log.Infof("[%s] Acquired Remote Configuration workload token (next refresh in %v)", c.rcType, delay.Round(time.Second))
		} else {
			log.Debugf("[%s] Refreshed Remote Configuration workload token (next refresh in %v)", c.rcType, delay.Round(time.Second))
		}
		return
	}

	// Keep a still-valid cached token during brief blips; clear once expired or absent.
	if !c.hasToken || c.isExpiredLocked(now) {
		c.clearTokenLocked()
	}
	firstFailure := c.backoff == 0
	if firstFailure {
		c.backoff = workloadTokenInitialBackoff
	} else {
		c.backoff = min(c.backoff*2, workloadTokenMaxBackoff)
	}
	c.nextRetryAt = now.Add(c.backoff)
	if c.hasToken && !c.expiresAt.IsZero() && c.expiresAt.After(now) && c.nextRetryAt.After(c.expiresAt) {
		c.nextRetryAt = c.expiresAt
	}
	if firstFailure {
		log.Infof("[%s] Could not fetch Remote Configuration workload token (%v); next retry in %v", c.rcType, err, c.nextRetryAt.Sub(now).Round(time.Second))
	} else {
		log.Debugf("[%s] Could not fetch Remote Configuration workload token (%v); next retry in %v", c.rcType, err, c.nextRetryAt.Sub(now).Round(time.Second))
	}
}

func (c *workloadTokenClient) needsRefreshLocked(now time.Time) bool {
	if c.disabled {
		return false
	}
	if c.hasToken && c.isExpiredLocked(now) {
		c.clearTokenLocked()
	}
	if c.hasToken && now.Before(c.refreshAt) {
		return false
	}
	if !c.nextRetryAt.IsZero() && now.Before(c.nextRetryAt) {
		return false
	}
	return true
}

func (c *workloadTokenClient) isExpiredLocked(now time.Time) bool {
	if !c.expiresAt.IsZero() {
		return !now.Before(c.expiresAt)
	}
	return !c.acquiredAt.IsZero() && now.Sub(c.acquiredAt) >= workloadTokenFallbackRefreshInterval*2
}

func (c *workloadTokenClient) clearTokenLocked() {
	c.inner.UpdateWorkloadToken("")
	c.hasToken = false
	c.expiresAt = time.Time{}
	c.acquiredAt = time.Time{}
	c.refreshAt = time.Time{}
}

func (c *workloadTokenClient) nextRefreshDelay(now, expiresAt time.Time) time.Duration {
	base := workloadTokenFallbackRefreshInterval
	if !expiresAt.IsZero() && expiresAt.After(now) {
		ttl := expiresAt.Sub(now)
		base = ttl * 3 / 4
		if base > workloadTokenMaxRefreshInterval {
			base = workloadTokenMaxRefreshInterval
		}
		if base < workloadTokenMinRefreshInterval {
			if ttl > workloadTokenMinRefreshInterval {
				base = workloadTokenMinRefreshInterval
			} else if base < time.Second {
				base = time.Second
			}
		}
	}
	if c.jitter != nil && base > time.Second {
		if delta := c.jitter(base / 10); delta > 0 {
			base = max(base-delta, time.Second)
		}
	}
	return base
}
