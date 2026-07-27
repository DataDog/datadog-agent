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
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "test-ssid",
			bssid:        "test-bssid",
			channel:      1,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	defer func() {
		getWiFiInfo = nil
	}()

	expectedTags := []string{"ssid:test-ssid", "bssid:test-bssid", "mac_address:hardware-address", "status:ok"}

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()

	// Should emit status metric (OK)
	mockSender.AssertMetric(t, "Gauge", "system.wlan.status", 0.0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "system.wlan.rssi", 10.0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "system.wlan.noise", 20.0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "system.wlan.txrate", 4.0, "", expectedTags)
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)
}

func TestWLANGetInfoError(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{}, errors.New("some error message")
	}

	defer func() {
		getWiFiInfo = nil
	}()

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()

	// Should emit status metric (CRITICAL) and error count metric
	mockSender.AssertNumberOfCalls(t, "Gauge", 1)
	mockSender.AssertNumberOfCalls(t, "Count", 1)
	mockSender.AssertMetric(t, "Gauge", "system.wlan.status", 2.0, "", []string{"status:critical", "reason:ipc_failure"})
	mockSender.AssertMetric(t, "Count", "system.wlan.check.errors", 1.0, "", []string{"error_type:ipc_failure"})
}

func TestWLANErrorStoppedSender(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{}, nil
	}

	defer func() {
		getWiFiInfo = nil
	}()

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.NewStoppedSenderManager()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")
	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)

	err := wlanCheck.Run()

	assert.ErrorIs(t, err, mocksender.ErrStoppedSenderManager)

	mockSender.AssertNumberOfCalls(t, "Gauge", 0)
	mockSender.AssertNumberOfCalls(t, "Count", 0)
}

func TestWLANEmptySSIDisUnknown(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "",
			bssid:        "test-bssid",
			channel:      1,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	defer func() {
		getWiFiInfo = nil
	}()

	expectedTags := []string{"ssid:unknown", "bssid:test-bssid", "mac_address:hardware-address", "status:ok"}

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()

	mockSender.AssertMetricTaggedWith(t, "Gauge", "system.wlan.rssi", expectedTags)
	mockSender.AssertMetricTaggedWith(t, "Gauge", "system.wlan.noise", expectedTags)
	mockSender.AssertMetricTaggedWith(t, "Gauge", "system.wlan.txrate", expectedTags)
	mockSender.AssertMetricTaggedWith(t, "Count", "system.wlan.channel_swap_events", expectedTags)
}

func TestWLANEmptyBSSIDisUnknown(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "test-ssid",
			bssid:        "",
			channel:      1,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	defer func() {
		getWiFiInfo = nil
	}()

	expectedTags := []string{"ssid:test-ssid", "bssid:unknown", "mac_address:hardware-address", "status:ok"}

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()

	mockSender.AssertMetricTaggedWith(t, "Gauge", "system.wlan.rssi", expectedTags)
	mockSender.AssertMetricTaggedWith(t, "Gauge", "system.wlan.noise", expectedTags)
	mockSender.AssertMetricTaggedWith(t, "Gauge", "system.wlan.txrate", expectedTags)
	mockSender.AssertMetricTaggedWith(t, "Count", "system.wlan.channel_swap_events", expectedTags)
}

func TestWLANEmptyHardwareAddress(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "test-ssid",
			bssid:        "test-bssid",
			channel:      1,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "",
			phyMode:      "802.11ac",
		}, nil
	}

	defer func() {
		getWiFiInfo = nil
	}()

	expectedTags := []string{"ssid:test-ssid", "bssid:test-bssid", "mac_address:unknown", "status:ok"}

	wlanCheck := new(WLANCheck)

	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")
	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()

	mockSender.AssertMetric(t, "Gauge", "system.wlan.rssi", 10.0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "system.wlan.noise", 20.0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "system.wlan.txrate", 4.0, "", expectedTags)
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)
}

func TestWLANChannelSwapEventsBasic(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "test-ssid",
			bssid:        "test-bssid",
			channel:      1,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	defer func() {
		getWiFiInfo = nil
	}()

	expectedTags := []string{"ssid:test-ssid", "bssid:test-bssid", "mac_address:hardware-address", "status:ok"}

	wlanCheck := new(WLANCheck)

	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	// 1st run: initial channel number set to 1
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)

	// change channel number from 1 to 2
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "test-ssid",
			bssid:        "test-bssid",
			channel:      2,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	// 2nd run: changing the channel number to 2
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 1.0, "", expectedTags)

	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "test-ssid",
			bssid:        "test-bssid",
			channel:      1,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	// 3rd run: changing the channel number back to 1
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 1.0, "", expectedTags)

	// 4th run: keeping the same channel number
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)
}

