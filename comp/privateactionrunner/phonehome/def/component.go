// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package phonehome defines the core-owned PAR launch/reconciliation component.
package phonehome

// team: action-platform

// Component owns lifecycle and exposes sanitized enrollment state, not action readiness.
type Component interface{ Status() *State }

// State contains no identity, response body or credential.
type State struct {
	State                  string `json:"state"`
	Reason                 string `json:"reason"`
	NextRetry              string `json:"next_retry,omitempty"`
	RCReady                bool   `json:"rc_ready"`
	RCReason               string `json:"rc_reason"`
	ScopeReady             bool   `json:"scope_ready"`
	ScopeChecked           bool   `json:"scope_checked"`
	EnrollmentOutcome      string `json:"enrollment_outcome"`
	RetrySafe              bool   `json:"retry_safe"`
	ReconciliationRequired bool   `json:"reconciliation_required"`
	NextDiscovery          string `json:"next_discovery,omitempty"`
	NextEnrollmentAttempt  string `json:"next_enrollment_attempt,omitempty"`
	EnrollmentRetryAfter   string `json:"enrollment_retry_after,omitempty"`
	HelperRetained         bool   `json:"helper_retained"`
}
