// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package snmpscanmanagerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsCacheable(t *testing.T) {
	tests := []struct {
		name               string
		deviceScan         deviceScan
		expectedIsCachable bool
	}{
		{
			name: "pending device scan is not cachable",
			deviceScan: deviceScan{
				ScanStatus: pendingStatus,
			},
			expectedIsCachable: false,
		},
		{
			name: "success device scan is cachable",
			deviceScan: deviceScan{
				ScanStatus: successStatus,
			},
			expectedIsCachable: true,
		},
		{
			name: "failed device scan is cachable",
			deviceScan: deviceScan{
				ScanStatus: failedStatus,
			},
			expectedIsCachable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedIsCachable, tt.deviceScan.isCacheable())
		})
	}
}
