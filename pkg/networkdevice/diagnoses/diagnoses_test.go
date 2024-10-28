// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package diagnoses

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestReportDeviceDiagnoses(t *testing.T) {
	diagnoses := NewDeviceDiagnoses("default:1.2.3.4.5")

	diagnoses.Add("error", "TEST_ERROR_DIAG", "This is a test error diagnosis")

	diagnosisMetadata := diagnoses.Report()

	expectedMetadata := []metadata.DiagnosisMetadata{{
		ResourceType: "device",
		ResourceID:   "default:1.2.3.4.5",
		Diagnoses: []metadata.Diagnosis{
			{
				Severity: "error",
				Code:     "TEST_ERROR_DIAG",
				Message:  "This is a test error diagnosis",
			},
		}}}

	assert.Equal(t, expectedMetadata, diagnosisMetadata)
}

func TestReportDeviceDiagnosesReset(t *testing.T) {
	diagnoses := NewDeviceDiagnoses("default:1.2.3.4.5")

	diagnoses.Add("error", "TEST_ERROR_DIAG", "This is a test error diagnosis")

	diagnosisMetadata := diagnoses.Report()

	expectedMetadata := []metadata.DiagnosisMetadata{{
		ResourceType: "device",
		ResourceID:   "default:1.2.3.4.5",
		Diagnoses: []metadata.Diagnosis{{
			Severity: "error",
			Code:     "TEST_ERROR_DIAG",
			Message:  "This is a test error diagnosis",
		}},
	}}

	assert.Equal(t, expectedMetadata, diagnosisMetadata)

	diagnosisResetedMetadata := diagnoses.Report()

	expectedResetedMetadata := []metadata.DiagnosisMetadata{{
		ResourceType: "device",
		ResourceID:   "default:1.2.3.4.5",
		Diagnoses:    nil,
	}}

	assert.Equal(t, expectedResetedMetadata, diagnosisResetedMetadata)
}

func TestReportAsAgentDiagnoses(t *testing.T) {
	diagnoses := NewDeviceDiagnoses("default:1.2.3.4.5")

	diagnoses.Add("error", "TEST_ERROR_DIAG", "This is a test error diagnosis")
	diagnoses.Report()

	diagnosesCLI := diagnoses.ReportAsAgentDiagnoses()

	expected := []diagnosis.Diagnosis{{
		Result:    diagnosis.DiagnosisFail,
		Name:      "NDM device - default:1.2.3.4.5 - TEST_ERROR_DIAG",
		Diagnosis: "This is a test error diagnosis",
	}}

	assert.Equal(t, expected, diagnosesCLI)
}
