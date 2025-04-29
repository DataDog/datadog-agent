// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package firewallscanner

import (
	"testing"

	"github.com/stretchr/testify/assert"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
)

func Test_buildBlockingRulesDiagnosis(t *testing.T) {
	tests := []struct {
		name              string
		diagnosisName     string
		blockingRules     []blockingRule
		expectedDiagnosis diagnose.Diagnosis
	}{
		{
			name:          "no blocking rules",
			diagnosisName: "Diagnosis Name",
			blockingRules: []blockingRule{},
			expectedDiagnosis: diagnose.Diagnosis{
				Status:    diagnose.DiagnosisSuccess,
				Name:      "Diagnosis Name",
				Diagnosis: "No blocking firewall rules were found",
			},
		},
		{
			name:          "with blocking rules",
			diagnosisName: "Diagnosis Name",
			blockingRules: []blockingRule{
				{
					Protocol: "UDP",
					Port:     "9162",
					ForIntegrations: []string{
						"snmp_traps",
						"netflow (ipfix)",
					},
				},
				{
					Protocol: "UDP",
					Port:     "1234",
					ForIntegrations: []string{
						"netflow (sflow5)",
					},
				},
			},
			expectedDiagnosis: diagnose.Diagnosis{
				Status: diagnose.DiagnosisWarning,
				Name:   "Diagnosis Name",
				Diagnosis: "Blocking firewall rules were found:\n" +
					"snmp_traps, netflow (ipfix) packets might be blocked because destination port 9162 is blocked for protocol UDP\n" +
					"netflow (sflow5) packets might be blocked because destination port 1234 is blocked for protocol UDP\n",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnosis := buildBlockingRulesDiagnosis(tt.diagnosisName, tt.blockingRules)
			assert.Equal(t, tt.expectedDiagnosis, diagnosis)
		})
	}
}
