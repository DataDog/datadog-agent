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

	diagnosisMetadata := diagnoses.ReportDiagnosis()

	expectedMetadata := []metadata.DiagnosisMetadata{{
		ResourceType: "ndm_device",
		ResourceID:   "default:1.2.3.4.5",
		Diagnoses: []metadata.Diagnosis{
			{
				Severity:  "error",
				ErrorCode: "TEST_ERROR_DIAG",
				Diagnosis: "This is a test error diagnosis",
			},
		}}}

	assert.Equal(t, expectedMetadata, diagnosisMetadata)
}

func TestReportDeviceDiagnosesEmptyArray(t *testing.T) {
	diagnoses := NewDeviceDiagnoses("default:1.2.3.4.5")

	diagnoses.Add("error", "TEST_ERROR_DIAG", "This is a test error diagnosis")

	diagnoses.ReportDiagnosis()

	diagnosisMetadata := diagnoses.ReportDiagnosis()

	expectedMetadata := []metadata.DiagnosisMetadata{{
		ResourceType: "ndm_device",
		ResourceID:   "default:1.2.3.4.5",
		Diagnoses:    []metadata.Diagnosis{},
	}}

	// Expecting an empty array to reset diagnoses stored in backend
	assert.Equal(t, expectedMetadata, diagnosisMetadata)
}

func TestReportCLIDiagnoses(t *testing.T) {
	diagnoses := NewDeviceDiagnoses("default:1.2.3.4.5")

	diagnoses.Add("error", "TEST_ERROR_DIAG", "This is a test error diagnosis")
	diagnoses.ReportDiagnosis()

	diagnosesCLI := diagnoses.ConvertToCLI()

	expected := []diagnosis.Diagnosis{{
		Result:    diagnosis.DiagnosisFail,
		Name:      "ndm_device - default:1.2.3.4.5 - TEST_ERROR_DIAG",
		Diagnosis: "This is a test error diagnosis",
	}}

	assert.Equal(t, expected, diagnosesCLI)
}
