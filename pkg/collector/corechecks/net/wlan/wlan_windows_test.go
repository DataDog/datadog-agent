// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && test

package wlan

import (
	"testing"

	"golang.org/x/sys/windows"

	"github.com/stretchr/testify/assert"
)

func setMissingWLANAPIDLLForTest(t *testing.T) {
	t.Helper()

	origWlanAPI := wlanAPI
	origWlanOpenHandle := wlanOpenHandle
	origWlanCloseHandle := wlanCloseHandle
	origWlanEnumInterfaces := wlanEnumInterfaces
	origWlanQueryInterface := wlanQueryInterface
	origWlanFreeMemory := wlanFreeMemory

	t.Cleanup(func() {
		wlanAPI = origWlanAPI
		wlanOpenHandle = origWlanOpenHandle
		wlanCloseHandle = origWlanCloseHandle
		wlanEnumInterfaces = origWlanEnumInterfaces
		wlanQueryInterface = origWlanQueryInterface
		wlanFreeMemory = origWlanFreeMemory
	})

	badDLL := windows.NewLazySystemDLL("ddagent_missing_wlanapi.dll")
	wlanAPI = badDLL
	wlanOpenHandle = badDLL.NewProc("WlanOpenHandle")
	wlanCloseHandle = badDLL.NewProc("WlanCloseHandle")
	wlanEnumInterfaces = badDLL.NewProc("WlanEnumInterfaces")
	wlanQueryInterface = badDLL.NewProc("WlanQueryInterface")
	wlanFreeMemory = badDLL.NewProc("WlanFreeMemory")
}

func TestGetWlanHandleMissingDLLReturnsErrorNotPanic(t *testing.T) {
	setMissingWLANAPIDLLForTest(t)

	assert.NotPanics(t, func() {
		_, err := getWlanHandle()
		assert.Error(t, err)
		assert.ErrorContains(t, err, "WLAN API unavailable")
	})
}

func TestGetFirstConnectedWlanInfoMissingDLLReturnsErrorNotPanic(t *testing.T) {
	setMissingWLANAPIDLLForTest(t)

	assert.NotPanics(t, func() {
		_, err := getFirstConnectedWlanInfo()
		assert.Error(t, err)
		assert.ErrorContains(t, err, "failed to get WLAN client handle")
	})
}

func TestGetWiFiInfoMissingDLLReturnsErrorNotPanic(t *testing.T) {
	setMissingWLANAPIDLLForTest(t)

	assert.NotPanics(t, func() {
		_, err := GetWiFiInfo()
		assert.Error(t, err)
		assert.ErrorContains(t, err, "failed to get WLAN client handle")
	})
}
