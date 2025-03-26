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
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

const (
	CheckName                    = "wlan"
	defaultMinCollectionInterval = 15
)

var getWiFiInfo = GetWiFiInfo

// WiFiInfo contains information about the WiFi connection as defined in wlan_darwin.h
type WiFiInfo struct {
	Rssi         int
	Ssid         string
	Bssid        string
	Channel      int
	Noise        int
	TransmitRate float64 // in Mbps
	MacAddress   string
	PHYMode      string
}

// WLANCheck monitors the status of the WLAN interface
type WLANCheck struct {
	core.CheckBase
	lastChannelID int
	lastBSSID     string
	lastSSID      string
	isWarmedUp    bool
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
		return err
	}

	if wifiInfo.PHYMode == "None" {
		log.Warn("No active Wi-Fi interface detected: PHYMode is none.")
		return nil
	}

	ssid := wifiInfo.Ssid
	if ssid == "" {
		ssid = "unknown"
	}
	bssid := wifiInfo.Bssid
	if bssid == "" {
		bssid = "unknown"
	}

	macAddress := strings.ToLower(strings.Replace(wifiInfo.MacAddress, " ", "_", -1))
	if macAddress == "" {
		macAddress = "unknown"
	}

	tags := []string{}
	tags = append(tags, "ssid:"+ssid)
	tags = append(tags, "bssid:"+bssid)
	tags = append(tags, "mac_address:"+macAddress)

	sender.Gauge("wlan.rssi", float64(wifiInfo.Rssi), "", tags)
	sender.Gauge("wlan.noise", float64(wifiInfo.Noise), "", tags)
	sender.Gauge("wlan.transmit_rate", float64(wifiInfo.TransmitRate), "", tags)

	// channel swap events
	if c.isWarmedUp && c.lastChannelID != wifiInfo.Channel {
		sender.Count("wlan.channel_swap_events", 1.0, "", tags)
	} else {
		sender.Count("wlan.channel_swap_events", 0.0, "", tags)
	}

	// roaming events / ssid swap events
	if c.isWarmedUp && c.lastBSSID != "" && c.lastSSID != "" && c.lastBSSID == wifiInfo.Bssid && c.lastSSID != wifiInfo.Ssid {
		sender.Count("wlan.roaming_events", 1.0, "", tags)
	} else {
		sender.Count("wlan.roaming_events", 0.0, "", tags)
	}

	// update last values
	c.lastChannelID = wifiInfo.Channel
	c.lastBSSID = wifiInfo.Bssid
	c.lastSSID = wifiInfo.Ssid
	c.isWarmedUp = true

	sender.Commit()
	return nil
}

// Factory creates a new check factory
func Factory() option.Option[func() check.Check] {
	return option.New(newCheck)
}

func newCheck() check.Check {
	return &WLANCheck{
		CheckBase: core.NewCheckBaseWithInterval(CheckName, time.Duration(defaultMinCollectionInterval)*time.Second),
	}
}
