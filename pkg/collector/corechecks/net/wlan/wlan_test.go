// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// //nolint:revive // TODO(PLINT) Fix revive linter
package wlan

import (
	"errors"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/stretchr/testify/assert"
)

func TestWLANOK(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "test-ssid",
			Bssid:           "test-bssid",
			Channel:         1,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
			ActivePHYMode:   6,
		}, nil
	}

	defer func() {
		getWiFiInfo = GetWiFiInfo
	}()

	expectedTags := []string{"ssid:test-ssid", "bssid:test-bssid", "mac_address:hardware-address"}

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()

	mockSender.AssertMetric(t, "Gauge", "wlan.rssi", 10.0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "wlan.noise", 20.0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "wlan.transmit_rate", 4.0, "", expectedTags)
	mockSender.AssertMetric(t, "Count", "wlan.channel_swap_events", 0.0, "", expectedTags)
}

func TestWLANGetInfoError(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{}, errors.New("some error message")
	}

	defer func() {
		getWiFiInfo = GetWiFiInfo
	}()

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()

	mockSender.AssertNumberOfCalls(t, "Gauge", 0)
	mockSender.AssertNumberOfCalls(t, "Count", 0)
}

func TestWLANErrorStoppedSender(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{}, nil
	}

	defer func() {
		getWiFiInfo = GetWiFiInfo
	}()

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")
	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)

	senderManager.Stop(false)
	err := wlanCheck.Run()

	assert.Equal(t, "demultiplexer is stopped", err.Error())

	mockSender.AssertNumberOfCalls(t, "Gauge", 0)
	mockSender.AssertNumberOfCalls(t, "Count", 0)
}

func TestWLANEmptySSIDisUnknown(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "",
			Bssid:           "test-bssid",
			Channel:         1,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
			ActivePHYMode:   6,
		}, nil
	}

	defer func() {
		getWiFiInfo = GetWiFiInfo
	}()

	expectedTags := []string{"ssid:unknown", "bssid:test-bssid", "mac_address:hardware-address"}

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()

	mockSender.AssertMetricTaggedWith(t, "Gauge", "wlan.rssi", expectedTags)
	mockSender.AssertMetricTaggedWith(t, "Gauge", "wlan.noise", expectedTags)
	mockSender.AssertMetricTaggedWith(t, "Gauge", "wlan.transmit_rate", expectedTags)
	mockSender.AssertMetricTaggedWith(t, "Count", "wlan.channel_swap_events", expectedTags)
}

func TestWLANEmptyBSSIDisUnknown(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "test-ssid",
			Bssid:           "",
			Channel:         1,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
			ActivePHYMode:   6,
		}, nil
	}

	defer func() {
		getWiFiInfo = GetWiFiInfo
	}()

	expectedTags := []string{"ssid:test-ssid", "bssid:unknown", "mac_address:hardware-address"}

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()

	mockSender.AssertMetricTaggedWith(t, "Gauge", "wlan.rssi", expectedTags)
	mockSender.AssertMetricTaggedWith(t, "Gauge", "wlan.noise", expectedTags)
	mockSender.AssertMetricTaggedWith(t, "Gauge", "wlan.transmit_rate", expectedTags)
	mockSender.AssertMetricTaggedWith(t, "Count", "wlan.channel_swap_events", expectedTags)
}

func TestWLANEmptyHardwareAddress(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "test-ssid",
			Bssid:           "test-bssid",
			Channel:         1,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "",
			ActivePHYMode:   6,
		}, nil
	}

	defer func() {
		getWiFiInfo = GetWiFiInfo
	}()

	expectedTags := []string{"ssid:test-ssid", "bssid:test-bssid", "mac_address:unknown"}

	wlanCheck := new(WLANCheck)

	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")
	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()

	mockSender.AssertMetric(t, "Gauge", "wlan.rssi", 10.0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "wlan.noise", 20.0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "wlan.transmit_rate", 4.0, "", expectedTags)
	mockSender.AssertMetric(t, "Count", "wlan.channel_swap_events", 0.0, "", expectedTags)
}

