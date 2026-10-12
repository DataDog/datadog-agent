// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package service

import "time"

// PARScopeObservation is separate from RC org/key status. RC authorization is
// never proof of PAR enrollment permission. The producer is added in stack PR 3.
type PARScopeObservation struct {
	Reason       string
	CheckedAt    time.Time
	ValidUntil   time.Time
	NextCheck    time.Time
	RCGeneration uint64
	binding      [32]byte
}

// Granted rechecks expiry and RC continuity, not just the last scope boolean.
func (s PARScopeObservation) Granted(now time.Time, rcGeneration uint64) bool {
	return s.Reason == "scope_present" && s.RCGeneration == rcGeneration &&
		!s.CheckedAt.IsZero() && !now.Before(s.CheckedAt) && now.Before(s.ValidUntil)
}

// Matches rejects a scope observation for a different enrollment key/AAA endpoint.
func (s PARScopeObservation) Matches(key, endpoint string) bool {
	return key != "" && s.binding != [32]byte{} && s.binding == orgStatusBinding(key, endpoint)
}

// GetPARScope is deliberately unknown until an RC-owned producer is installed.
// The RC-only slice cannot authorize fresh PAR enrollment.
func (s *CoreAgentService) GetPARScope() PARScopeObservation {
	return PARScopeObservation{Reason: "par_scope_unchecked"}
}

// SetPARDiscoveryDemand expresses interest, not permission to issue a request.
// A producer must independently enforce RC readiness and its own deadlines.
func (s *CoreAgentService) SetPARDiscoveryDemand(wanted bool) {
	s.parDiscoveryDemand.Store(wanted)
}
