// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package service

import (
	"crypto/sha256"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/remote/api"
)

const maxOrgStatusAge = 5 * time.Minute

// OrgStatusState distinguishes missing/failed observations from explicit false values.
type OrgStatusState string

const (
	OrgStatusUnavailable   OrgStatusState = "unavailable"
	OrgStatusUnchecked     OrgStatusState = "unchecked"
	OrgStatusObserved      OrgStatusState = "observed"
	OrgStatusFetchFailed   OrgStatusState = "fetch_failed"
	OrgStatusStale         OrgStatusState = "stale"
	OrgStatusConfigChanged OrgStatusState = "configuration_changed"
)

// OrgStatus is a value snapshot of the existing /status poll. Booleans retain the
// last successful observation even on failure; they are not authorization alone.
// Provenance is private and cannot expose the credential through JSON/status.
type OrgStatus struct {
	State         OrgStatusState
	Enabled       bool
	Authorized    bool
	ObservedAt    time.Time
	LastAttemptAt time.Time
	ValidUntil    time.Time
	// Generation changes when readiness or credential continuity is lost.
	Generation uint64
	binding    [32]byte
}

func orgStatusBinding(key, endpoint string) [32]byte {
	return sha256.Sum256([]byte(strings.TrimSpace(key) + "\x00" + endpoint))
}

// Matches checks API-key/endpoint provenance, not readiness. JWT/legacy-app-key
// status cannot be used as proof of API-key authorization.
func (s OrgStatus) Matches(apiKey, endpoint string) bool {
	return strings.TrimSpace(apiKey) != "" && s.binding != [32]byte{} && s.binding == orgStatusBinding(apiKey, endpoint)
}

// Ready checks freshness again at use/dispatch time, including for saved copies.
// Callers must also check Matches against their credential/endpoint and reread
// at dispatch to see failures since this snapshot. This is not other permission
// or configuration-delivery readiness.
func (s OrgStatus) Ready(now time.Time) bool {
	return s.State == OrgStatusObserved && s.Enabled && s.Authorized &&
		!s.ObservedAt.IsZero() && !now.Before(s.ObservedAt) && now.Before(s.ValidUntil)
}

// GetOrgStatus returns a copy, never the mutable diagnostic response cache.
// Reading does not initiate requests or change the existing RC polling cadence.
func (s *CoreAgentService) GetOrgStatus() OrgStatus {
	if s == nil || s.orgStatusPoller == nil {
		return OrgStatus{State: OrgStatusUnavailable}
	}
	p := s.orgStatusPoller
	p.mu.Lock()
	defer p.mu.Unlock()
	out := p.mu.status
	if p.mu.stopped {
		out.State = OrgStatusUnavailable
	} else if p.mu.configChanged {
		out.State = OrgStatusConfigChanged
	} else if out.State == OrgStatusObserved && !p.clock.Now().Before(out.ValidUntil) {
		out.State = OrgStatusStale
	}
	return out
}

// configureStatus is called before registering config callbacks or starting polls.
func (p *orgStatusPoller) configureStatus(auth api.Auth, endpoint string) {
	p.mu.endpoint = endpoint
	p.mu.apiKeyOnly = auth.APIKey != "" && auth.PARJWT == "" && !auth.UseAppKey
	if p.mu.apiKeyOnly {
		p.mu.status.binding = orgStatusBinding(auth.APIKey, endpoint)
	}
}

// invalidateStatusLocked fences in-flight observations without changing cadence.
func (p *orgStatusPoller) invalidateStatusLocked() {
	p.mu.status.Generation++
	p.mu.status.State = OrgStatusUnchecked
	p.mu.status.ValidUntil = time.Time{}
}

func (p *orgStatusPoller) configurationChanged() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.invalidateStatusLocked()
	p.mu.configChanged = true // The transport/override is not live-reconfigured.
}

func (p *orgStatusPoller) updateAPIKey(client api.API, key string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Updating the HTTP headers and generation under one lock prevents a poll
	// from acquiring the new generation but dispatching with the old key.
	client.UpdateAPIKey(key)
	p.invalidateStatusLocked()
	if p.mu.apiKeyOnly {
		p.mu.status.binding = orgStatusBinding(key, p.mu.endpoint)
	}
}
