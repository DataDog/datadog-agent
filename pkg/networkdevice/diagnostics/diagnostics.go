// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package diagnostics

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

// Diagnostics hold diagnostics for a NDM resource
type Diagnostics struct {
	resourceType    string
	resourceID      string
	diagnostics     []metadata.Diagnostic
	prevDiagnostics []metadata.Diagnostic
}

var severityMap = map[string]diagnosis.Result{
	"success": diagnosis.DiagnosisSuccess,
	"error":   diagnosis.DiagnosisFail,
	"warn":    diagnosis.DiagnosisWarning,
}

// NewDeviceDiagnostics returns a new Diagnostics for a NDM device resource
func NewDeviceDiagnostics(deviceID string) *Diagnostics {
	return &Diagnostics{
		resourceType: "ndm_device",
		resourceID:   deviceID,
	}
}

// Add adds a diagnostic
func (d *Diagnostics) Add(result string, errorCode string, message string) {
	d.diagnostics = append(d.diagnostics, metadata.Diagnostic{
		Severity:   result,
		ErrorCode:  errorCode,
		Diagnostic: message,
	})
}

// ReportDiagnostics returns diagnostics metadata
func (d *Diagnostics) ReportDiagnostics() []metadata.DiagnosticMetadata {
	d.prevDiagnostics = d.diagnostics
	d.diagnostics = nil

	if d.prevDiagnostics == nil {
		return nil
	}

	return []metadata.DiagnosticMetadata{{ResourceType: d.resourceType, ResourceID: d.resourceID, Diagnostics: d.prevDiagnostics}}
}

// ConvertToCLI converts diagnostics to diagnose CLI format
func (d *Diagnostics) ConvertToCLI() []diagnosis.Diagnosis {
	var cliDiagnostics []diagnosis.Diagnosis

	for _, diagnostic := range d.prevDiagnostics {
		cliDiagnostics = append(cliDiagnostics, diagnosis.Diagnosis{
			Result:    severityMap[diagnostic.Severity],
			Name:      fmt.Sprintf("%s - %s - %s", d.resourceType, d.resourceID, diagnostic.ErrorCode),
			Diagnosis: diagnostic.Diagnostic,
		})
	}

	return cliDiagnostics
}
