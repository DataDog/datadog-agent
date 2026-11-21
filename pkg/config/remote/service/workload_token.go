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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// remoteConfigWorkloadGrant is the ETS workload grant for ordinary Agent Remote
	// Configuration (parity with the API key Remote Config scope), distinct from
	// RC Secrets (rc_secrets) and PAR enrollment (par_enrollment).
	remoteConfigWorkloadGrant = "remote_config"

	// delegatedAuthPrimaryAPIKey is the delegatedauth instance key for the Agent's
	// primary intake identity.
	delegatedAuthPrimaryAPIKey = "api_key"

	// workloadTokenRequestTimeout bounds a single ETS workload-token exchange.
	workloadTokenRequestTimeout = 35 * time.Second

	// Proactive refresh targets 75% of the server-reported token lifetime, with
	// negative jitter up to 10% of that delay (~67.5%-75% of TTL) so refreshes
	// happen well before expiry and stay spread across fleets.
	workloadTokenRefreshNumerator   = 3
	workloadTokenRefreshDenominator = 4
	workloadTokenMinRefreshInterval = 10 * time.Second
	workloadTokenMaxRefreshInterval = 6 * time.Hour

	// Fallback refresh interval when ETS omits or returns an unparseable expires
	// attribute. 4m keeps even a 5m default workload grant fresh.
	workloadTokenFallbackRefreshInterval = 4 * time.Minute

	// Backoff bounds on HTTP 403 (remote_config grant disabled on the mapping/org).
	// Periodic retries ensure enabling remote_config later takes effect without
	// restarting the Agent.
	workloadTokenDeniedInitialBackoff = 1 * time.Minute
	workloadTokenDeniedMaxBackoff     = 5 * time.Minute

	// Backoff bounds on transient failures (network, 5xx, delegatedauth startup recovery).
	workloadTokenTransientInitialBackoff = 15 * time.Second
	workloadTokenTransientMaxBackoff     = 5 * time.Minute
)

// WorkloadTokenProvider exchanges the primary intake WIF identity ("api_key")
// for an ordinary Remote Configuration workload token ("remote_config" grant).
// delegatedauth.Component satisfies this interface directly.
type WorkloadTokenProvider interface {
	GetWorkloadTokenWithExpiry(ctx context.Context, apiKeyConfigKey string, request delegatedauthcommon.WorkloadTokenRequest) (delegatedauthcommon.WorkloadToken, error)
	IsInstanceManaged(apiKeyConfigKey string) bool
}

// workloadTokenRefresher proactively fetches and refreshes "remote_config"
// workload tokens and updates DD-Workload-Token on the RC HTTP client.
type workloadTokenRefresher struct {
	provider         WorkloadTokenProvider
	jitter           func(max time.Duration) time.Duration
	onTimerScheduled func(time.Duration) // test hook invoked after clk.Timer is registered

	mu      sync.Mutex
	started bool
	stopped bool
	cancel  context.CancelFunc
	done    chan struct{}
}

type workloadTokenState struct {
	hasToken            bool
	expiresAt           time.Time
	acquiredAt          time.Time
	denied              bool
	consecutiveFailures int
	backoffDelay        time.Duration
}

func newWorkloadTokenRefresher(provider WorkloadTokenProvider) *workloadTokenRefresher {
	return &workloadTokenRefresher{
		provider: provider,
		jitter:   defaultWorkloadTokenJitter,
	}
}

func defaultWorkloadTokenJitter(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	return time.Duration(rand.Int64N(int64(max) + 1))
}

