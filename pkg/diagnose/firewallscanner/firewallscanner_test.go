// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package firewallscanner

import (
	"testing"

	"github.com/stretchr/testify/assert"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func Test_getRulesToCheck(t *testing.T) {
	tests := []struct {
		name          string
		conf          string
		expectedRules sourcesByRule
	}{
		{
			name:          "empty config",
			conf:          ``,
			expectedRules: sourcesByRule{},
		},
		{
			name: "snmp traps config",
			conf: `
network_devices:
  snmp_traps:
    enabled: true
    port: 1000
`,
			expectedRules: sourcesByRule{
				firewallRule{
					protocol: "UDP",
					destPort: "1000",
				}: []string{"snmp_traps"},
			},
		},
		{
			name: "snmp traps config not enabled",
			conf: `
network_devices:
  snmp_traps:
    port: 1000
`,
			expectedRules: sourcesByRule{},
		},
		{
			name: "netflow config",
			conf: `
network_devices:
  netflow:
    enabled: true
    listeners:
      - flow_type: netflow9
        port: 1000
      - flow_type: netflow5
      - flow_type: ipfix
        port: 1001
      - flow_type: sflow5
`,
			expectedRules: sourcesByRule{
				firewallRule{
					protocol: "UDP",
					destPort: "1000",
				}: []string{"netflow (netflow9)"},
				firewallRule{
					protocol: "UDP",
					destPort: "2055",
				}: []string{"netflow (netflow5)"},
				firewallRule{
					protocol: "UDP",
					destPort: "1001",
				}: []string{"netflow (ipfix)"},
				firewallRule{
					protocol: "UDP",
					destPort: "6343",
				}: []string{"netflow (sflow5)"},
			},
		},
		{
			name: "snmp traps and netflow config",
			conf: `
network_devices:
  snmp_traps:
    enabled: true
    port: 1000
  netflow:
    enabled: true
    listeners:
      - flow_type: netflow9
        port: 1000
      - flow_type: netflow5
        port: 1001
`,
			expectedRules: sourcesByRule{
				firewallRule{
					protocol: "UDP",
					destPort: "1000",
				}: []string{"snmp_traps", "netflow (netflow9)"},
				firewallRule{
					protocol: "UDP",
					destPort: "1001",
				}: []string{"netflow (netflow5)"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conf := configmock.NewFromYAML(t, tt.conf)
			rules := getRulesToCheck(conf)
			assert.Equal(t, tt.expectedRules, rules)
		})
	}
}

func Test_buildBlockingRulesDiagnosis(t *testing.T) {
	tests := []struct {
		name                      string
		diagnosisName             string
		blockingRules             sourcesByRule
		expectedStatus            diagnose.Status
		expectedName              string
		expectedDiagnosisMessages []string
	}{
		{
			name:                      "no blocking rules",
			diagnosisName:             "Diagnosis Name",
			blockingRules:             make(sourcesByRule),
			expectedStatus:            diagnose.DiagnosisSuccess,
			expectedName:              "Diagnosis Name",
			expectedDiagnosisMessages: []string{"No blocking firewall rules were found"},
		},
		{
			name:          "with blocking rules",
			diagnosisName: "Diagnosis Name",
			blockingRules: sourcesByRule{
				firewallRule{
					protocol: "UDP",
					destPort: "9162",
				}: []string{"snmp_traps", "netflow (ipfix)"},
				firewallRule{
					protocol: "UDP",
					destPort: "1234",
				}: []string{"netflow (sflow5)"},
			},
			expectedStatus: diagnose.DiagnosisWarning,
			expectedName:   "Diagnosis Name",
			expectedDiagnosisMessages: []string{
				"Blocking firewall rules were found:",
				"snmp_traps, netflow (ipfix) packets might be blocked because destination port 9162 is blocked for protocol UDP",
				"netflow (sflow5) packets might be blocked because destination port 1234 is blocked for protocol UDP",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diagnosis := buildBlockingRulesDiagnosis(tt.diagnosisName, tt.blockingRules)
			assert.Equal(t, tt.expectedStatus, diagnosis.Status)
			assert.Equal(t, tt.expectedName, diagnosis.Name)
			for _, message := range tt.expectedDiagnosisMessages {
				assert.Contains(t, diagnosis.Diagnosis, message)
			}
		})
	}
}