func TestWLANChannelSwapEventsFromZeroToZeroAndOne(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "test-ssid",
			bssid:        "test-bssid",
			channel:      0,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	defer func() {
		getWiFiInfo = nil
	}()

	expectedTags := []string{"ssid:test-ssid", "bssid:test-bssid", "mac_address:hardware-address", "status:ok"}

	wlanCheck := new(WLANCheck)

	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	// 1st run: no channel change
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)

	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "test-ssid",
			bssid:        "test-bssid",
			channel:      0,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	// 2nd run: no channel change
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)

	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "test-ssid",
			bssid:        "test-bssid",
			channel:      1,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	// 3nd run: change channel to 1
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 1.0, "", expectedTags)
}

func TestWLANChannelSwapEventsWhenSSIDEmptyAndBSSIDIsTheSame(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "",
			bssid:        "test-bssid",
			channel:      1,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	defer func() {
		getWiFiInfo = nil
	}()

	expectedTags := []string{"ssid:unknown", "bssid:test-bssid", "mac_address:hardware-address", "status:ok"}

	wlanCheck := new(WLANCheck)

	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	// 1st run: no channel change
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)

	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "",
			bssid:        "test-bssid",
			channel:      1,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	// 2nd run: no channel change
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)

	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "",
			bssid:        "test-bssid",
			channel:      2,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	// 3nd run: change channel to 1
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 1.0, "", expectedTags)
}

func TestWLANChannelSwapEventsUnlessThereIsRoaming(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "test-ssid",
			bssid:        "test-bssid-1",
			channel:      1,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	defer func() {
		getWiFiInfo = nil
	}()

	expectedTags := []string{"ssid:test-ssid", "bssid:test-bssid-1", "mac_address:hardware-address", "status:ok"}

	wlanCheck := new(WLANCheck)

	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	// 1st run: no channel change
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)

	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "test-ssid",
			bssid:        "test-bssid-1",
			channel:      2,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	// 2nd run: channel change (BSSID is the same)
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 1.0, "", expectedTags)

	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "test-ssid",
			bssid:        "test-bssid-2",
			channel:      3,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	// 3nd run: channel event is not triggered because it is roaming
	wlanCheck.Run()
	expectedTags = []string{"ssid:test-ssid", "bssid:test-bssid-2", "mac_address:hardware-address"}
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)
	mockSender.AssertMetric(t, "Count", "system.wlan.roaming_events", 1.0, "", expectedTags)
}

func TestWLANRoamingEvents(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "ssid",
			bssid:        "test-bssid-1",
			channel:      1,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	defer func() {
		getWiFiInfo = nil
	}()

	expectedTags := []string{"ssid:ssid", "bssid:test-bssid-1", "mac_address:hardware-address", "status:ok"}

	wlanCheck := new(WLANCheck)

	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	// 1st run: initial bssid set to test-bssid-1
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.roaming_events", 0.0, "", expectedTags)

	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "ssid",
			bssid:        "test-bssid-2",
			channel:      2,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	expectedTags = []string{"ssid:ssid", "bssid:test-bssid-2", "mac_address:hardware-address"}

	// 2nd run: changing the test-bssid-1 to test-bssid-2
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.roaming_events", 1.0, "", expectedTags)
	// despite channel change, and due to roaming event the channel swap event is not changed
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)

	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "ssid",
			bssid:        "test-bssid-1",
			channel:      1,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	expectedTags = []string{"ssid:ssid", "bssid:test-bssid-1", "mac_address:hardware-address"}

	// 3rd run: changing the test-bssid-2 back to test-bssid-1
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.roaming_events", 1.0, "", expectedTags)
	// despite channel change, and due to roaming event the channel swap event is not changed
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)

	// 4th run: keeping the same ssid
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.roaming_events", 0.0, "", expectedTags)
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)
}

