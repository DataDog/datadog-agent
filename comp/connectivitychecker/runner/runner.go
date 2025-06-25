// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package runner implements the connectivity checker component
package runner

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
)

type status string

const (
	success status = "success"
	failure status = "failure"
)

// DiagnosisPayload contains the result payload of the diagnosis
type DiagnosisPayload struct {
	Status      status            `json:"status"`
	Description string            `json:"description"`
	Error       string            `json:"error,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Diagnose runs the connectivity checks
func Diagnose(
	config config.Component,
) (map[string][]DiagnosisPayload, error) {

	return map[string][]DiagnosisPayload{
		"connectivity": diagnoseConnectivity(config),
	}, nil
}
