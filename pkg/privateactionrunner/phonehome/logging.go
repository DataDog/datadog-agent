// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package phonehome

import (
	"strings"

	phonehomedef "github.com/DataDog/datadog-agent/comp/privateactionrunner/phonehome/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Receives state/reason transitions from the controller's emitter.
func newStateLogger(bypass bool) func(State) {
	return func(s State) {
		// Log handoffs at the operation, not when connectivity restores the same status.
		if s.State == phonehomedef.StateHandedOff {
			return
		}
		reason := s.Reason
		// Only allowlisted reasons reach logs, even if a source returns unexpected data.
		switch reason {
		case phonehomedef.ReasonLegacyInit:
			log.Debug("PAR startup gate: not applicable; legacy init does not start the process manager: reason=legacy_init")
		case phonehomedef.ReasonUnsupportedConfiguration:
			if bypass {
				log.Debugf("PAR startup gate: bypassed in an intentionally ungated mode: reason=%s", reason)
				return
			}
			fallthrough
		case phonehomedef.ReasonInvalidSupervisorSocket, phonehomedef.ReasonSupervisorConnectionFailed, phonehomedef.ReasonUnsupportedSupervisorPolicy,
			phonehomedef.ReasonPreviousProcessExitRequiresReview, phonehomedef.ReasonLegacyAttemptRequiresReconciliation, phonehomedef.ReasonLaunchOutcomeUnknownRequiresReview,
			phonehomedef.ReasonRCCredentialOrEndpointMismatch, phonehomedef.ReasonInvalidCredentials, phonehomedef.ReasonInvalidEndpoint:
			log.Warnf("PAR startup gate: startup requires operator review: reason=%s", reason)
		case phonehomedef.ReasonRCLocallyDisabled, phonehomedef.ReasonRCUnchecked, phonehomedef.ReasonRCOrgDisabled, phonehomedef.ReasonRCKeyUnauthorized,
			phonehomedef.ReasonPARScopeUnchecked, phonehomedef.ReasonPARScopeWaitingForRCOrDemand, phonehomedef.ReasonPARScopeExpired, phonehomedef.ReasonMissingEnrollmentScope:
			log.Infof("PAR startup gate: waiting for permission: reason=%s", reason)
		case phonehomedef.ReasonStartupJitter, phonehomedef.ReasonSupervisorUnavailable, phonehomedef.ReasonSupervisorRegistrationUnavailable, phonehomedef.ReasonSupervisorRegistrationChangePending,
			phonehomedef.ReasonRCServiceUnavailable, phonehomedef.ReasonDiscoveryUnavailable, phonehomedef.ReasonDiscoveryRateLimited, phonehomedef.ReasonInvalidDiscoveryResponse:
			log.Debugf("PAR startup gate: waiting to retry: reason=%s", reason)
		default:
			if strings.HasPrefix(string(reason), "discovery_http_") {
				log.Debug("PAR startup gate: waiting to retry: reason=discovery_http_error")
			} else {
				log.Debug("PAR startup gate: waiting to retry: reason=unrecognized_observation")
			}
		}
	}
}
