// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package healthplatformimpl

import (
	"fmt"
	"strings"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/def"
)

// Diagnose converts health platform issues to diagnose format
func Diagnose(healthplatformComp healthplatformdef.Component) []diagnose.Diagnosis {
	count, issues := healthplatformComp.GetAllIssues()

	// If no issues, return nil (nothing to report)
	if count == 0 {
		return nil
	}

	// Convert each health platform issue to a diagnosis
	var diagnoses []diagnose.Diagnosis
	for checkID, issue := range issues {
		if issue == nil {
			continue
		}

		status := diagnose.DiagnosisWarning
		if issue.Severity == "critical" || issue.Severity == "high" {
			status = diagnose.DiagnosisFail
		}

		diagnoses = append(diagnoses, diagnose.Diagnosis{
			Status:      status,
			Name:        issue.Title,
			Category:    checkID,
			Diagnosis:   issue.Description,
			Remediation: formatRemediation(issue.Remediation),
		})
	}

	return diagnoses
}

// formatRemediation formats a Remediation struct into a readable string
func formatRemediation(r *healthplatformpayload.Remediation) string {
	if r == nil {
		return ""
	}

	var parts []string
	if r.Summary != "" {
		parts = append(parts, r.Summary)
	}

	for _, step := range r.Steps {
		if step != nil && step.Text != "" {
			parts = append(parts, fmt.Sprintf("  %d. %s", step.Order, step.Text))
		}
	}

	return strings.Join(parts, "\n")
}
