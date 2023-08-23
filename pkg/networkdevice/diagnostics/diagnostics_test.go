// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package diagnostics

import (
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestReportDeviceDiagnostics(t *testing.T) {
	diagnostics := NewDeviceDiagnostics("default:1.2.3.4.5")

	diagnostics.Add("error", "TEST_ERROR_DIAG", "This is a test error diagnostic")

	diagnosticsMetadata := diagnostics.ReportDiagnostics()

	expectedMetadata := []metadata.DiagnosticMetadata{{
		ResourceType: "ndm_device",
		ResourceID:   "default:1.2.3.4.5",
		Diagnostics: []metadata.Diagnostic{
			{
				Severity:   "error",
				ErrorCode:  "TEST_ERROR_DIAG",
				Diagnostic: "This is a test error diagnostic",
			},
		}}}

	assert.Equal(t, expectedMetadata, diagnosticsMetadata)
}

func TestReportCLIDiagnostics(t *testing.T) {
	diagnostics := NewDeviceDiagnostics("default:1.2.3.4.5")

	diagnostics.Add("error", "TEST_ERROR_DIAG", "This is a test error diagnostic")
	diagnostics.ReportDiagnostics()

	diagnosticsCLI := diagnostics.ConvertToCLI()

	expected := []diagnosis.Diagnosis{{
		Result:    diagnosis.DiagnosisFail,
		Name:      "ndm_device - default:1.2.3.4.5 - TEST_ERROR_DIAG",
		Diagnosis: "This is a test error diagnostic",
	}}

	assert.Equal(t, expected, diagnosticsCLI)
}
