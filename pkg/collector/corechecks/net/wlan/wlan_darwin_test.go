// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

//nolint:revive // TODO(PLINT) Fix revive linter
package wlan

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestQueryWifi(t *testing.T) {
	// setupLocationAccess()
	data, err := GetWiFiInfo()
	if err != nil {
		t.Errorf("Error querying wifi RSSI: %s", err)
	}

	// assert.NotZero(t, data.Rssi)

	// b, err := json.Marshal(data)
	// assert.Nil(t, err)
	// assert.Empty(t, string(b))
	assert.Empty(t, fmt.Sprintf("TEST: %+v", data))
	// assert.NotEmpty(t, data.Ssid)
	// assert.NotEmpty(t, data.Bssid)
}
