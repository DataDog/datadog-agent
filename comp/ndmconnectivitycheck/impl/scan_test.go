// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmconnectivitycheckimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

func TestScanReturnsAGroupedRunInGroupOrder(t *testing.T) {
	c := newImpl(logmock.New(t))

	res, err := c.scan(context.Background(), connectivity.Request{
		Checks:      []string{},
		Credentials: []connectivity.SNMPCredential{{ID: "cred-a", Version: "2c"}, {ID: "cred-b", Version: "2c"}},
		TargetGroups: []connectivity.TargetGroup{
			{CredentialIDs: []string{"cred-b"}, TargetIPs: []string{"10.0.0.3", "10.0.0.1"}},
			{CredentialIDs: []string{"cred-a"}, TargetIPs: []string{"10.0.0.2"}},
		},
	})

	require.NoError(t, err)
	require.Len(t, res.Devices, 3)
	assert.Equal(t, "10.0.0.3", res.Devices[0].IPAddress)
	assert.Equal(t, "10.0.0.1", res.Devices[1].IPAddress)
	assert.Equal(t, "10.0.0.2", res.Devices[2].IPAddress)
}

func TestScanWithoutGroupsTriesTheTargetsInOrder(t *testing.T) {
	c := newImpl(logmock.New(t))

	res, err := c.scan(context.Background(), connectivity.Request{
		Targets: []string{"10.0.0.2", "10.0.0.1"},
		Checks:  []string{},
	})

	require.NoError(t, err)
	require.Len(t, res.Devices, 2)
	assert.Equal(t, "10.0.0.2", res.Devices[0].IPAddress)
	assert.Equal(t, "10.0.0.1", res.Devices[1].IPAddress)
}

func TestScanRequiresExactlyOneOfTargetsAndTargetGroups(t *testing.T) {
	group := []connectivity.TargetGroup{{CredentialIDs: []string{"cred-a"}, TargetIPs: []string{"10.0.0.1"}}}
	tests := map[string]connectivity.Request{
		"both set": {
			Targets:      []string{"10.0.0.1"},
			Credentials:  []connectivity.SNMPCredential{{ID: "cred-a", Version: "2c"}},
			TargetGroups: group,
		},
		"neither set": {Checks: []string{}},
	}

	for name, req := range tests {
		t.Run(name, func(t *testing.T) {
			c := newImpl(logmock.New(t))

			_, err := c.scan(context.Background(), req)

			assert.ErrorIs(t, err, connectivity.ErrInvalidRequest)
		})
	}
}

func TestScanRejectsAnInvalidGroupedRequest(t *testing.T) {
	c := newImpl(logmock.New(t))

	_, err := c.scan(context.Background(), connectivity.Request{
		Checks:       []string{},
		TargetGroups: []connectivity.TargetGroup{{CredentialIDs: []string{"cred-z"}, TargetIPs: []string{"10.0.0.1"}}},
	})

	assert.ErrorIs(t, err, connectivity.ErrInvalidRequest)
}
