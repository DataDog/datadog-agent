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

// Status metric values (replacing deprecated service checks)
const (
	statusOK       float64 = 0 // WiFi operational
	statusWarning  float64 = 1 // WiFi interface inactive
	statusCritical float64 = 2 // WiFi collection failed
)

// Run runs the check
func (c *WLANCheck) Run() error {
	sender, err := c.GetSender()
	if err != nil {
		return err
	}

	// Attempt to get WiFi info from GUI via IPC
	wi, err := c.GetWiFiInfo()
	if err != nil {
		// Failed to get WiFi info - emit CRITICAL status
		log.Errorf("WLAN check failed: %v", err)
		log.Error("Ensure the Datadog Agent GUI is running for WiFi metrics collection on macOS 15+")

		// Emit status metric: CRITICAL (replaces deprecated service check)
		sender.Gauge("system.wlan.status", statusCritical, "", []string{
			"status:critical",
			"reason:ipc_failure",
		})
		// Track error count for monitoring
		sender.Count("system.wlan.check.errors", 1, "", []string{
			"error_type:ipc_failure",
		})
		sender.Commit()
		return err
	}

	// Check if WiFi interface is active
	if wi.phyMode == "None" {
		log.Warn("No active Wi-Fi interface detected: PHYMode is none.")

		// Emit status metric: WARNING (replaces deprecated service check)
		sender.Gauge("system.wlan.status", statusWarning, "", []string{
			"status:warning",
			"reason:interface_inactive",
		})
		sender.Commit()
		return nil
	}

	// Prepare tags
	ssid := wi.ssid
	if ssid == "" {
		ssid = "unknown"
		log.Debug("SSID is empty - this may indicate missing location permission")
	}
	bssid := wi.bssid
	if bssid == "" {
		bssid = "unknown"
		log.Debug("BSSID is empty - this may indicate missing location permission")
	}

	macAddress := strings.ToLower(strings.ReplaceAll(wi.macAddress, " ", "_"))
	if macAddress == "" {
		macAddress = "unknown"
	}

	tags := []string{
		"ssid:" + ssid,
		"bssid:" + bssid,
		"mac_address:" + macAddress,
		"status:ok",
	}

	// WiFi data collected successfully - emit OK status (replaces deprecated service check)
	sender.Gauge("system.wlan.status", statusOK, "", tags)

	// Emit metrics
	sender.Gauge("system.wlan.rssi", float64(wi.rssi), "", tags)
	if wi.noiseValid {
		sender.Gauge("system.wlan.noise", float64(wi.noise), "", tags)
	}
	sender.Gauge("system.wlan.txrate", float64(wi.transmitRate), "", tags)
	if wi.receiveRateValid {
		sender.Gauge("system.wlan.rxrate", float64(wi.receiveRate), "", tags)
	}

	// Emit event metrics for roaming and channel swaps
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

	// Update last values for next run
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
