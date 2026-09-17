// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package common

import (
	"errors"
	"time"
)

// PAREnrollmentPurpose is the only supported workload authorization purpose.
const PAREnrollmentPurpose = "private_action_runner_enrollment"

// ErrWorkloadAuthorizationUnavailable indicates that the requested delegated-auth
// instance has not been configured. A WIF-bound runner must not fall back to an API key.
var ErrWorkloadAuthorizationUnavailable = errors.New("workload authorization is unavailable")

// WorkloadAuthorization is a short-lived ETS assertion. Keep Token in memory only;
// the remaining fields are informational and must never authorize enrollment themselves.
type WorkloadAuthorization struct {
	Token           string    `json:"-"`
	ExpiresAt       time.Time `json:"expires_at"`
	OrgID           uint64    `json:"org_id"`
	OrgUUID         string    `json:"org_uuid"`
	Provider        string    `json:"provider"`
	IntakeMappingID string    `json:"intake_mapping_id"`
	StablePrincipal string    `json:"stable_principal"`
}

// String and GoString keep accidental formatted logging from exposing the assertion.
func (a WorkloadAuthorization) String() string   { return "WorkloadAuthorization{token:[redacted]}" }
func (a WorkloadAuthorization) GoString() string { return a.String() }
