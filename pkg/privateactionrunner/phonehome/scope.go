// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package phonehome

import (
	"time"

	phonehomedef "github.com/DataDog/datadog-agent/comp/privateactionrunner/phonehome/def"
)

type ScopeSource interface {
	GetPARScope() PARScopeObservation
	SetPARDiscoveryDemand(bool)
}

type PARScopeObservation struct {
	Reason    phonehomedef.Reason
	CheckedAt time.Time
	NextCheck time.Time
}

func (s PARScopeObservation) Granted(now time.Time) bool {
	return s.Reason == phonehomedef.ReasonScopePresent && !s.CheckedAt.IsZero() &&
		!now.Before(s.CheckedAt) && now.Before(s.CheckedAt.Add(2*time.Minute))
}

type scopedSource struct {
	rc    Source
	scope ScopeSource
}

// NewScopedSource adds AAA approval to every new start, regardless of saved identity.
func NewScopedSource(rc Source, scope ScopeSource) Source { return &scopedSource{rc: rc, scope: scope} }

func (s *scopedSource) Read(now time.Time) Observation {
	rc := s.rc.Read(now)
	if s.scope != nil {
		s.scope.SetPARDiscoveryDemand(rc.Ready)
	}
	if !rc.Ready {
		return rc
	}
	if s.scope == nil {
		return Observation{Reason: phonehomedef.ReasonPARScopeUnchecked}
	}
	grant := s.scope.GetPARScope()
	if grant.Granted(now) {
		return Observation{Ready: true, Reason: phonehomedef.ReasonScopePresent}
	}
	if grant.Reason == "" {
		grant.Reason = phonehomedef.ReasonPARScopeUnchecked
	}
	if grant.Reason == phonehomedef.ReasonScopePresent {
		grant.Reason = phonehomedef.ReasonPARScopeExpired
	}
	return Observation{Reason: grant.Reason}
}
