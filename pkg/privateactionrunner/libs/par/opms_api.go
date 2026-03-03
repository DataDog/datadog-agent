// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package par

import (
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
)

// CreateRunnerRequest represents the request to create a runner with API key auth
type CreateRunnerRequest struct {
	ID           string       `jsonapi:"primary,createRunnerRequest"`
	RunnerName   string       `json:"runner_name" jsonapi:"attribute" validate:"required"`
	RunnerModes  []modes.Mode `json:"runner_modes" jsonapi:"attribute" validate:"gt=0,max=2,dive"`
	RunnerHost   string       `json:"runner_host" jsonapi:"attribute" validate:"omitempty,hostname"`
	PublicKeyPEM string       `json:"public_key_pem" jsonapi:"attribute" validate:"required"`
}

// CreateRunnerResponse represents the response for runner creation
type CreateRunnerResponse struct {
	ID          string   `jsonapi:"primary,createRunnerResponse"`
	RunnerID    string   `json:"runner_id" jsonapi:"attribute"`
	OrgID       int64    `json:"org_id" jsonapi:"attribute"`
	RunnerModes []string `json:"runner_modes" jsonapi:"attribute"`
}
