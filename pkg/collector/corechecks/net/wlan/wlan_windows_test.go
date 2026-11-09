// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && test

package wlan

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetWiFiInfoWithoutWLANAPI covers a host with no WLAN stack, the default
// state of Windows Server: wlanapi.dll is absent and calling into it panics.
func TestGetWiFiInfoWithoutWLANAPI(t *testing.T) {
	getWiFiInfo = nil // exercise the real implementation
	orig := loadWlanAPI
	loadWlanAPI = func() error { return errors.New("wlanapi.dll is missing") }
	t.Cleanup(func() { loadWlanAPI = orig })

	c := &WLANCheck{}
	var (
		wi  wifiInfo
		err error
	)
	require.NotPanics(t, func() { wi, err = c.GetWiFiInfo() })
	assert.NoError(t, err)
	assert.Equal(t, "None", wi.phyMode)
}
