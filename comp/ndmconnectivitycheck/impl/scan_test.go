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

func TestScanTriesEachGroupOnlyWithItsCredentials(t *testing.T) {
	creds := []connectivity.SNMPCredential{{ID: "cred-a", Version: "2c"}, {ID: "cred-b", Version: "2c"}}
	assert.Equal(t, creds[1:], credsByID(creds, []string{"cred-b", "cred-z"}))

	c := newImpl(logmock.New(t))
	res, err := c.scan(context.Background(), connectivity.Request{
		Checks:      []string{connectivity.CheckSNMP},
		SNMPOptions: &connectivity.SNMPOptions{Port: 1, TimeoutMs: 50},
		Credentials: creds,
		TargetGroups: []connectivity.TargetGroup{
			{CredentialIDs: []string{"cred-a"}, TargetIPs: []string{"127.0.0.1"}},
			{CredentialIDs: []string{"cred-z"}, TargetIPs: []string{"10.0.0.2"}},
		},
	})

	require.NoError(t, err)
	require.Len(t, res.Devices, 2)
	assert.Equal(t, "127.0.0.1", res.Devices[0].IPAddress)
	assert.NotNil(t, res.Devices[0].SNMPResult, "tried with cred-a")
	assert.Equal(t, "10.0.0.2", res.Devices[1].IPAddress)
	assert.Nil(t, res.Devices[1].SNMPResult, "its group names no known credential")
}
