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
	lastChannel int
	lastBSSID   string
	lastSSID    string
	isWarmedUp  bool
}

func (c *WLANCheck) String() string {
	return "wlan"
}

func (c *WLANCheck) isRoaming(wi *wifiInfo) bool {
	// cannot determine roaming without a previous state <SSID,BSSID>
	if !c.isWarmedUp {
		return false
	}

	// current and previous BSSIDs should not be empty, otherwise we cannot
	// actually determine if we are roaming or not
	if len(c.lastBSSID) == 0 || len(wi.bssid) == 0 {
		return false
	}

	// current and previous SSIDs should not be empty, otherwise we cannot
	// actually determine if are on the same network
	if len(c.lastSSID) == 0 || len(wi.ssid) == 0 {
		return false
	}

	// current and previous sample has to be in the same network (SSID)
	if c.lastSSID != wi.ssid {
		return false
	}

	// has to be in the same network (SSID) but in a different AP (BSSID)
	return c.lastBSSID != wi.bssid
}

func (c *WLANCheck) isChannelSwap(wi *wifiInfo) bool {
	// cannot determine roaming without a previous state <SSID,BSSID>
	if !c.isWarmedUp {
		return false
	}

	// current and previous BSSIDs should not be empty, otherwise we cannot
	// actually determine if we are channel swapping or not
	if len(c.lastBSSID) == 0 || len(wi.bssid) == 0 {
		return false
	}

	// current and previous SSIDs should be equal (empty SSID is valid if the AP does not advertise it)
	if c.lastSSID != wi.ssid {
		return false
	}

	// has to be in the same network (SSID) and on the same AP (BSSID)
	if c.lastBSSID != wi.bssid {
		return false
	}

	// has to be in the same network (SSID) and the same AP (BSSID) but in a different channel
	return c.lastChannel != wi.channel
}

// Run runs the check
func (c *WLANCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}
	wi, err := getWiFiInfo()
	if err != nil {
		log.Error(err)
		return err
	}

	if wi.phyMode == "None" {
		log.Warn("No active Wi-Fi interface detected: PHYMode is none.")
		return nil
	}

	ssid := wi.ssid
	if ssid == "" {
		ssid = "unknown"
	}
	bssid := wi.bssid
	if bssid == "" {
		bssid = "unknown"
	}

	macAddress := strings.ToLower(strings.ReplaceAll(wi.macAddress, " ", "_"))
	if macAddress == "" {
		macAddress = "unknown"
	}

	tags := []string{}
	tags = append(tags, "ssid:"+ssid)
	tags = append(tags, "bssid:"+bssid)
	tags = append(tags, "mac_address:"+macAddress)

	sender.Gauge("system.wlan.rssi", float64(wi.rssi), "", tags)
	if wi.noiseValid {
		sender.Gauge("system.wlan.noise", float64(wi.noise), "", tags)
	}
	sender.Gauge("system.wlan.txrate", float64(wi.transmitRate), "", tags)
	if wi.receiveRateValid {
		sender.Gauge("system.wlan.rxrate", float64(wi.receiveRate), "", tags)
	}

	if c.isRoaming(&wi) {
		sender.Count("system.wlan.roaming_events", 1.0, "", tags)
		sender.Count("system.wlan.channel_swap_events", 0.0, "", tags)
	} else if c.isChannelSwap(&wi) {
		sender.Count("system.wlan.roaming_events", 0.0, "", tags)
		sender.Count("system.wlan.channel_swap_events", 1.0, "", tags)
	} else {
		sender.Count("system.wlan.roaming_events", 0.0, "", tags)
		sender.Count("system.wlan.channel_swap_events", 0.0, "", tags)
	}

	// update last values
	c.lastChannel = wi.channel
	c.lastBSSID = wi.bssid
	c.lastSSID = wi.ssid
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
