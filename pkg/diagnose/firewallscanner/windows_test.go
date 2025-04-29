// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package firewallscanner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_checkBlockedPortsWindows(t *testing.T) {
	tests := []struct {
		name                 string
		output               []byte
		forProtocol          string
		destPorts            integrationsByDestPort
		expectedBlockedPorts []blockedPort
		expectedError        error
	}{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blockedPorts, err := checkBlockedPortsWindows(tt.output, tt.forProtocol, tt.destPorts)
			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedBlockedPorts, blockedPorts)
			}
		})
	}
}
