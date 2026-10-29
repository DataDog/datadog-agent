// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package phonehome

import (
	"encoding/json"
	"testing"
)

func TestStateWireVocabulary(t *testing.T) {
	states := map[StateName]string{
		StateBlocked:   "blocked",
		StateHandedOff: "handed_off",
		StateWaiting:   "waiting",
	}
	for state, want := range states {
		got, err := json.Marshal(State{State: state})
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != `{"state":"`+want+`","reason":""}` {
			t.Errorf("state %s: %s", state, got)
		}
	}
	reasons := map[Reason]string{
		ReasonExistingProcessLeftUntouched:        "existing_process_left_untouched",
		ReasonLaunchOutcomeUnknownRequiresReview:  "launch_outcome_unknown_requires_review",
		ReasonLegacyAttemptRequiresReconciliation: "legacy_attempt_requires_reconciliation",
		ReasonPreviousProcessExitRequiresReview:   "previous_process_exit_requires_review",
		ReasonRCCredentialOrEndpointMismatch:      "rc_credential_or_endpoint_mismatch",
		ReasonRCKeyUnauthorized:                   "rc_key_unauthorized",
		ReasonRCLocallyDisabled:                   "rc_locally_disabled",
		ReasonRCOrgDisabled:                       "rc_org_disabled",
		ReasonRCReady:                             "rc_ready",
		ReasonRCServiceUnavailable:                "rc_service_unavailable",
		ReasonRCUnchecked:                         "rc_unchecked",
		ReasonStartupJitter:                       "startup_jitter",
		ReasonSupervisorConnectionFailed:          "supervisor_connection_failed",
		ReasonSupervisorStartAcknowledged:         "supervisor_start_acknowledged",
		ReasonSupervisorUnavailable:               "supervisor_unavailable",
		ReasonUnsupportedConfiguration:            "unsupported_configuration",
		ReasonDiscoveryRateLimited:                "discovery_rate_limited",
		ReasonDiscoveryUnavailable:                "discovery_unavailable",
		ReasonInvalidCredentials:                  "invalid_credentials",
		ReasonInvalidDiscoveryResponse:            "invalid_discovery_response",
		ReasonInvalidEndpoint:                     "invalid_endpoint",
		ReasonMissingEnrollmentScope:              "missing_enrollment_scope",
		ReasonPARScopeExpired:                     "par_scope_expired",
		ReasonPARScopeUnchecked:                   "par_scope_unchecked",
		ReasonPARScopeWaitingForRCOrDemand:        "par_scope_waiting_for_rc_or_demand",
		ReasonScopePresent:                        "scope_present",
		ReasonSupervisorRegistrationChangePending: "supervisor_registration_change_pending",
		ReasonSupervisorRegistrationUnavailable:   "supervisor_registration_unavailable",
		ReasonInvalidSupervisorSocket:             "invalid_supervisor_socket",
		ReasonUnsupportedSupervisorPolicy:         "unsupported_supervisor_policy",
	}
	for reason, want := range reasons {
		got, err := json.Marshal(State{Reason: reason})
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != `{"state":"","reason":"`+want+`"}` {
			t.Errorf("reason %s: %s", reason, got)
		}
	}
}

func TestDiscoveryHTTPReason(t *testing.T) {
	for code, want := range map[int]string{302: "discovery_http_302", 403: "discovery_http_403", 418: "discovery_http_418"} {
		if got := DiscoveryHTTPReason(code); string(got) != want {
			t.Errorf("HTTP %d: got %s, want %s", code, got, want)
		}
	}
}
