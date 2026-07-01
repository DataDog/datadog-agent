// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PLINT) Fix revive linter
package wlan

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gobwas/glob"
	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
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

// accessPointInfo contains the subset of WiFi data we report per visible
// access point during a scan (connected or not). RSSI is the payload; ssid
// and bssid are used for tagging. Whether an AP is the connected one is
// derived at emit time by comparing its BSSID to the connected BSSID, so it
// is intentionally not stored here.
type accessPointInfo struct {
	rssi  int
	ssid  string
	bssid string
}

// wlanInitConfig mirrors the init_config section of wlan.d/conf.yaml.
type wlanInitConfig struct {
	RequestLocationPermission bool `yaml:"request_location_permission"`

	// APScan toggles emission of system.wlan.scan.rssi for every visible
	// access point (connected and nearby). A nil pointer means the option was
	// omitted, in which case it defaults to enabled.
	APScan *bool `yaml:"ap_scan"`

	// APScanLimit, when set and > 0, caps the nearby access points reported to
	// the N strongest by RSSI. The connected AP is always reported and does
	// not count against the limit. nil/0 means no limit.
	APScanLimit *int `yaml:"ap_scan_limit"`

	// APScanRSSICutoff, when set, drops nearby access points whose RSSI is
	// below this value (in dBm, e.g. -80). The connected AP is exempt.
	APScanRSSICutoff *int `yaml:"ap_scan_rssi_cutoff"`

	// Metric filter (filter 1): gate the connected-AP metrics by the connected
	// network's SSID/BSSID. Glob patterns. Empty lists allow everything.
	SSIDInclude  []string `yaml:"ssid_include"`
	SSIDExclude  []string `yaml:"ssid_exclude"`
	BSSIDInclude []string `yaml:"bssid_include"`
	BSSIDExclude []string `yaml:"bssid_exclude"`

	// Scan filter (filter 2): gate whether the nearby-AP scan runs, by the
	// connected network's SSID/BSSID (e.g. "scan only at work"). Glob patterns.
	ScanSSIDInclude  []string `yaml:"scan_ssid_include"`
	ScanSSIDExclude  []string `yaml:"scan_ssid_exclude"`
	ScanBSSIDInclude []string `yaml:"scan_bssid_include"`
	ScanBSSIDExclude []string `yaml:"scan_bssid_exclude"`
}

// apFilter decides whether an access point identified by (ssid, bssid) should
// be collected, based on glob include/exclude lists. ssid and bssid lists are
// OR-combined within the include set and within the exclude set; exclude wins.
// A filter with no lists allows everything.
type apFilter struct {
	ssidInclude  []glob.Glob
	ssidExclude  []glob.Glob
	bssidInclude []glob.Glob
	bssidExclude []glob.Glob
}

// compileGlobs compiles patterns (lowercased for case-insensitive matching),
// logging and skipping any that are invalid.
func compileGlobs(patterns []string) []glob.Glob {
	var globs []glob.Glob
	for _, p := range patterns {
		g, err := glob.Compile(strings.ToLower(p))
		if err != nil {
			log.Warnf("Ignoring invalid wlan filter pattern %q: %v", p, err)
			continue
		}
		globs = append(globs, g)
	}
	return globs
}

// newAPFilter builds a filter from include/exclude string lists.
func newAPFilter(ssidInc, ssidExc, bssidInc, bssidExc []string) apFilter {
	return apFilter{
		ssidInclude:  compileGlobs(ssidInc),
		ssidExclude:  compileGlobs(ssidExc),
		bssidInclude: compileGlobs(bssidInc),
		bssidExclude: compileGlobs(bssidExc),
	}
}

func matchAny(globs []glob.Glob, s string) bool {
	for _, g := range globs {
		if g.Match(s) {
			return true
		}
	}
	return false
}

// allowed reports whether an AP with the given ssid/bssid passes the filter.
func (f apFilter) allowed(ssid, bssid string) bool {
	ssid = strings.ToLower(ssid)
	bssid = strings.ToLower(bssid)

	if matchAny(f.ssidExclude, ssid) || matchAny(f.bssidExclude, bssid) {
		return false
	}

	hasInclude := len(f.ssidInclude) > 0 || len(f.bssidInclude) > 0
	if !hasInclude {
		return true
	}
	return matchAny(f.ssidInclude, ssid) || matchAny(f.bssidInclude, bssid)
}

// WLANCheck monitors the status of the WLAN interface
type WLANCheck struct {
	core.CheckBase
	lastChannel int
	lastBSSID   string
	lastSSID    string
	isWarmedUp  bool

	// Forwarded to the GUI over IPC so it can decide whether to prompt for
	// Location Services permission. The agent owns the config; the GUI has no
	// read access to auth_token / ipc_cert.pem and cannot query the agent itself.
	requestLocationPermission bool

	// When true, emit system.wlan.scan.rssi for every visible access point
	// (connected and nearby). Defaults to true; disabled via init_config.
	apScanEnabled bool

	// apScanLimit caps reported nearby APs to the N strongest (0 = unlimited).
	// apScanRSSICutoff drops nearby APs below this RSSI (nil = no cutoff).
	// The connected AP is exempt from both.
	apScanLimit      int
	apScanRSSICutoff *int

	// metricFilter gates the connected-AP metrics; scanFilter gates whether
	// the nearby-AP scan runs. Both match the connected network's ssid/bssid.
	metricFilter apFilter
	scanFilter   apFilter
}