func (r *workloadTokenRefresher) start(clk clock.Clock, apiClient api.API, rcType string, onTokenPresenceChange func()) {
	r.mu.Lock()
	if r.started || r.stopped {
		r.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	r.started = true
	r.cancel = cancel
	r.done = done
	r.mu.Unlock()

	go r.run(ctx, done, clk, apiClient, rcType, onTokenPresenceChange)
}

func (r *workloadTokenRefresher) stop() {
	r.mu.Lock()
	if r.stopped {
		done := r.done
		r.mu.Unlock()
		if done != nil {
			<-done
		}
		return
	}
	r.stopped = true
	cancel := r.cancel
	done := r.done
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (r *workloadTokenRefresher) run(ctx context.Context, done chan struct{}, clk clock.Clock, apiClient api.API, rcType string, onTokenPresenceChange func()) {
	defer close(done)

	var state workloadTokenState
	for {
		delay, exit := r.step(ctx, &state, clk, apiClient, rcType, onTokenPresenceChange)
		if exit {
			return
		}
		timer := clk.Timer(delay)
		if r.onTimerScheduled != nil {
			r.onTimerScheduled(delay)
		}
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (r *workloadTokenRefresher) step(
	ctx context.Context,
	state *workloadTokenState,
	clk clock.Clock,
	apiClient api.API,
	rcType string,
	onTokenPresenceChange func(),
) (time.Duration, bool) {
	if ctx.Err() != nil {
		return 0, true
	}

	attemptCtx, cancel := clk.WithTimeout(ctx, workloadTokenRequestTimeout)
	tok, err := r.provider.GetWorkloadTokenWithExpiry(attemptCtx, delegatedAuthPrimaryAPIKey, delegatedauthcommon.WorkloadTokenRequest{
		Grants: []string{remoteConfigWorkloadGrant},
	})
	cancel()

	if ctx.Err() != nil {
		return 0, true
	}

	if err == nil && tok.Token == "" {
		err = errors.New("workload token response contained an empty token")
	}

	now := clk.Now()
	if err == nil {
		wasPresent := state.hasToken
		wasDenied := state.denied
		hadFailures := state.consecutiveFailures > 0

		apiClient.UpdateWorkloadToken(tok.Token)
		state.hasToken = true
		state.expiresAt = tok.ExpiresAt
		state.acquiredAt = now
		state.denied = false
		state.consecutiveFailures = 0
		state.backoffDelay = 0

		delay := r.nextSuccessDelay(now, tok.ExpiresAt)
		if !wasPresent || wasDenied || hadFailures {
			log.Infof("[%s] Acquired Remote Configuration workload token (next refresh in %v)", rcType, delay.Round(time.Second))
		} else {
			log.Debugf("[%s] Refreshed Remote Configuration workload token (next refresh in %v)", rcType, delay.Round(time.Second))
		}
		if !wasPresent && onTokenPresenceChange != nil {
			onTokenPresenceChange()
		}
		return delay, false
	}

	// HTTP 403: remote_config capability is disabled on the WIF mapping or org.
	// Clear DD-Workload-Token immediately so /api/v0.1/status reflects the
	// revoked/disabled capability, and back off quietly while retrying periodically
	// so enabling it later takes effect without restarting the Agent.
	if errors.Is(err, delegatedauthcommon.ErrWorkloadTokenDenied) {
		wasPresent := state.hasToken
		wasDenied := state.denied

		apiClient.UpdateWorkloadToken("")
		state.hasToken = false
		state.expiresAt = time.Time{}
		state.acquiredAt = time.Time{}

		if !wasDenied {
			state.backoffDelay = workloadTokenDeniedInitialBackoff
			state.consecutiveFailures = 1
		} else {
			state.backoffDelay = min(state.backoffDelay*2, workloadTokenDeniedMaxBackoff)
			state.consecutiveFailures++
		}
		state.denied = true

		delay := r.applyBackoffJitter(state.backoffDelay, workloadTokenDeniedMaxBackoff)
		if !wasDenied {
			log.Infof("[%s] Remote Configuration workload token denied (%v); cleared DD-Workload-Token and will retry in %v", rcType, err, delay.Round(time.Second))
		} else {
			log.Debugf("[%s] Remote Configuration workload token still denied (%v); next retry in %v", rcType, err, delay.Round(time.Second))
		}
		if wasPresent && onTokenPresenceChange != nil {
			onTokenPresenceChange()
		}
		return delay, false
	}

	// If delegated auth stopped managing api_key (for example, async startup
	// recovery found no supported cloud provider), clear any token and stop.
	if !r.provider.IsInstanceManaged(delegatedAuthPrimaryAPIKey) {
		wasPresent := state.hasToken
		if wasPresent {
			apiClient.UpdateWorkloadToken("")
			state.hasToken = false
			state.expiresAt = time.Time{}
			state.acquiredAt = time.Time{}
			if onTokenPresenceChange != nil {
				onTokenPresenceChange()
			}
		}
		log.Infof("[%s] Stopping Remote Configuration workload token refresher: delegated auth is no longer managing %q", rcType, delegatedAuthPrimaryAPIKey)
		return 0, true
	}

	// Transient failure (network, 5xx, or delegatedauth async startup recovery).
	// Keep a still-valid cached token during brief blips, but clear it once expired.
	if state.hasToken && isCachedTokenExpired(state, now) {
		apiClient.UpdateWorkloadToken("")
		state.hasToken = false
		state.expiresAt = time.Time{}
		state.acquiredAt = time.Time{}
		if onTokenPresenceChange != nil {
			onTokenPresenceChange()
		}
		log.Warnf("[%s] Cached Remote Configuration workload token expired and renewal failed (%v); cleared DD-Workload-Token", rcType, err)
	}

	if state.denied || state.backoffDelay == 0 {
		state.backoffDelay = workloadTokenTransientInitialBackoff
		state.consecutiveFailures = 1
	} else {
		state.backoffDelay = min(state.backoffDelay*2, workloadTokenTransientMaxBackoff)
		state.consecutiveFailures++
	}
	state.denied = false

	delay := r.applyBackoffJitter(state.backoffDelay, workloadTokenTransientMaxBackoff)
	// Do not oversleep past a known token expiry while retrying so an expired
	// token is cleared promptly if renewal keeps failing.
	if state.hasToken && !state.expiresAt.IsZero() && state.expiresAt.After(now) {
		if remaining := state.expiresAt.Sub(now); delay > remaining {
			delay = remaining
		}
	}

	if state.consecutiveFailures == 1 {
		log.Warnf("[%s] Failed to fetch Remote Configuration workload token: %v; retrying in %v", rcType, err, delay.Round(time.Second))
	} else {
		log.Debugf("[%s] Failed to fetch Remote Configuration workload token (attempt %d): %v; retrying in %v", rcType, state.consecutiveFailures, err, delay.Round(time.Second))
	}
	return delay, false
}

func isCachedTokenExpired(state *workloadTokenState, now time.Time) bool {
	if !state.expiresAt.IsZero() {
		return !now.Before(state.expiresAt)
	}
	return !state.acquiredAt.IsZero() && now.Sub(state.acquiredAt) >= workloadTokenFallbackRefreshInterval*2
}

func (r *workloadTokenRefresher) nextSuccessDelay(now, expiresAt time.Time) time.Duration {
	var base time.Duration
	if !expiresAt.IsZero() && expiresAt.After(now) {
		ttl := expiresAt.Sub(now)
		base = ttl * workloadTokenRefreshNumerator / workloadTokenRefreshDenominator
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
	} else {
		base = workloadTokenFallbackRefreshInterval
	}
	return r.applyNegativeJitter(base)
}

func (r *workloadTokenRefresher) applyNegativeJitter(base time.Duration) time.Duration {
	if r.jitter == nil || base <= time.Second {
		return base
	}
	delta := r.jitter(base / 10)
	if delta <= 0 {
		return base
	}
	if base-delta < time.Second {
		return time.Second
	}
	return base - delta
}

func (r *workloadTokenRefresher) applyBackoffJitter(base, maxDelay time.Duration) time.Duration {
	if r.jitter == nil || base <= 0 {
		return base
	}
	delta := r.jitter(base / 10)
	if base+delta > maxDelay {
		return maxDelay
	}
	return base + delta
}
