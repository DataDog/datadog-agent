// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

//nolint:revive // TODO(PLINT) Fix revive linter
package wlan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQueryWifi(t *testing.T) {
	data, err := queryWiFiRSSI()
	if err != nil {
		t.Errorf("Error querying wifi RSSI: %s", err)
	}

	assert.Equal(t, data.rssi, 0)
	assert.Equal(t, data.ssid, "SSID")
	assert.Equal(t, data.bssid, "BSSID")
}
