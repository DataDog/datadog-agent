// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package diagnoses implements the diagnosis collection for NDM resources
package diagnoses

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"sync"
)

// Diagnoses hold diagnoses for a NDM resource
type Diagnoses struct {
	resourceType           string
	resourceID             string
	diagnoses              []metadata.Diagnosis
	lastFlushedDiagnoses   []metadata.Diagnosis
	lastFlushedDiagnosesMu sync.Mutex
}

var severityMap = map[string]diagnosis.Result{
	"success": diagnosis.DiagnosisSuccess,
	"error":   diagnosis.DiagnosisFail,
	"warn":    diagnosis.DiagnosisWarning,
}

// NewDeviceDiagnoses returns a new Diagnoses for a NDM device resource
func NewDeviceDiagnoses(deviceID string) *Diagnoses {
	return &Diagnoses{
		resourceType: "device",
		resourceID:   deviceID,
	}
}

// Add adds a diagnoses
func (d *Diagnoses) Add(result string, code string, message string) {
	d.diagnoses = append(d.diagnoses, metadata.Diagnosis{
		Severity: result,
		Code:     code,
		Message:  message,
	})
}

// Report returns diagnosis metadata
func (d *Diagnoses) Report() []metadata.DiagnosisMetadata {
	d.lastFlushedDiagnosesMu.Lock()
	d.lastFlushedDiagnoses = d.diagnoses
	d.lastFlushedDiagnosesMu.Unlock()

	d.diagnoses = nil

	return []metadata.DiagnosisMetadata{{ResourceType: d.resourceType, ResourceID: d.resourceID, Diagnoses: d.lastFlushedDiagnoses}}
}

// ReportAsAgentDiagnoses converts diagnoses to Agent diagnose CLI format
func (d *Diagnoses) ReportAsAgentDiagnoses() []diagnosis.Diagnosis {
	var cliDiagnoses []diagnosis.Diagnosis

	d.lastFlushedDiagnosesMu.Lock()
	diagnoses := d.lastFlushedDiagnoses
	d.lastFlushedDiagnosesMu.Unlock()

	for _, diag := range diagnoses {
		cliDiagnoses = append(cliDiagnoses, diagnosis.Diagnosis{
			Result:    severityMap[diag.Severity],
			Name:      fmt.Sprintf("NDM %s - %s - %s", d.resourceType, d.resourceID, diag.Code),
			Diagnosis: diag.Message,
		})
	}

	return cliDiagnoses
}
