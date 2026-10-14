// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package service

import "time"

// PARScopeObservation is separate from RC org/key status. RC authorization is
// never proof of PAR enrollment permission.
type PARScopeObservation struct {
	Reason    string
	CheckedAt time.Time
	NextCheck time.Time
}

// Granted bounds reuse of an AAA grant; RC status has no expiry of its own.
func (s PARScopeObservation) Granted(now time.Time) bool {
	return s.Reason == "scope_present" && !s.CheckedAt.IsZero() &&
		!now.Before(s.CheckedAt) && now.Before(s.CheckedAt.Add(2*time.Minute))
}

// GetPARScope checks the current gate without forcing a discovery request.
func (s *CoreAgentService) GetPARScope() PARScopeObservation {
	if s == nil || s.parScopePoller == nil {
		return PARScopeObservation{Reason: "par_scope_unchecked"}
	}
	p := s.parScopePoller
	now := p.clock.Now()
	ready := p.gate()
	p.mu.Lock()
	defer p.mu.Unlock()
	if !ready {
		p.clearLocked()
		p.observation.Reason = "par_scope_waiting_for_rc_or_demand"
	}
	result := p.observation
	if result.Reason == "scope_present" && !result.Granted(now) {
		result.Reason = "par_scope_expired"
	}
	return result
}

// SetPARDiscoveryDemand expresses interest, not permission to issue a request.
// A producer must independently enforce RC readiness and its own deadlines.
func (s *CoreAgentService) SetPARDiscoveryDemand(wanted bool) {
	if s.parDiscoveryDemand.Swap(wanted) != wanted && s.parScopePoller != nil {
		s.parScopePoller.demandChanged()
	}
}
