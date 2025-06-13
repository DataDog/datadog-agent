// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package remote contains the code to run diagnose to be sent as payload
package remote

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/pkg/diagnose/connectivity"
)

// diagnosisType is the type of diagnosis
type diagnosisType int

const (
	// connectivityCheck is the type of diagnosis for connectivity check
	connectivityCheck diagnosisType = iota
)

// ConnectivityCheckPayload contains the result payload of the diagnosis
type ConnectivityCheckPayload struct {
	Status        diagnose.Status `json:"result"`
	DiagnosisType diagnosisType   `json:"check_type"`
	Name          string          `json:"name"`
	Error         string          `json:"error,omitempty"`
}

// Run the connectivity checks
func Run(
	diagnoseConfig diagnose.Config,
	diagnoseComponent diagnose.Component,
	config config.Component,
) ([]ConnectivityCheckPayload, error) {

	remoteCheckSuites := diagnose.Suites{
		"internal-connectivity": func(_ diagnose.Config) []diagnose.Diagnosis {
			return connectivity.DiagnoseDatadogURL(config)
		},
	}

	res, err := diagnoseComponent.RunLocalSuite(remoteCheckSuites, diagnoseConfig)
	if err != nil {
		return nil, fmt.Errorf("Error while running diagnostics: %s", err)
	}

	payloads := make([]ConnectivityCheckPayload, 0)

	for _, run := range res.Runs {
		for _, diagnosis := range run.Diagnoses {
			payloads = append(payloads, ConnectivityCheckPayload{
				Status:        diagnosis.Status,
				DiagnosisType: connectivityCheck,
				Name:          diagnosis.Name,
				Error:         diagnosis.RawError,
			})
		}
	}
	return payloads, nil
}
