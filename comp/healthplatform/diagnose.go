// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package healthplatform

import (
	"fmt"
	"strings"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

// team: agent-health

// Diagnose converts health platform issues to diagnose format.
// When diagCfg.Verbose is true, remediations are included in the output.
func Diagnose(hp healthplatformdef.Component, diagCfg diagnose.Config) []diagnose.Diagnosis {
	count, issues := hp.GetAllIssues()
	if count == 0 {
		return nil
	}

	var diagnoses []diagnose.Diagnosis
	for checkID, issue := range issues {
		if issue == nil {
			continue
		}

		status := diagnose.DiagnosisWarning
		if issue.Severity == "critical" || issue.Severity == "high" {
			status = diagnose.DiagnosisFail
		}

		d := diagnose.Diagnosis{
			Status:    status,
			Name:      issue.Title,
			Category:  checkID,
			Diagnosis: issue.Description,
		}

		if diagCfg.Verbose {
			d.Remediation = formatRemediation(issue.Remediation)
		}

		diagnoses = append(diagnoses, d)
	}

	return diagnoses
}

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
