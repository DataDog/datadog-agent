// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package firewallscanner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_checkBlockingRulesWindows(t *testing.T) {
	tests := []struct {
		name                  string
		output                []byte
		rulesToCheck          rulesToCheckByPort
		expectedBlockingRules []blockingRule
		expectError           bool
	}{
		{
			name:        "invalid output",
			output:      []byte(`invalid`),
			expectError: true,
		},
		{
			name:                  "no rules",
			output:                []byte(``),
			rulesToCheck:          rulesToCheckByPort{},
			expectedBlockingRules: nil,
		},
		{
			name: "blocked protocol and port",
			output: []byte(`{
    "direction":  1,
    "protocol":  "UDP",
    "localPort":  "9162"
}`),
			rulesToCheck: rulesToCheckByPort{
				"9162": {
					ProtocolsSet: map[string]struct{}{"UDP": {}},
					Sources:      []string{"snmp_traps"},
				},
			},
			expectedBlockingRules: []blockingRule{
				{
					Protocol: "UDP",
					DestPort: "9162",
					Sources:  []string{"snmp_traps"},
				},
			},
		},
		{
			name: "blocked protocol but not port",
			output: []byte(`{
    "direction":  1,
    "protocol":  "UDP",
    "localPort":  "9160"
}`),
			rulesToCheck: rulesToCheckByPort{
				"9162": {
					ProtocolsSet: map[string]struct{}{"UDP": {}},
					Sources:      []string{"snmp_traps"},
				},
			},
			expectedBlockingRules: nil,
		},
		{
			name: "blocked port but not protocol",
			output: []byte(`{
    "direction":  1,
    "protocol":  "TCP",
    "localPort":  "9162"
}`),
			rulesToCheck: rulesToCheckByPort{
				"9162": {
					ProtocolsSet: map[string]struct{}{"UDP": {}},
					Sources:      []string{"snmp_traps"},
				},
			},
			expectedBlockingRules: nil,
		},
		{
			name: "multiple rules with multiple blocked",
			output: []byte(`[
    {
        "direction":  1,
        "protocol":  "UDP",
        "localPort":  "9162"
    },
    {
        "direction":  1,
        "protocol":  "UDP",
        "localPort":  "1000"
    },
    {
        "direction":  1,
        "protocol":  "UDP",
        "localPort":  "2000"
    }
]`),
			rulesToCheck: rulesToCheckByPort{
				"9162": {
					ProtocolsSet: map[string]struct{}{"UDP": {}},
					Sources:      []string{"snmp_traps", "netflow (netflow5)"},
				},
				"1111": {
					ProtocolsSet: map[string]struct{}{"UDP": {}},
					Sources:      []string{"netflow (netflow9)"},
				},
				"2000": {
					ProtocolsSet: map[string]struct{}{"UDP": {}},
					Sources:      []string{"netflow (ipfix)"},
				},
			},
			expectedBlockingRules: []blockingRule{
				{
					Protocol: "UDP",
					DestPort: "9162",
					Sources:  []string{"snmp_traps", "netflow (netflow5)"},
				},
				{
					Protocol: "UDP",
					DestPort: "2000",
					Sources:  []string{"netflow (ipfix)"},
				},
			},
		},
		{
			name: "multiple rules with no blocked",
			output: []byte(`[
    {
        "direction":  2,
        "protocol":  "UDP",
        "localPort":  "1000"
    },
    {
        "direction":  1,
        "protocol":  "TCP",
        "localPort":  "2000"
    },
    {
        "direction":  1,
        "protocol":  "UDP",
        "localPort":  "3000"
    }
]`),
			rulesToCheck: rulesToCheckByPort{
				"9162": {
					ProtocolsSet: map[string]struct{}{"UDP": {}},
					Sources:      []string{"snmp_traps", "netflow (netflow5)"},
				},
				"1000": {
					ProtocolsSet: map[string]struct{}{"UDP": {}},
					Sources:      []string{"netflow (netflow9)"},
				},
				"2000": {
					ProtocolsSet: map[string]struct{}{"UDP": {}},
					Sources:      []string{"netflow (ipfix)"},
				},
			},
			expectedBlockingRules: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blockingRules, err := checkBlockingRulesWindows(tt.output, tt.rulesToCheck)
			if !tt.expectError {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedBlockingRules, blockingRules)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
