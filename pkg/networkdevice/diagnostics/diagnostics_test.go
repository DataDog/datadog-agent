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
		ResourceId:   "default:1.2.3.4.5",
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
		Name:      "TEST_ERROR_DIAG",
		Diagnosis: "This is a test error diagnostic",
	}}

	assert.Equal(t, expected, diagnosticsCLI)
}