func TestWLANNoRoamingOrChannelSwapEventsWhenDifferentNetwork(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "ssid",
			bssid:        "test-bssid",
			channel:      1,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	defer func() {
		getWiFiInfo = nil
	}()

	expectedTags := []string{"ssid:ssid", "bssid:test-bssid", "mac_address:hardware-address", "status:ok"}

	wlanCheck := new(WLANCheck)

	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	// 1st run: initial no detection is performed
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.roaming_events", 0.0, "", expectedTags)
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)

	// change channel, bssid and ssid
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "ssid-2",
			bssid:        "test-bssid-2",
			channel:      2,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	expectedTags = []string{"ssid:ssid-2", "bssid:test-bssid-2", "mac_address:hardware-address"}

	// 2nd run: since ssid changed roaming or channel swap will not be detected
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.roaming_events", 0.0, "", expectedTags)
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)

	// ssid is empty
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "",
			bssid:        "test-bssid-3",
			channel:      3,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	expectedTags = []string{"ssid:unknown", "bssid:test-bssid-3", "mac_address:hardware-address"}

	// 3rd run: ssid is empty and different from premise scan, meaning it is a different network and no
	// 1.0 metrics for roaming or channel swap would be emitted
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.roaming_events", 0.0, "", expectedTags)
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)

	// now both ssid asre empty but bssid is also different
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "",
			bssid:        "test-bssid-4",
			channel:      4,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	expectedTags = []string{"ssid:unknown", "bssid:test-bssid-4", "mac_address:hardware-address"}

	// 4rd run: both ssids are empty and different and bssid is different
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.roaming_events", 0.0, "", expectedTags)
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)

	// now both ssid asre empty but bssid is also different
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "ssid",
			bssid:        "test-bssid-5",
			channel:      5,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	expectedTags = []string{"ssid:ssid", "bssid:test-bssid-5", "mac_address:hardware-address"}

	// 5rd run: now one ssids is not empty and other is so dsifferent network
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.roaming_events", 0.0, "", expectedTags)
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)

	// one bssid is empty
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "ssid",
			bssid:        "",
			channel:      6,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	expectedTags = []string{"ssid:ssid", "bssid:unknown", "mac_address:hardware-address"}

	// 6rd run: if a bssid empty we cannot determine roaming or channel swap
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.roaming_events", 0.0, "", expectedTags)
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)

	// now both bssid are empty
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "ssid",
			bssid:        "",
			channel:      7,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	expectedTags = []string{"ssid:ssid", "bssid:unknown", "mac_address:hardware-address"}

	// 7rd run: if a bssid empty we cannot determine roaming or channel swap
	wlanCheck.Run()
	mockSender.AssertMetric(t, "Count", "system.wlan.roaming_events", 0.0, "", expectedTags)
	mockSender.AssertMetric(t, "Count", "system.wlan.channel_swap_events", 0.0, "", expectedTags)
}

func TestWLANNoMetricsWhenWiFiInterfaceInactive(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "ssid-1",
			bssid:        "test-bssid",
			channel:      1,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "None",
		}, nil
	}

	defer func() {
		getWiFiInfo = nil
	}()

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()

	// Should emit status metric (WARNING) when WiFi interface is inactive
	mockSender.AssertNumberOfCalls(t, "Gauge", 1)
	mockSender.AssertNumberOfCalls(t, "Count", 0)
	mockSender.AssertMetric(t, "Gauge", "system.wlan.status", 1.0, "", []string{"status:warning", "reason:interface_inactive"})
}

func TestWLANNoiseValidDisabled(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "test-ssid",
			bssid:        "test-bssid",
			channel:      1,
			noise:        20,
			noiseValid:   false,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}

	defer func() {
		getWiFiInfo = nil
	}()

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()
	mockSender.AssertMetricMissing(t, "Gauge", "system.wlan.noise")
}

func TestWLANNoiseValidEnabled(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:             10,
			ssid:             "test-ssid",
			bssid:            "test-bssid",
			channel:          1,
			noise:            20,
			noiseValid:       true,
			transmitRate:     4.0,
			receiveRate:      5.0,
			receiveRateValid: false,
			macAddress:       "hardware-address",
			phyMode:          "802.11ac",
		}, nil
	}

	defer func() {
		getWiFiInfo = nil
	}()

	expectedTags := []string{"ssid:test-ssid", "bssid:test-bssid", "mac_address:hardware-address", "status:ok"}

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()
	mockSender.AssertMetric(t, "Gauge", "system.wlan.noise", 20.0, "", expectedTags)
}

func TestWLANReceiveRateValidDisabled(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:             10,
			ssid:             "test-ssid",
			bssid:            "test-bssid",
			channel:          1,
			noise:            20,
			noiseValid:       true,
			transmitRate:     4.0,
			receiveRate:      5.0,
			receiveRateValid: false,
			macAddress:       "hardware-address",
			phyMode:          "802.11ac",
		}, nil
	}

	defer func() {
		getWiFiInfo = nil
	}()

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()
	mockSender.AssertMetricMissing(t, "Gauge", "system.wlan.rxrate")
}

