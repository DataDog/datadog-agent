// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checker implements the connectivity checker component
package checker

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
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

// Check runs the connectivity checks
func Check(
	ctx context.Context,
	config config.Component,
	log log.Component,
) (map[string][]DiagnosisPayload, error) {
	diagnosesPayload, err := DiagnoseInventory(ctx, config, log)
	if err != nil {
		return nil, err
	}

	return map[string][]DiagnosisPayload{
		"connectivity": diagnosesPayload,
	}, nil
}
