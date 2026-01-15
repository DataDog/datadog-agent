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
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

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
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

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
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

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
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

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

	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")
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

	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

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

	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

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

	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

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

	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

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

	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

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

	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

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
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

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
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

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
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

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
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

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
	senderManager := mocksender.CreateDefaultDemultiplexer()
	wlanCheck.Configure(senderManager, integration.FakeConfigHash, nil, nil, "test")

	mockSender := mocksender.NewMockSenderWithSenderManager(wlanCheck.ID(), senderManager)
	mockSender.SetupAcceptAll()

	wlanCheck.Run()
	mockSender.AssertMetric(t, "Gauge", "system.wlan.rxrate", 5.0, "", expectedTags)
}
