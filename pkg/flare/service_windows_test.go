// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package flare

import (
	"testing"

	"golang.org/x/sys/windows"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

func TestWindowsService(t *testing.T) {
	manager, err := winutil.OpenSCManager(scManagerAccess)
	if !assert.NoError(t, err) {
		assert.FailNow(t, "Error connecting to SC Manager: %v", err)
	}
	defer manager.Disconnect()

	evtlog, err := winutil.OpenService(manager, "EventLog", windows.GENERIC_READ)
	if !assert.NoError(t, err) {
		assert.FailNow(t, "Error opening service EventLog: %v", err)
	}
	evtlogConf, err := getServiceInfo(evtlog)
	if !assert.NoError(t, err) {
		assert.FailNow(t, "Error getting Service Info: %v", err)
	}

	assert.Contains(t, evtlogConf.ServiceName, "EventLog", "Expected service name EventLog")

	assert.Contains(t, evtlogConf.Config.ServiceType, "Win32ShareProcess", "Expected EventLog to have service type Win32ShareProcess")

	var zero uint32
	assert.Equal(t, *evtlogConf.TriggersCount, zero, "Expected EventLog to have trigger count 0")

	assert.NotNil(t, evtlogConf.ServiceFailureActions.RecoveryActions, "Expected EventLog to have Recovery Actions")
	assert.NotContains(t, evtlogConf.ServiceState, "Unknown", "Expected EventLog to have a valid State")

	if evtlogConf.ServiceState == "Running" {
		assert.NotEqual(t, evtlogConf.ProcessID, zero, "Expected Running EventLog to have a non-zero ProcessID")
	}

}
