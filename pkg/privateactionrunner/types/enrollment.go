// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package types

// EnrollmentData represents the unified enrollment information needed to create a runner configuration
type EnrollmentData struct {
	DDBaseURL        string   `json:"ddBaseUrl"`
	OrgID            int64    `json:"orgId"`
	RunnerID         string   `json:"runnerId"`
	RunnerModes      []string `json:"runnerModes"`
	ActionsAllowlist []string `json:"actionsAllowlist"`
	PrivateKey       []byte   `json:"privateKey"`
}

// EnrollmentOptions represents options for the enrollment process
type EnrollmentOptions struct {
	EnrollOnly   bool   // Enrolls and exits (replaces --enroll-and-print-config)
	ResultFormat string // Output format: "config", "helm-values", "env" (default: "config")
	ResultOutput string // File path to save enrollment result (default: stdout)
	UseAPIKey    bool   // Use API key authentication instead of enrollment token
}