func TestWLANReceiveRateValid(t *testing.T) {
	// setup mocks
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:             10,
			ssid:             "test-ssid",
			bssid:            "test-bssid",
			channel:          1,
			noise:            20,
			noiseValid:       true,
			transmitRate:     4.0,
			receiveRate:      5.0,
			receiveRateValid: true,
			macAddress:       "hardware-address",
			phyMode:          "802.11ac",
		}, nil
	}

	defer func() {
		getWiFiInfo = nil
	}()

	expectedTags := []string{"ssid:test-ssid", "bssid:test-bssid", "mac_address:hardware-address", "status:ok"}

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()
	mockSender.AssertMetric(t, "Gauge", "system.wlan.rxrate", 5.0, "", expectedTags)
}

func TestWLANAPScanDefaultEnabled(t *testing.T) {
	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	// init_config omitted -> ap_scan defaults to enabled.
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	assert.True(t, wlanCheck.apScanEnabled)
}

func TestWLANAPScanExplicitTrue(t *testing.T) {
	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, integration.Data("ap_scan: true"), "test", "provider")

	assert.True(t, wlanCheck.apScanEnabled)
}

func TestWLANAPScanDisabled(t *testing.T) {
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:       10,
			ssid:       "test-ssid",
			bssid:      "test-bssid",
			macAddress: "hardware-address",
			phyMode:    "802.11ac",
		}, nil
	}
	getNearbyAPs = func() ([]accessPointInfo, error) {
		t.Fatal("GetNearbyAccessPoints should not be called when ap_scan is false")
		return nil, nil
	}
	defer func() {
		getWiFiInfo = nil
		getNearbyAPs = nil
	}()

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, integration.Data("ap_scan: false"), "test", "provider")

	assert.False(t, wlanCheck.apScanEnabled)

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()

	mockSender.AssertMetricMissing(t, "Gauge", "system.wlan.scan.rssi")
}

func TestWLANScanRSSIEmission(t *testing.T) {
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "test-ssid",
			bssid:        "AA:BB:CC:DD:EE:01",
			channel:      1,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}
	getNearbyAPs = func() ([]accessPointInfo, error) {
		return []accessPointInfo{
			{rssi: 10, ssid: "test-ssid", bssid: "AA:BB:CC:DD:EE:01"},   // the connected AP
			{rssi: -60, ssid: "neighbor-1", bssid: "AA:BB:CC:DD:EE:02"}, // nearby
			{rssi: -70, ssid: "", bssid: ""},                            // nearby, missing ssid/bssid
		}, nil
	}
	defer func() {
		getWiFiInfo = nil
		getNearbyAPs = nil
	}()

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()

	// The connected AP is tagged connected:1; nearby APs connected:0. Empty
	// ssid/bssid fall back to "unknown" like the connected-AP path.
	mockSender.AssertMetric(t, "Gauge", "system.wlan.scan.rssi", 10.0, "", []string{"ssid:test-ssid", "bssid:AA:BB:CC:DD:EE:01", "connected:1"})
	mockSender.AssertMetric(t, "Gauge", "system.wlan.scan.rssi", -60.0, "", []string{"ssid:neighbor-1", "bssid:AA:BB:CC:DD:EE:02", "connected:0"})
	mockSender.AssertMetric(t, "Gauge", "system.wlan.scan.rssi", -70.0, "", []string{"ssid:unknown", "bssid:unknown", "connected:0"})
}

func TestWLANScanErrorDoesNotFailCheck(t *testing.T) {
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi:         10,
			ssid:         "test-ssid",
			bssid:        "test-bssid",
			channel:      1,
			noise:        20,
			noiseValid:   true,
			transmitRate: 4.0,
			macAddress:   "hardware-address",
			phyMode:      "802.11ac",
		}, nil
	}
	getNearbyAPs = func() ([]accessPointInfo, error) {
		return nil, errors.New("scan failed")
	}
	defer func() {
		getWiFiInfo = nil
		getNearbyAPs = nil
	}()

	expectedTags := []string{"ssid:test-ssid", "bssid:test-bssid", "mac_address:hardware-address", "status:ok"}

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	// A scan failure is non-fatal: connected-AP metrics still emit and Run succeeds.
	err := wlanCheck.Run()
	assert.NoError(t, err)
	mockSender.AssertMetric(t, "Gauge", "system.wlan.rssi", 10.0, "", expectedTags)
	mockSender.AssertMetricMissing(t, "Gauge", "system.wlan.scan.rssi")
}

