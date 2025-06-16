// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package remote contains the code to run diagnose to be sent as payload
package remote

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/pkg/diagnose/connectivity"
)

// DiagnosisType is the type of diagnosis
type DiagnosisType int

const (
	// ConnectivityCheck is the type of diagnosis for connectivity check
	ConnectivityCheck DiagnosisType = iota
)

// DiagnosisPayload contains the result payload of the diagnosis
type DiagnosisPayload struct {
	Status        diagnose.Status `json:"result"`
	DiagnosisType DiagnosisType   `json:"diagnosis_type"`
	Name          string          `json:"name"`
	Error         string          `json:"diagnosis"`
}

// Run runs the remote diagnose suite.
func Run(
	diagnoseConfig diagnose.Config,
	config config.Component,
) []DiagnosisPayload {

	lightSuite := diagnose.Suites{
		"internal-": func(_ diagnose.Config) []diagnose.Diagnosis {
			return connectivity.DiagnoseDatadogURL(config)
		},
	}

	return runDiagnoses(lightSuite, diagnoseConfig)
}

// runDiagnoses runs the diagnoses for the given suites and returns the diagnosis payloads
func runDiagnoses(suites diagnose.Suites, diagnoseConfig diagnose.Config) []DiagnosisPayload {
	diagnosisPayloads := make([]DiagnosisPayload, 0)

	for _, diag := range suites {
		diagnoses := diag(diagnoseConfig)
		for _, diagnosis := range diagnoses {
			diagnosisPayloads = append(diagnosisPayloads, DiagnosisPayload{
				Status:        diagnosis.Status,
				DiagnosisType: ConnectivityCheck,
				Name:          diagnosis.Name,
				Error:         diagnosis.Diagnosis,
			})
		}
	}
	return diagnosisPayloads
}
