// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package phonehome defines the core-owned PAR startup gate.
package phonehome

import "strconv"

// team: action-platform

// Component owns the core phone-home lifecycle.
type Component interface{}

// StateName identifies a phone-home startup state.
type StateName string

// Reason identifies why phone-home is in its current state.
type Reason string

// State describes the startup handoff, not enrollment or action readiness.
type State struct {
	State     StateName `json:"state"`
	Reason    Reason    `json:"reason"`
	NextCheck string    `json:"next_check,omitempty"`
}

// Phone-home startup states.
const (
	StateBlocked   StateName = "blocked"
	StateHandedOff StateName = "handed_off"
	StateWaiting   StateName = "waiting"
)

// Phone-home startup reasons.
const (
	ReasonExistingProcessLeftUntouched        Reason = "existing_process_left_untouched"
	ReasonExplicitSupervisorSocketRequired    Reason = "explicit_supervisor_socket_required"
	ReasonLaunchOutcomeUnknownRequiresReview  Reason = "launch_outcome_unknown_requires_review"
	ReasonLegacyAttemptRequiresReconciliation Reason = "legacy_attempt_requires_reconciliation"
	ReasonPreviousProcessExitRequiresReview   Reason = "previous_process_exit_requires_review"
	ReasonRCCredentialOrEndpointMismatch      Reason = "rc_credential_or_endpoint_mismatch"
	ReasonRCKeyUnauthorized                   Reason = "rc_key_unauthorized"
	ReasonRCLocallyDisabled                   Reason = "rc_locally_disabled"
	ReasonRCOrgDisabled                       Reason = "rc_org_disabled"
	ReasonRCReady                             Reason = "rc_ready"
	ReasonRCServiceUnavailable                Reason = "rc_service_unavailable"
	ReasonRCUnchecked                         Reason = "rc_unchecked"
	ReasonStartupJitter                       Reason = "startup_jitter"
	ReasonSupervisorConnectionFailed          Reason = "supervisor_connection_failed"
	ReasonSupervisorMustBeOnDemandNoRestart   Reason = "supervisor_must_be_on_demand_no_restart"
	ReasonSupervisorStartAcknowledged         Reason = "supervisor_start_acknowledged"
	ReasonSupervisorUnavailable               Reason = "supervisor_unavailable"
	ReasonUnsupportedConfiguration            Reason = "unsupported_configuration"
	ReasonDiscoveryRateLimited                Reason = "discovery_rate_limited"
	ReasonDiscoveryUnavailable                Reason = "discovery_unavailable"
	ReasonInvalidCredentials                  Reason = "invalid_credentials"
	ReasonInvalidDiscoveryResponse            Reason = "invalid_discovery_response"
	ReasonInvalidEndpoint                     Reason = "invalid_endpoint"
	ReasonMissingEnrollmentScope              Reason = "missing_enrollment_scope"
	ReasonPARScopeExpired                     Reason = "par_scope_expired"
	ReasonPARScopeUnchecked                   Reason = "par_scope_unchecked"
	ReasonPARScopeWaitingForRCOrDemand        Reason = "par_scope_waiting_for_rc_or_demand"
	ReasonScopePresent                        Reason = "scope_present"
)

// DiscoveryHTTPReason preserves the status code for unexpected discovery responses.
func DiscoveryHTTPReason(statusCode int) Reason {
	return Reason("discovery_http_" + strconv.Itoa(statusCode))
}
