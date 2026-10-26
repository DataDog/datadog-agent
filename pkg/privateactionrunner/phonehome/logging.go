// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package phonehome

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// The controller reports serially; NextCheck changes are not transitions.
func newStateLogger(bypass bool) func(State) {
	var previous State
	return func(s State) {
		if s.State == previous.State && s.Reason == previous.Reason {
			return
		}
		previous = s
		// Log handoffs at the operation, not when connectivity restores the same status.
		if s.State == "handed_off" {
			return
		}
		reason := s.Reason
		// Only allowlisted reasons reach logs, even if a source returns unexpected data.
		switch reason {
		case "unsupported_configuration":
			if bypass {
				log.Debug("PAR startup gate: bypassed in an intentionally ungated mode: reason=unsupported_configuration")
				return
			}
			fallthrough
		case "invalid_supervisor_socket", "supervisor_connection_failed", "unsupported_supervisor_policy",
			"previous_process_exit_requires_review", "legacy_attempt_requires_reconciliation", "launch_outcome_unknown_requires_review",
			"rc_credential_or_endpoint_mismatch", "invalid_credentials", "invalid_endpoint":
			log.Warnf("PAR startup gate: startup requires operator review: reason=%s", reason)
		case "rc_locally_disabled", "rc_unchecked", "rc_org_disabled", "rc_key_unauthorized",
			"par_scope_unchecked", "par_scope_waiting_for_rc_or_demand", "par_scope_expired", "missing_enrollment_scope":
			log.Infof("PAR startup gate: waiting for permission: reason=%s", reason)
		case "startup_jitter", "supervisor_unavailable", "supervisor_registration_unavailable", "supervisor_registration_change_pending",
			"rc_service_unavailable", "discovery_unavailable", "discovery_rate_limited", "invalid_discovery_response":
			log.Debugf("PAR startup gate: waiting to retry: reason=%s", reason)
		default:
			if strings.HasPrefix(reason, "discovery_http_") {
				log.Debug("PAR startup gate: waiting to retry: reason=discovery_http_error")
			} else {
				log.Debug("PAR startup gate: waiting to retry: reason=unrecognized_observation")
			}
		}
	}
}
