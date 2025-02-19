// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018-present Datadog, Inc.

//go:build windows

package winutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestGetSidFromUser(t *testing.T) {
	sid, err := GetSidFromUser()
	t.Logf("The SID found was: %v", sid)
	assert.Nil(t, err)
	assert.NotNil(t, sid)
}

func TestGetServiceUserSID(t *testing.T) {
	// create LocalService SID
	serviceSid, err := windows.StringToSid("S-1-5-19")
	require.NoError(t, err)

	// get the SID for the EventLog service (has LocalService as its user)
	sid, err := GetServiceUserSID("EventLog")
	require.NoError(t, err)
	assert.NotNil(t, sid)
	assert.True(t, windows.EqualSid(sid, serviceSid))
	t.Logf("The SID found was: %v", sid)

	// create LocalSystem SID
	systemSid, err := windows.StringToSid("S-1-5-18")
	require.NoError(t, err)

	// get the SID for the BITS service (has LocalSystem as its user)
	sid, err = GetServiceUserSID("BITS")
	require.NoError(t, err)
	assert.NotNil(t, sid)
	assert.True(t, windows.EqualSid(sid, systemSid))
	t.Logf("The SID found was: %v", sid)
}
