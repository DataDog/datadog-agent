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

// wifiInfo contains information about the WiFi connection (defined in Mac wlan_darwin.h and Windows wlan.h)
type wifiInfo struct {
	rssi             int
	ssid             string
	bssid            string
	channel          int
	noise            int
	noiseValid       bool
	transmitRate     float64 // in Mbps
	receiveRate      float64 // in Mbps
	receiveRateValid bool
	macAddress       string
	phyMode          string
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

	if wifiInfo.phyMode == "None" {
		log.Warn("No active Wi-Fi interface detected: PHYMode is none.")
		return nil
	}

	ssid := wifiInfo.ssid
	if ssid == "" {
		ssid = "unknown"
	}
	bssid := wifiInfo.bssid
	if bssid == "" {
		bssid = "unknown"
	}

	macAddress := strings.ToLower(strings.Replace(wifiInfo.macAddress, " ", "_", -1))
	if macAddress == "" {
		macAddress = "unknown"
	}

	tags := []string{}
	tags = append(tags, "ssid:"+ssid)
	tags = append(tags, "bssid:"+bssid)
	tags = append(tags, "mac_address:"+macAddress)

	sender.Gauge("wlan.rssi", float64(wifiInfo.rssi), "", tags)
	if wifiInfo.noiseValid {
		sender.Gauge("wlan.noise", float64(wifiInfo.noise), "", tags)
	}
	sender.Gauge("wlan.transmit_rate", float64(wifiInfo.transmitRate), "", tags)
	if wifiInfo.receiveRateValid {
		sender.Gauge("wlan.receive_rate", float64(wifiInfo.receiveRate), "", tags)
	}

	roamingEvent := 0.0
	channelSwapEvent := 0.0
	if c.isWarmedUp {
		// Check if the BSSID or channel has changed since the last run
		if c.lastBSSID != wifiInfo.bssid {
			// BSSID has changed
			roamingEvent = 1.0
		} else if c.lastChannelID != wifiInfo.channel {
			// Channel has changed (if the BSSID is changed it is roaming and not channel swap)
			channelSwapEvent = 1.0
		}
	}
	sender.Count("wlan.roaming_events", roamingEvent, "", tags)
	sender.Count("wlan.channel_swap_events", channelSwapEvent, "", tags)

	// update last values
	c.lastChannelID = wifiInfo.channel
	c.lastBSSID = wifiInfo.bssid
	c.lastSSID = wifiInfo.ssid
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
