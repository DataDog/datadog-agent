// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package checker runs the diagnostics for the connectivity checker component
package checker

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/pkg/diagnose/connectivity"
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
	diagnoses, err := connectivity.DiagnoseInventory(ctx, config, log)
	if err != nil {
		return nil, err
	}

	diagnosesPayload := []DiagnosisPayload{}
	for _, diagnosis := range diagnoses {
		payload := DiagnosisPayload{
			Status:      toStatus(diagnosis.Status),
			Description: diagnosis.Name,
			Metadata:    diagnosis.Metadata,
		}
		if diagnosis.Status == diagnose.DiagnosisFail {
			payload.Error = diagnosis.Diagnosis
		}
		diagnosesPayload = append(diagnosesPayload, payload)
	}

	eventplatformDiagnoses := eventplatformimpl.Diagnose()
	for _, diagnosis := range eventplatformDiagnoses {
		payload := DiagnosisPayload{
			Status:      toStatus(diagnosis.Status),
			Description: diagnosis.Name,
			Metadata:    diagnosis.Metadata,
		}
		if diagnosis.Status == diagnose.DiagnosisFail {
			payload.Error = diagnosis.Diagnosis
		}
		diagnosesPayload = append(diagnosesPayload, payload)
	}

	for _, diagnosis := range connectivity.Diagnose(diagnose.Config{}, log) {
		payload := DiagnosisPayload{
			Status:      toStatus(diagnosis.Status),
			Description: diagnosis.Name,
			Metadata:    diagnosis.Metadata,
		}
		if diagnosis.Status == diagnose.DiagnosisFail {
			payload.Error = diagnosis.Diagnosis
		}
		diagnosesPayload = append(diagnosesPayload, payload)
	}

	return map[string][]DiagnosisPayload{
		"connectivity": diagnosesPayload,
	}, nil
}

func toStatus(ds diagnose.Status) status {
	switch ds {
	case diagnose.DiagnosisSuccess:
		return success
	case diagnose.DiagnosisFail:
		return failure
	default:
		return failure
	}
}
