// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// //nolint:revive // TODO(PLINT) Fix revive linter
package wlan

import (
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/stretchr/testify/mock"
)

var testGetWifiInfo = func() (WiFiInfo, error) {
	return WiFiInfo{
		Rssi:         10,
		Ssid:         "test-ssid",
		Bssid:        "test-bssid",
		Channel:      1,
		Noise:        20,
		TransmitRate: 4.0,
		SecurityType: "test-security-type",
	}, nil
}

var testSetupLocationAccess = func() {
}

func TestWLANOK(t *testing.T) {
	// setup mocks
	getWifiInfo = testGetWifiInfo
	setupLocationAccess = testSetupLocationAccess
	defer func() {
		getWifiInfo = GetWiFiInfo
		setupLocationAccess = SetupLocationAccess
	}()

	wlanCheck := new(WLANCheck)

	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)

	mockSender.On("Gauge", "wlan.rssi", 10.0, mock.Anything, []string{"ssid:test-ssid", "bssid:test-bssid"}).Return().Times(1)
	mockSender.On("Gauge", "wlan.noise", 20.0, mock.Anything, []string{"ssid:test-ssid", "bssid:test-bssid"}).Return().Times(1)
	mockSender.On("Gauge", "wlan.transmit_rate", 4.0, mock.Anything, []string{"ssid:test-ssid", "bssid:test-bssid"}).Return().Times(1)

	mockSender.On("Commit").Return().Times(1)
	wlanCheck.Run()

	mockSender.AssertExpectations(t)
	mockSender.AssertNumberOfCalls(t, "Gauge", 3)
	mockSender.AssertNumberOfCalls(t, "Commit", 1)
}
