// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agentaccountcheck

import (
	"testing"

	"github.com/stretchr/testify/assert"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
)

func TestDiagnose(t *testing.T) {
	diagnoses := Diagnose()

	// Should always return at least one diagnosis
	assert.NotEmpty(t, diagnoses)

	// All diagnoses should have required fields
	for _, diagnosis := range diagnoses {
		assert.NotEmpty(t, diagnosis.Name, "Diagnosis name should not be empty")
		assert.NotEmpty(t, diagnosis.Diagnosis, "Diagnosis message should not be empty")
		assert.NotEmpty(t, diagnosis.Category, "Diagnosis category should not be empty")
		assert.True(t, diagnosis.Status >= diagnose.DiagnosisSuccess && diagnosis.Status <= diagnose.DiagnosisUnexpectedError, "Diagnosis status should be valid")
	}
}

func TestDiagnosesHaveAgentAccountCheckCategory(t *testing.T) {
	diagnoses := Diagnose()

	// All diagnoses should have agent-account-check category
	for _, diagnosis := range diagnoses {
		assert.Equal(t, "agent-account-check", diagnosis.Category)
	}
}

func TestDiagnoseContent(t *testing.T) {
	diagnoses := Diagnose()

	t.Logf("=== Agent Account Check Diagnosis Results ===")
	for i, diagnosis := range diagnoses {
		t.Logf("Diagnosis %d:", i+1)
		t.Logf("  Name: %s", diagnosis.Name)
		t.Logf("  Status: %s", diagnosis.Status.ToString(false))
		t.Logf("  Diagnosis: %s", diagnosis.Diagnosis)
		if diagnosis.RawError != "" {
			t.Logf("  RawError: %s", diagnosis.RawError)
		}
		if diagnosis.Remediation != "" {
			t.Logf("  Remediation: %s", diagnosis.Remediation)
		}
		t.Logf("")
	}

	// Check if we have the expected diagnosis types on Windows
	hasGroupsDiagnosis := false
	hasRightsDiagnosis := false

	for _, diagnosis := range diagnoses {
		if diagnosis.Name == "Agent Account Group Membership" {
			hasGroupsDiagnosis = true
		}
		if diagnosis.Name == "Agent Account Rights" {
			hasRightsDiagnosis = true
		}
	}

	// On Windows, we should have both group and rights diagnoses
	// On other platforms, we should have at least one diagnosis (the stub)
	if len(diagnoses) >= 2 {
		assert.True(t, hasGroupsDiagnosis, "Should have an Agent Account Group Membership diagnosis")
		assert.True(t, hasRightsDiagnosis, "Should have an Agent Account Rights diagnosis")
	}
}

// TestNoCriticalErrors verifies that the diagnose function doesn't return unexpected errors
// for normal permission checking scenarios (following integration test expectations)
func TestNoCriticalErrors(t *testing.T) {
	diagnoses := Diagnose()

	unexpectedErrors := 0
	failures := 0
	warnings := 0
	successes := 0

	for _, diagnosis := range diagnoses {
		switch diagnosis.Status {
		case diagnose.DiagnosisSuccess:
			successes++
		case diagnose.DiagnosisFail:
			failures++
		case diagnose.DiagnosisWarning:
			warnings++
		case diagnose.DiagnosisUnexpectedError:
			unexpectedErrors++
			if diagnosis.RawError != "" {
				t.Errorf("Unexpected error in diagnosis '%s': %s\n  RawError: %s", diagnosis.Name, diagnosis.Diagnosis, diagnosis.RawError)
			} else {
				t.Errorf("Unexpected error in diagnosis '%s': %s", diagnosis.Name, diagnosis.Diagnosis)
			}
		}
	}

	t.Logf("Results: %d success, %d fail, %d warning, %d unexpected error", successes, failures, warnings, unexpectedErrors)

	// Following the pattern from integration tests - no unexpected errors should occur
	// during normal operation (access denied should be warnings, not unexpected errors)
	assert.Equal(t, 0, unexpectedErrors, "Should have no unexpected errors - access denied should be treated as warnings")
}