func TestAPFilterAllowed(t *testing.T) {
	tests := []struct {
		name                           string
		ssidInc, ssidExc, bsInc, bsExc []string
		ssid, bssid                    string
		want                           bool
	}{
		{name: "default allow all", ssid: "Any", bssid: "aa:bb", want: true},
		{name: "ssid include match", ssidInc: []string{"Corp-*"}, ssid: "Corp-5G", bssid: "x", want: true},
		{name: "ssid include no match", ssidInc: []string{"Corp-*"}, ssid: "Home", bssid: "x", want: false},
		{name: "ssid include case-insensitive", ssidInc: []string{"corp-*"}, ssid: "CORP-5G", bssid: "x", want: true},
		{name: "ssid exclude wins", ssidInc: []string{"*"}, ssidExc: []string{"Home"}, ssid: "Home", bssid: "x", want: false},
		{name: "exclude only blocks match", ssidExc: []string{"Home"}, ssid: "Home", bssid: "x", want: false},
		{name: "exclude only allows non-match", ssidExc: []string{"Home"}, ssid: "Work", bssid: "x", want: true},
		{name: "bssid include match", bsInc: []string{"00:1a:2b:*"}, ssid: "x", bssid: "00:1A:2B:CC:DD:EE", want: true},
		{name: "include set but empty inputs", ssidInc: []string{"Corp-*"}, ssid: "", bssid: "", want: false},
		{name: "ssid or bssid include (bssid matches)", ssidInc: []string{"Corp-*"}, bsInc: []string{"00:*"}, ssid: "Home", bssid: "00:11:22", want: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newAPFilter(tc.ssidInc, tc.ssidExc, tc.bsInc, tc.bsExc)
			assert.Equal(t, tc.want, f.allowed(tc.ssid, tc.bssid))
		})
	}
}

func TestWLANMetricFilterExcludesConnected(t *testing.T) {
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi: 10, ssid: "HomeNet", bssid: "AA:BB:CC:DD:EE:01",
			noise: 20, noiseValid: true, transmitRate: 4.0,
			macAddress: "hardware-address", phyMode: "802.11ac",
		}, nil
	}
	getNearbyAPs = func() ([]accessPointInfo, error) { return nil, nil }
	defer func() { getWiFiInfo = nil; getNearbyAPs = nil }()

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, integration.Data("ssid_exclude:\n  - HomeNet\n"), "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	err := wlanCheck.Run()
	assert.NoError(t, err)
	// Connected network is excluded -> no connected metrics, no status.
	mockSender.AssertMetricMissing(t, "Gauge", "system.wlan.rssi")
	mockSender.AssertMetricMissing(t, "Gauge", "system.wlan.status")
}

func TestWLANScanFilterGatesScan(t *testing.T) {
	// Connected SSID does not match scan_ssid_include -> scan must not run.
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi: 10, ssid: "HomeNet", bssid: "AA:BB:CC:DD:EE:01",
			noise: 20, noiseValid: true, transmitRate: 4.0,
			macAddress: "hardware-address", phyMode: "802.11ac",
		}, nil
	}
	getNearbyAPs = func() ([]accessPointInfo, error) {
		t.Fatal("scan must not run when connected network is not in scan_ssid_include")
		return nil, nil
	}
	defer func() { getWiFiInfo = nil; getNearbyAPs = nil }()

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, integration.Data("scan_ssid_include:\n  - \"Corp-*\"\n"), "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	err := wlanCheck.Run()
	assert.NoError(t, err)
	// Connected metrics still emit (metric filter is allow-all); scan does not.
	mockSender.AssertMetric(t, "Gauge", "system.wlan.rssi", 10.0, "", []string{"ssid:HomeNet", "bssid:AA:BB:CC:DD:EE:01", "mac_address:hardware-address", "status:ok"})
	mockSender.AssertMetricMissing(t, "Gauge", "system.wlan.scan.rssi")
}

