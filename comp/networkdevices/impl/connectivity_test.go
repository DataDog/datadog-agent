// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package networkdevicesimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

func TestCheckConnectivity(t *testing.T) {
	cases := []struct {
		name        string
		req         connectivity.Request
		expectedErr string
	}{
		{
			name: "fails when ping options are missing",
			req: connectivity.Request{
				Targets: []string{"10.0.0.1"},
				Checks:  []string{connectivity.CheckPing},
			},
			expectedErr: "options are required for ping",
		},
		{
			name: "fails when snmp options are missing",
			req: connectivity.Request{
				Targets: []string{"10.0.0.1"},
				Checks:  []string{connectivity.CheckSNMP},
			},
			expectedErr: "options are required for SNMP",
		},
		{
			name: "fails when the snmp port is out of range",
			req: connectivity.Request{
				Targets:     []string{"10.0.0.1"},
				Checks:      []string{connectivity.CheckSNMP},
				SNMPOptions: &connectivity.SNMPOptions{Port: 65536},
			},
			expectedErr: "SNMP port 65536 out of range",
		},
		{
			name: "fails when a check is not supported",
			req: connectivity.Request{
				Targets: []string{"10.0.0.1"},
				Checks:  []string{"telnet"},
			},
			expectedErr: "unsupported check: 'telnet'",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &networkDevicesImpl{}

			res, err := c.CheckConnectivity(context.Background(), tc.req)
			require.Equal(t, connectivity.Result{}, res)
			require.ErrorContains(t, err, tc.expectedErr)
		})
	}
}
