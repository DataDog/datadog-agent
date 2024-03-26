// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package payload

import (
	"testing"

	"github.com/stretchr/testify/require"

	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

func TestConvertOperStatus(t *testing.T) {
	tests := []struct {
		statusMap      map[string]devicemetadata.IfOperStatus
		status         string
		expectedStatus devicemetadata.IfOperStatus
	}{
		{
			status: "up",
			statusMap: map[string]devicemetadata.IfOperStatus{
				"up":   devicemetadata.OperStatusUp,
				"down": devicemetadata.OperStatusDown,
			},
			expectedStatus: devicemetadata.OperStatusUp,
		},
		{
			status: "down",
			statusMap: map[string]devicemetadata.IfOperStatus{
				"up":   devicemetadata.OperStatusUp,
				"down": devicemetadata.OperStatusDown,
			},
			expectedStatus: devicemetadata.OperStatusDown,
		},
		{
			status: "unknown",
			statusMap: map[string]devicemetadata.IfOperStatus{
				"up":   devicemetadata.OperStatusUp,
				"down": devicemetadata.OperStatusDown,
			},
			expectedStatus: devicemetadata.OperStatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			status := convertOperStatus(tt.statusMap, tt.status)
			require.Equal(t, tt.expectedStatus, status)
		})
	}
}

func TestConvertAdminStatus(t *testing.T) {
	tests := []struct {
		statusMap      map[string]devicemetadata.IfAdminStatus
		status         string
		expectedStatus devicemetadata.IfAdminStatus
	}{
		{
			status: "up",
			statusMap: map[string]devicemetadata.IfAdminStatus{
				"up":   devicemetadata.AdminStatusUp,
				"down": devicemetadata.AdminStatusDown,
			},
			expectedStatus: devicemetadata.AdminStatusUp,
		},
		{
			status: "down",
			statusMap: map[string]devicemetadata.IfAdminStatus{
				"up":   devicemetadata.AdminStatusUp,
				"down": devicemetadata.AdminStatusDown,
			},
			expectedStatus: devicemetadata.AdminStatusDown,
		},
		{
			status: "unknown",
			statusMap: map[string]devicemetadata.IfAdminStatus{
				"up":   devicemetadata.AdminStatusUp,
				"down": devicemetadata.AdminStatusDown,
			},
			expectedStatus: devicemetadata.AdminStatusDown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			status := convertAdminStatus(tt.statusMap, tt.status)
			require.Equal(t, tt.expectedStatus, status)
		})
	}
}