func TestWLANScanFilterAllowsGlob(t *testing.T) {
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{
			rssi: 10, ssid: "Corp-5G", bssid: "AA:BB:CC:DD:EE:01",
			noise: 20, noiseValid: true, transmitRate: 4.0,
			macAddress: "hardware-address", phyMode: "802.11ac",
		}, nil
	}
	getNearbyAPs = func() ([]accessPointInfo, error) {
		return []accessPointInfo{{rssi: -60, ssid: "Corp-5G", bssid: "AA:BB:CC:DD:EE:02"}}, nil
	}
	defer func() { getWiFiInfo = nil; getNearbyAPs = nil }()

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, integration.Data("scan_ssid_include:\n  - \"Corp-*\"\n"), "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	err := wlanCheck.Run()
	assert.NoError(t, err)
	mockSender.AssertMetric(t, "Gauge", "system.wlan.scan.rssi", -60.0, "", []string{"ssid:Corp-5G", "bssid:AA:BB:CC:DD:EE:02", "connected:0"})
}

func TestWLANAPScanRSSICutoffKeepsConnected(t *testing.T) {
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{rssi: -85, ssid: "test-ssid", bssid: "AA:BB:CC:DD:EE:01", macAddress: "m", phyMode: "802.11ac"}, nil
	}
	getNearbyAPs = func() ([]accessPointInfo, error) {
		return []accessPointInfo{
			{rssi: -85, ssid: "test-ssid", bssid: "AA:BB:CC:DD:EE:01"}, // connected, weak (kept anyway)
			{rssi: -60, ssid: "near-strong", bssid: "AA:BB:CC:DD:EE:02"},
			{rssi: -90, ssid: "near-weak", bssid: "AA:BB:CC:DD:EE:03"}, // below cutoff -> dropped
		}, nil
	}
	defer func() { getWiFiInfo = nil; getNearbyAPs = nil }()

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	// Exclude connected metrics so only scan.rssi gauges are emitted (isolates the count).
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, integration.Data("ap_scan_rssi_cutoff: -80\nssid_exclude:\n  - \"*\"\n"), "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	err := wlanCheck.Run()
	assert.NoError(t, err)
	mockSender.AssertMetric(t, "Gauge", "system.wlan.scan.rssi", -85.0, "", []string{"ssid:test-ssid", "bssid:AA:BB:CC:DD:EE:01", "connected:1"})
	mockSender.AssertMetric(t, "Gauge", "system.wlan.scan.rssi", -60.0, "", []string{"ssid:near-strong", "bssid:AA:BB:CC:DD:EE:02", "connected:0"})
	// Only connected(-85) and near-strong(-60) survive; near-weak(-90) dropped.
	mockSender.AssertNumberOfCalls(t, "Gauge", 2)
}

func TestWLANAPScanLimitKeepsStrongestPlusConnected(t *testing.T) {
	getWiFiInfo = func() (wifiInfo, error) {
		return wifiInfo{rssi: -50, ssid: "test-ssid", bssid: "AA:BB:CC:DD:EE:01", macAddress: "m", phyMode: "802.11ac"}, nil
	}
	getNearbyAPs = func() ([]accessPointInfo, error) {
		return []accessPointInfo{
			{rssi: -50, ssid: "test-ssid", bssid: "AA:BB:CC:DD:EE:01"}, // connected
			{rssi: -70, ssid: "n3", bssid: "AA:BB:CC:DD:EE:04"},
			{rssi: -60, ssid: "n2", bssid: "AA:BB:CC:DD:EE:03"},
			{rssi: -80, ssid: "n4", bssid: "AA:BB:CC:DD:EE:05"},
			{rssi: -55, ssid: "n1", bssid: "AA:BB:CC:DD:EE:02"},
		}, nil
	}
	defer func() { getWiFiInfo = nil; getNearbyAPs = nil }()

	wlanCheck := new(WLANCheck)
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, integration.Data("ap_scan_limit: 2\nssid_exclude:\n  - \"*\"\n"), "test", "provider")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	err := wlanCheck.Run()
	assert.NoError(t, err)
	// connected(-50) always + 2 strongest nearby (-55, -60).
	mockSender.AssertMetric(t, "Gauge", "system.wlan.scan.rssi", -50.0, "", []string{"ssid:test-ssid", "bssid:AA:BB:CC:DD:EE:01", "connected:1"})
	mockSender.AssertMetric(t, "Gauge", "system.wlan.scan.rssi", -55.0, "", []string{"ssid:n1", "bssid:AA:BB:CC:DD:EE:02", "connected:0"})
	mockSender.AssertMetric(t, "Gauge", "system.wlan.scan.rssi", -60.0, "", []string{"ssid:n2", "bssid:AA:BB:CC:DD:EE:03", "connected:0"})
	mockSender.AssertNumberOfCalls(t, "Gauge", 3)
}
