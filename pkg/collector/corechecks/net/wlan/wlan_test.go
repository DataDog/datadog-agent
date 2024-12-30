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
)

func TestWLANOK(t *testing.T) {
	// setup mocks
	getWifiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "test-ssid",
			Bssid:           "test-bssid",
			Channel:         1,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
		}, nil
	}
	setupLocationAccess = func() {
	}
	defer func() {
		getWifiInfo = GetWiFiInfo
		setupLocationAccess = SetupLocationAccess
	}()

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()

	expectedTags := []string{"ssid:test-ssid", "bssid:test-bssid", "mac_address:hardware-address"}

	mockSender.AssertMetric(t, "Gauge", "wlan.rssi", 10.0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "wlan.noise", 20.0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "wlan.transmit_rate", 4.0, "", expectedTags)
	mockSender.AssertMetric(t, "Count", "wlan.channel_swap_events", 0.0, "", expectedTags)
}

func TestWLANEmptySSIDandBSSID(t *testing.T) {
	// setup mocks
	getWifiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "",
			Bssid:           "",
			Channel:         1,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
		}, nil
	}
	setupLocationAccess = func() {
	}
	defer func() {
		getWifiInfo = GetWiFiInfo
		setupLocationAccess = SetupLocationAccess
	}()

	wlanCheck := new(WLANCheck)

	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")
	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()

	expectedTags := []string{"ssid:unknown", "bssid:unknown", "mac_address:hardware-address"}

	mockSender.AssertMetric(t, "Gauge", "wlan.rssi", 10.0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "wlan.noise", 20.0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "wlan.transmit_rate", 4.0, "", expectedTags)
	mockSender.AssertMetric(t, "Count", "wlan.channel_swap_events", 0.0, "", expectedTags)
}

func TestWLANChannelSwapEvents(t *testing.T) {
	// setup mocks
	getWifiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "",
			Bssid:           "",
			Channel:         1,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
		}, nil
	}
	setupLocationAccess = func() {
	}
	defer func() {
		getWifiInfo = GetWiFiInfo
		setupLocationAccess = SetupLocationAccess
	}()

	expectedTags := []string{"ssid:unknown", "bssid:unknown", "mac_address:hardware-address"}

	wlanCheck := new(WLANCheck)

	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	// 1st run: initial channel number set to 1
	wlanCheck.Run()

	mockSender.AssertMetric(t, "Count", "wlan.channel_swap_events", 0.0, "", expectedTags)

	// change channel number from 1 to 2
	getWifiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "",
			Bssid:           "",
			Channel:         2,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
		}, nil
	}

	// 2nd run: changing the channel number to 2
	wlanCheck.Run()

	mockSender.AssertMetric(t, "Count", "wlan.channel_swap_events", 1.0, "", expectedTags)

	getWifiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "",
			Bssid:           "",
			Channel:         1,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
		}, nil
	}

	// 3rd run: changing the channel number back to 1
	wlanCheck.Run()

	mockSender.AssertMetric(t, "Count", "wlan.channel_swap_events", 1.0, "", expectedTags)

	// 4th run: keeping the same channel number
	wlanCheck.Run()

	mockSender.AssertMetric(t, "Count", "wlan.channel_swap_events", 0.0, "", expectedTags)
}
