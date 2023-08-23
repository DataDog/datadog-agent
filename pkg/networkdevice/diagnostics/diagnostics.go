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

type Diagnostics struct {
	resourceType    string
	resourceId      string
	diagnostics     []metadata.Diagnostic
	prevDiagnostics []metadata.Diagnostic
}

var severityMap = map[string]diagnosis.Result{
	"success": diagnosis.DiagnosisSuccess,
	"error":   diagnosis.DiagnosisFail,
	"warn":    diagnosis.DiagnosisWarning,
}

func NewDeviceDiagnostics(deviceId string) *Diagnostics {
	return &Diagnostics{
		resourceType: "ndm_device",
		resourceId:   deviceId,
	}
}

func (d *Diagnostics) Add(result string, errorCode string, message string) {
	d.diagnostics = append(d.diagnostics, metadata.Diagnostic{
		Severity:   result,
		ErrorCode:  errorCode,
		Diagnostic: message,
	})
}

func (d *Diagnostics) ReportDiagnostics() []metadata.DiagnosticMetadata {
	d.prevDiagnostics = d.diagnostics
	d.diagnostics = nil

	if d.prevDiagnostics == nil {
		return nil
	}

	return []metadata.DiagnosticMetadata{{ResourceType: d.resourceType, ResourceId: d.resourceId, Diagnostics: d.prevDiagnostics}}
}

func (d *Diagnostics) ConvertToCLI() []diagnosis.Diagnosis {
	var cliDiagnostics []diagnosis.Diagnosis

	for _, diagnostic := range d.prevDiagnostics {
		cliDiagnostics = append(cliDiagnostics, diagnosis.Diagnosis{
			Result:    severityMap[diagnostic.Severity],
			Name:      fmt.Sprintf("%s - %s - %s", d.resourceType, d.resourceId, diagnostic.ErrorCode),
			Diagnosis: diagnostic.Diagnostic,
		})
	}

	return cliDiagnostics
}