// Configure reads request_location_permission from init_config so it can be
// forwarded to the GUI on every check run.
func (c *WLANCheck) Configure(senderManager sender.SenderManager, _ uint64, data integration.Data, initConfig integration.Data, source string, provider string) error {
	if err := c.CommonConfigure(senderManager, initConfig, data, source, provider); err != nil {
		return err
	}

	s, err := c.GetSender()
	if err != nil {
		return err
	}
	s.FinalizeCheckServiceTag()

	var ic wlanInitConfig
	if len(initConfig) > 0 {
		if err := yaml.Unmarshal(initConfig, &ic); err != nil {
			return fmt.Errorf("parsing wlan init_config: %w", err)
		}
	}
	c.requestLocationPermission = ic.RequestLocationPermission
	// Default to enabled when the option is omitted from init_config.
	c.apScanEnabled = ic.APScan == nil || *ic.APScan

	c.apScanLimit = 0
	if ic.APScanLimit != nil && *ic.APScanLimit > 0 {
		c.apScanLimit = *ic.APScanLimit
	}
	c.apScanRSSICutoff = ic.APScanRSSICutoff

	c.metricFilter = newAPFilter(ic.SSIDInclude, ic.SSIDExclude, ic.BSSIDInclude, ic.BSSIDExclude)
	c.scanFilter = newAPFilter(ic.ScanSSIDInclude, ic.ScanSSIDExclude, ic.ScanBSSIDInclude, ic.ScanBSSIDExclude)
	return nil
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

	// Filter 1: only emit connected-AP metrics for allowed networks. When the
	// connected network is filtered out we emit nothing for it (so its
	// ssid/bssid never appear in tags) and skip the state update below so a
	// later allowed sample does not register a false roam/channel-swap.
	if c.metricFilter.allowed(wi.ssid, wi.bssid) {
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
	} else {
		log.Debugf("Connected network filtered out by metric include/exclude; skipping connected WLAN metrics")
	}

	// Filter 2: emit per-AP RSSI only when scanning is enabled and the
	// connected network is allowed to scan ("scan only at work"). Gating here
	// avoids triggering the relatively expensive active scan when not wanted.
	if c.apScanEnabled && c.scanFilter.allowed(wi.ssid, wi.bssid) {
		c.emitNearbyAPs(sender, wi.bssid)
	}

	sender.Commit()
	return nil
}

// emitNearbyAPs scans for all visible access points and emits one
// system.wlan.scan.rssi gauge per AP, tagged with ssid, bssid, and
// connected:1|0 (1 for the AP matching connectedBSSID, 0 otherwise).
//
// The connected AP is always reported. Nearby (non-connected) APs are first
// dropped below apScanRSSICutoff (when set), then the strongest apScanLimit of
// the remainder are kept (when set). Both limits exempt the connected AP.
//
// Failure to scan is non-fatal: the connected-AP metrics have already been
// emitted in Run(), so we log and return rather than failing the check.
func (c *WLANCheck) emitNearbyAPs(sender sender.Sender, connectedBSSID string) {
	aps, err := c.GetNearbyAccessPoints()
	if err != nil {
		log.Warnf("Failed to scan for nearby access points: %v", err)
		return
	}

	// Partition into the connected AP (always reported) and the rest.
	var connectedAP *accessPointInfo
	others := make([]accessPointInfo, 0, len(aps))
	for i := range aps {
		ap := aps[i]
		if connectedAP == nil && connectedBSSID != "" && strings.EqualFold(ap.bssid, connectedBSSID) {
			connectedAP = &aps[i]
			continue
		}
		others = append(others, ap)
	}

	// RSSI cutoff on nearby APs only.
	if c.apScanRSSICutoff != nil {
		filtered := others[:0]
		for _, ap := range others {
			if ap.rssi >= *c.apScanRSSICutoff {
				filtered = append(filtered, ap)
			}
		}
		others = filtered
	}

	// Keep the strongest N nearby APs (connected AP excluded from the cap).
	if c.apScanLimit > 0 && len(others) > c.apScanLimit {
		sort.SliceStable(others, func(i, j int) bool { return others[i].rssi > others[j].rssi })
		others = others[:c.apScanLimit]
	}

	if connectedAP != nil {
		c.emitScanRSSI(sender, *connectedAP, connectedBSSID)
	}
	for _, ap := range others {
		c.emitScanRSSI(sender, ap, connectedBSSID)
	}
}

// emitScanRSSI emits a single system.wlan.scan.rssi gauge for one access point.
func (c *WLANCheck) emitScanRSSI(sender sender.Sender, ap accessPointInfo, connectedBSSID string) {
	ssid := ap.ssid
	if ssid == "" {
		ssid = "unknown"
	}
	bssid := ap.bssid
	if bssid == "" {
		bssid = "unknown"
	}

	connected := "0"
	if connectedBSSID != "" && strings.EqualFold(ap.bssid, connectedBSSID) {
		connected = "1"
	}

	tags := []string{
		"ssid:" + ssid,
		"bssid:" + bssid,
		"connected:" + connected,
	}
	sender.Gauge("system.wlan.scan.rssi", float64(ap.rssi), "", tags)
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
