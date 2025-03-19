// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package collector

import (
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
)

// Diagnose returns the collector diagnoses
func Diagnose(collectorComponent Component, log log.Component) []diagnose.Diagnosis {
	var diagnoses []diagnose.Diagnosis
	// get list of checks
	checks := collectorComponent.GetChecks()

	// get diagnoses from each
	for _, ch := range checks {
		if ch.Interval() == 0 {
			log.Infof("Ignoring long running check %s", ch.String())
			continue
		}
		instanceDiagnoses := GetInstanceDiagnoses(ch)
		diagnoses = append(diagnoses, instanceDiagnoses...)
	}

	return diagnoses
}

// GetInstanceDiagnoses returns the diagnoses for a check instance
func GetInstanceDiagnoses(instance check.Check) []diagnose.Diagnosis {
	// Get diagnoses
	diagnoses, err := instance.GetDiagnoses()
	if err != nil {
		// return as diagnosis.DiagnosisUnexpectedError diagnosis
		return []diagnose.Diagnosis{
			{
				Status:    diagnose.DiagnosisUnexpectedError,
				Name:      string(instance.ID()),
				Category:  instance.String(),
				Diagnosis: "Check Diagnose fails with unexpected errors",
				RawError:  err.Error(),
			},
		}
	}

	// Set category as check name if it was not set
	if len(diagnoses) > 0 {
		for i, d := range diagnoses {
			if len(d.Category) == 0 {
				diagnoses[i].Category = instance.String()
			}
		}
	}

	return diagnoses
}
