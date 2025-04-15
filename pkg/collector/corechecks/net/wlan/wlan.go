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

	// Roaming and channel swap events could happen only if we are still on the same network (and warmed up).
	// Notes:
	//   * SSID could be empty if it is not "advertised" by the AP and in this case
	//     the sameness of the network is determined by the matching BSSID.
	//   * BSSID could not be empty (it is an error condition) and it would not
	//     contribute to the sameness of the network. If current or previous sample of BSSID
	//     is empty, we still cannot determine if we are roaming or have changed the channel.
	if c.isWarmedUp && c.lastSSID == wifiInfo.ssid && len(c.lastBSSID) > 0 && len(wifiInfo.bssid) > 0 {
		if len(c.lastSSID) > 0 {
			// The same non-empty SSID
			if c.lastBSSID != wifiInfo.bssid {
				roamingEvent = 1.0
			} else if c.lastChannelID != wifiInfo.channel {
				channelSwapEvent = 1.0
			}
		} else {
			// Empty SSID, roaming detection is not possible but channel swapping is
			if c.lastBSSID == wifiInfo.bssid && c.lastChannelID != wifiInfo.channel {
				channelSwapEvent = 1.0
			}
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
