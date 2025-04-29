// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package firewallscanner

import (
	"testing"

	"go.uber.org/fx"
	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
)

func Test_getRulesToCheck(t *testing.T) {
	tests := []struct {
		name          string
		bConf         []byte
		expectedRules sourcesByRule
	}{
		{
			name:          "empty config",
			bConf:         []byte(``),
			expectedRules: sourcesByRule{},
		},
		{
			name: "snmp traps config",
			bConf: []byte(`
network_devices:
  snmp_traps:
    enabled: true
    port: 1000
`),
			expectedRules: sourcesByRule{
				firewallRule{
					protocol: "UDP",
					destPort: "1000",
				}: []string{"snmp_traps"},
			},
		},
		{
			name: "snmp traps config not enabled",
			bConf: []byte(`
network_devices:
  snmp_traps:
    port: 1000
`),
			expectedRules: sourcesByRule{},
		},
		{
			name: "netflow config",
			bConf: []byte(`
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
`),
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
			bConf: []byte(`
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
`),
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
			rawConf := make(map[string]any)
			require.NoError(t, yaml.Unmarshal(tt.bConf, &rawConf))

			conf := fxutil.Test[config.Component](t,
				config.MockModule(),
				fx.Replace(config.MockParams{Overrides: rawConf}),
			)

			rules := getRulesToCheck(conf)
			assert.Equal(t, tt.expectedRules, rules)
		})
	}
}

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
					firewallRule: firewallRule{
						protocol: "UDP",
						destPort: "9162",
					},
					sources: []string{
						"snmp_traps",
						"netflow (ipfix)",
					},
				},
				{
					firewallRule: firewallRule{
						protocol: "UDP",
						destPort: "1234",
					},
					sources: []string{
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
