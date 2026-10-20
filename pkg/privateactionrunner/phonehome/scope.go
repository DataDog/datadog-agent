// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package phonehome

import "time"

// ScopeSource owns PAR enrollment permission and discovery demand, separately
// from RC's organization/key status. A nil source leaves fresh enrollment waiting.
type ScopeSource interface {
	GetPARScope() PARScopeObservation
	SetPARDiscoveryDemand(bool)
}

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
