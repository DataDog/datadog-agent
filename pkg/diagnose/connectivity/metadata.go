// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package connectivity contains logic for connectivity troubleshooting between the Agent
// and Datadog endpoints. It uses HTTP request to contact different endpoints and displays
// some results depending on endpoints responses, if any.
package connectivity

import (
	"fmt"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
)

func init() {
	diagnosis.Register("connectivity-datadog-autodiscovery", diagnoseMetadataAutodiscoveryConnectivity)
}

func diagnoseMetadataAutodiscoveryConnectivity(cfg diagnosis.Config, senderManager sender.SenderManager) []diagnosis.Diagnosis {
	if len(diagnosis.MetadataAvailCatalog) == 0 {
		return nil
	}

	var sortedDiagnosis []string
	for name := range diagnosis.MetadataAvailCatalog {
		sortedDiagnosis = append(sortedDiagnosis, name)
	}
	sort.Strings(sortedDiagnosis)

	var diagnoses []diagnosis.Diagnosis
	for _, name := range sortedDiagnosis {
		err := diagnosis.MetadataAvailCatalog[name]()

		// Will always add successful diagnosis because particular environment is auto-discovered
		// and may not exist and or configured but knowing if we can or cannot connect to it
		// could be still beneficial
		var diagnosisString string
		if err == nil {
			diagnosisString = fmt.Sprintf("Successfully connected to %s environment", name)
		} else {
			diagnosisString = fmt.Sprintf("[Ignore if not applied] %s", err.Error())
		}

		diagnoses = append(diagnoses, diagnosis.Diagnosis{
			Result:    diagnosis.DiagnosisSuccess,
			Name:      name,
			Diagnosis: diagnosisString,
		})
	}

	return diagnoses
}
