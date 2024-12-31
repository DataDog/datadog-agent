// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PLINT) Fix revive linter
package wlan

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const (
	CheckName                    = "wlan"
	defaultMinCollectionInterval = 15
)

var getWiFiInfo = GetWiFiInfo

var lastChannelID int = -1
var lastBSSID string = ""
var lastSSID string = ""

// WiFiInfo contains information about the WiFi connection as defined in wlan_darwin.h
type WiFiInfo struct {
	Rssi            int
	Ssid            string
	Bssid           string
	Channel         int
	Noise           int
	TransmitRate    float64
	HardwareAddress string
}

// WLANCheck monitors the status of the WLAN interface
type WLANCheck struct {
	core.CheckBase
}

func (c *WLANCheck) String() string {
	return "wlan"
}

// Run runs the check
func (c *WLANCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	wifiInfo, err := getWiFiInfo()
	if err != nil {
		log.Error(err)
		sender.Commit()
		return err
	}

	ssid := wifiInfo.Ssid
	if ssid == "" {
		ssid = "unknown"
	}
	bssid := wifiInfo.Bssid
	if bssid == "" {
		bssid = "unknown"
	}
	hardwareAddress := strings.ToLower(strings.Replace(wifiInfo.HardwareAddress, " ", "_", -1))

	tags := []string{}
	tags = append(tags, "ssid:"+ssid)
	tags = append(tags, "bssid:"+bssid)
	tags = append(tags, "mac_address:"+hardwareAddress)

	sender.Gauge("wlan.rssi", float64(wifiInfo.Rssi), "", tags)
	sender.Gauge("wlan.noise", float64(wifiInfo.Noise), "", tags)
	sender.Gauge("wlan.transmit_rate", float64(wifiInfo.TransmitRate), "", tags)

	// channel swap events
	if lastChannelID != -1 && lastChannelID != wifiInfo.Channel {
		sender.Count("wlan.channel_swap_events", 1.0, "", tags)
	} else {
		sender.Count("wlan.channel_swap_events", 0.0, "", tags)
	}

	// roaming events / ssid swap events
	if lastBSSID != "" && lastSSID != "" && lastBSSID == wifiInfo.Bssid && lastSSID != wifiInfo.Ssid {
		sender.Count("wlan.roaming_events", 1.0, "", tags)
	} else {
		sender.Count("wlan.roaming_events", 0.0, "", tags)
	}

	// update last values
	lastChannelID = wifiInfo.Channel
	lastBSSID = wifiInfo.Bssid
	lastSSID = wifiInfo.Ssid

	sender.Commit()
	return nil
}

// Factory creates a new check factory
func Factory() optional.Option[func() check.Check] {
	return optional.NewOption(newCheck)
}

func newCheck() check.Check {
	return &WLANCheck{
		CheckBase: core.NewCheckBaseWithInterval(CheckName, time.Duration(defaultMinCollectionInterval)*time.Second),
	}
}
