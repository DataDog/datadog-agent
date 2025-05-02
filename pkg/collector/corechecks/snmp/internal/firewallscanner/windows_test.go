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
		rulesToCheck          sourcesByRule
		expectedBlockingRules sourcesByRule
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
			rulesToCheck:          sourcesByRule{},
			expectedBlockingRules: nil,
		},
		{
			name: "blocked protocol and port",
			output: []byte(`{
    "direction":  1,
    "protocol":  "UDP",
    "localPort":  "9162"
}`),
			rulesToCheck: sourcesByRule{
				firewallRule{
					protocol: "UDP",
					destPort: "9162",
				}: []string{"snmp_traps"},
			},
			expectedBlockingRules: sourcesByRule{
				firewallRule{
					protocol: "UDP",
					destPort: "9162",
				}: []string{"snmp_traps"},
			},
		},
		{
			name: "blocked protocol but not port",
			output: []byte(`{
    "direction":  1,
    "protocol":  "UDP",
    "localPort":  "9160"
}`),
			rulesToCheck: sourcesByRule{
				firewallRule{
					protocol: "UDP",
					destPort: "9162",
				}: []string{"snmp_traps"},
			},
			expectedBlockingRules: make(sourcesByRule),
		},
		{
			name: "blocked port but not protocol",
			output: []byte(`{
    "direction":  1,
    "protocol":  "TCP",
    "localPort":  "9162"
}`),
			rulesToCheck: sourcesByRule{
				firewallRule{
					protocol: "UDP",
					destPort: "9162",
				}: []string{"snmp_traps"},
			},
			expectedBlockingRules: make(sourcesByRule),
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
			rulesToCheck: sourcesByRule{
				firewallRule{
					protocol: "UDP",
					destPort: "9162",
				}: []string{"snmp_traps", "netflow (netflow5)"},
				firewallRule{
					protocol: "UDP",
					destPort: "1111",
				}: []string{"netflow (netflow9)"},
				firewallRule{
					protocol: "UDP",
					destPort: "2000",
				}: []string{"netflow (ipfix)"},
			},
			expectedBlockingRules: sourcesByRule{
				firewallRule{
					protocol: "UDP",
					destPort: "9162",
				}: []string{"snmp_traps", "netflow (netflow5)"},
				firewallRule{
					protocol: "UDP",
					destPort: "2000",
				}: []string{"netflow (ipfix)"},
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
			rulesToCheck: sourcesByRule{
				firewallRule{
					protocol: "UDP",
					destPort: "9162",
				}: []string{"snmp_traps", "netflow (netflow5)"},
				firewallRule{
					protocol: "UDP",
					destPort: "1000",
				}: []string{"netflow (netflow9)"},
				firewallRule{
					protocol: "UDP",
					destPort: "2000",
				}: []string{"netflow (ipfix)"},
			},
			expectedBlockingRules: make(sourcesByRule),
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