func TestWLANChannelSwapEvents(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "test-ssid",
			Bssid:           "test-bssid",
			Channel:         1,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
			ActivePHYMode:   6,
		}, nil
	}

	defer func() {
		getWiFiInfo = GetWiFiInfo
	}()

	expectedTags := []string{"ssid:test-ssid", "bssid:test-bssid", "mac_address:hardware-address"}

	wlanCheck := new(WLANCheck)

	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	// 1st run: initial channel number set to 1
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "wlan.channel_swap_events", 0.0, "", expectedTags)

	// change channel number from 1 to 2
	getWiFiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "test-ssid",
			Bssid:           "test-bssid",
			Channel:         2,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
			ActivePHYMode:   6,
		}, nil
	}

	// 2nd run: changing the channel number to 2
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "wlan.channel_swap_events", 1.0, "", expectedTags)

	getWiFiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "test-ssid",
			Bssid:           "test-bssid",
			Channel:         1,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
			ActivePHYMode:   6,
		}, nil
	}

	// 3rd run: changing the channel number back to 1
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "wlan.channel_swap_events", 1.0, "", expectedTags)

	// 4th run: keeping the same channel number
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "wlan.channel_swap_events", 0.0, "", expectedTags)
}

func TestWLANChannelSwapEventsChannelZero(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "test-ssid",
			Bssid:           "test-bssid",
			Channel:         0,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
			ActivePHYMode:   6,
		}, nil
	}

	defer func() {
		getWiFiInfo = GetWiFiInfo
	}()

	expectedTags := []string{"ssid:test-ssid", "bssid:test-bssid", "mac_address:hardware-address"}

	wlanCheck := new(WLANCheck)

	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	// 1st run: no channel change
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "wlan.channel_swap_events", 0.0, "", expectedTags)

	getWiFiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "test-ssid",
			Bssid:           "test-bssid",
			Channel:         0,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
			ActivePHYMode:   6,
		}, nil
	}

	// 2nd run: no channel change
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "wlan.channel_swap_events", 0.0, "", expectedTags)

	getWiFiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "test-ssid",
			Bssid:           "test-bssid",
			Channel:         1,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
			ActivePHYMode:   6,
		}, nil
	}

	// 3nd run: change channel to 1
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "wlan.channel_swap_events", 1.0, "", expectedTags)
}

func TestWLANRoamingEvents(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "ssid-1",
			Bssid:           "test-bssid",
			Channel:         1,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
			ActivePHYMode:   6,
		}, nil
	}

	defer func() {
		getWiFiInfo = GetWiFiInfo
	}()

	expectedTags := []string{"ssid:ssid-1", "bssid:test-bssid", "mac_address:hardware-address"}

	wlanCheck := new(WLANCheck)

	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	// 1st run: initial ssid set to ssid-1
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "wlan.roaming_events", 0.0, "", expectedTags)

	getWiFiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "ssid-2",
			Bssid:           "test-bssid",
			Channel:         2,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
			ActivePHYMode:   6,
		}, nil
	}

	expectedTags = []string{"ssid:ssid-2", "bssid:test-bssid", "mac_address:hardware-address"}

	// 2nd run: changing the ssid to ssid-2
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "wlan.roaming_events", 1.0, "", expectedTags)

	getWiFiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "ssid-1",
			Bssid:           "test-bssid",
			Channel:         1,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
			ActivePHYMode:   6,
		}, nil
	}

	expectedTags = []string{"ssid:ssid-1", "bssid:test-bssid", "mac_address:hardware-address"}

	// 3rd run: changing the ssid back to ssid-1
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "wlan.roaming_events", 1.0, "", expectedTags)

	// 4th run: keeping the same ssid
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "wlan.roaming_events", 0.0, "", expectedTags)
}

func TestWLANNoMetricsWhenWiFiInterfaceInactive(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (WiFiInfo, error) {
		return WiFiInfo{
			Rssi:            10,
			Ssid:            "ssid-1",
			Bssid:           "test-bssid",
			Channel:         1,
			Noise:           20,
			TransmitRate:    4.0,
			HardwareAddress: "hardware-address",
			ActivePHYMode:   0,
		}, nil
	}

	defer func() {
		getWiFiInfo = GetWiFiInfo
	}()

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()

	mockSender.AssertNumberOfCalls(t, "Gauge", 0)
	mockSender.AssertNumberOfCalls(t, "Count", 0)
}
