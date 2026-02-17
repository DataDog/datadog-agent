// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package logondurationimpl implements the logon duration component
package logondurationimpl

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/sys/windows"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	logonduration "github.com/DataDog/datadog-agent/comp/logonduration/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// persistentCacheKey stores the last boot time to detect reboots across agent restarts.
// Stored at: run/logon_duration/last_boot_time
const persistentCacheKey = "logon_duration:last_boot_time"

// Requires defines the dependencies for the logon duration component
type Requires struct {
	Lc            compdef.Lifecycle
	Config        configcomp.Component
	Log           logcomp.Component
	EventPlatform eventplatform.Component
	Hostname      hostname.Component
}

// Provides defines what this component provides
type Provides struct {
	Comp logonduration.Component
}

type logonDurationComponent struct {
	config        configcomp.Component
	hostname      hostname.Component
	eventPlatform eventplatform.Component
}

// NewComponent creates a new logon duration component
func NewComponent(reqs Requires) Provides {
	if !reqs.Config.GetBool("logon_duration.enabled") {
		log.Debug("Logon duration component is disabled")
		return Provides{
			Comp: &logonDurationComponent{},
		}
	}

	forwarder, ok := reqs.EventPlatform.Get()
	if !ok {
		log.Error("Logon duration: failed to get event platform forwarder")
		return Provides{
			Comp: &logonDurationComponent{},
		}
	}
	// Ensure the forwarder is available (suppress unused warning)
	_ = forwarder

	comp := &logonDurationComponent{
		config:        reqs.Config,
		hostname:      reqs.Hostname,
		eventPlatform: reqs.EventPlatform,
	}

	reqs.Lc.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			return comp.run()
		},
	})

	log.Debug("Logon duration component initialized")

	return Provides{
		Comp: comp,
	}
}

// run executes the one-shot logon duration analysis:
//  1. Detect if a reboot occurred since last run (via persistent cache).
//  2. If no reboot, update cache and return.
//  3. If reboot, parse the ETL file, build a payload, and submit a notable event.
func (c *logonDurationComponent) run() error {
	rebooted, currentBootTime, err := detectReboot()
	if err != nil {
		log.Warnf("Logon duration: failed to detect reboot: %v", err)
		return nil
	}

	if !rebooted {
		log.Debug("Logon duration: no reboot detected since last run, skipping")
		return nil
	}

	log.Info("Logon duration: reboot detected, analyzing boot trace")

	etlPath := c.config.GetString("logon_duration.etl_path")
	if etlPath == "" {
		log.Warn("Logon duration: no ETL path configured (logon_duration.etl_path), skipping analysis")
		// Still update the cache so we don't re-trigger on every restart
		if err := persistentcache.Write(persistentCacheKey, currentBootTime); err != nil {
			log.Warnf("Logon duration: failed to update persistent cache: %v", err)
		}
		return nil
	}

	result, err := analyzeETL(etlPath)
	if err != nil {
		log.Errorf("Logon duration: failed to analyze ETL file: %v", err)
		// Update cache even on parse failure to avoid retrying the same boot
		if cacheErr := persistentcache.Write(persistentCacheKey, currentBootTime); cacheErr != nil {
			log.Warnf("Logon duration: failed to update persistent cache: %v", cacheErr)
		}
		return nil
	}

	if err := c.submitEvent(result); err != nil {
		log.Errorf("Logon duration: failed to submit event: %v", err)
	}

	if err := persistentcache.Write(persistentCacheKey, currentBootTime); err != nil {
		log.Warnf("Logon duration: failed to update persistent cache: %v", err)
	}

	log.Info("Logon duration: boot analysis complete")
	return nil
}

// detectReboot checks whether the system has rebooted since the last agent run
// by comparing the current boot time with the value stored in persistent cache.
// Returns (rebooted, currentBootTimeString, error).
func detectReboot() (bool, string, error) {
	currentBootTime, err := getLastBootTime()
	if err != nil {
		return false, "", fmt.Errorf("getting current boot time: %w", err)
	}

	previousBootTime, err := persistentcache.Read(persistentCacheKey)
	if err != nil {
		return false, currentBootTime, fmt.Errorf("reading persistent cache: %w", err)
	}

	// First run (no cached value) or boot time changed → reboot detected
	if previousBootTime == "" || previousBootTime != currentBootTime {
		return true, currentBootTime, nil
	}

	return false, currentBootTime, nil
}

// getLastBootTime returns the system's last boot time as a string by computing
// time.Now() minus the system uptime (via GetTickCount64). The result is
// truncated to second precision so that minor tick differences across agent
// restarts don't cause false reboot detections.
func getLastBootTime() (string, error) {
	tickCount := windows.GetTickCount64()
	uptime := time.Duration(tickCount) * time.Millisecond
	bootTime := time.Now().Add(-uptime).Truncate(time.Second)
	return bootTime.UTC().Format(time.RFC3339), nil
}

// submitEvent builds an Event Management v2 payload from the analysis result
// and sends it through the event platform forwarder.
func (c *logonDurationComponent) submitEvent(result *AnalysisResult) error {
	forwarder, ok := c.eventPlatform.Get()
	if !ok {
		return fmt.Errorf("event platform forwarder not available")
	}

	hostnameValue := c.hostname.GetSafe(context.TODO())
	tl := result.Timeline

	// Compute key durations (zero if the milestone was not observed)
	durations := make(map[string]interface{})
	if !tl.BootStart.IsZero() && !tl.DesktopReady.IsZero() {
		durations["boot_to_desktop_ms"] = tl.DesktopReady.Sub(tl.BootStart).Milliseconds()
	}
	if !tl.BootStart.IsZero() && !tl.ExplorerStart.IsZero() {
		durations["boot_to_explorer_ms"] = tl.ExplorerStart.Sub(tl.BootStart).Milliseconds()
	}
	if !tl.LogonStart.IsZero() && !tl.ShellStarted.IsZero() {
		durations["logon_to_shell_ms"] = tl.ShellStarted.Sub(tl.LogonStart).Milliseconds()
	}
	if !tl.ProfileStart.IsZero() && !tl.ProfileEnd.IsZero() {
		durations["profile_load_ms"] = tl.ProfileEnd.Sub(tl.ProfileStart).Milliseconds()
	}
	if !tl.MachineGPStart.IsZero() && !tl.MachineGPEnd.IsZero() {
		durations["machine_gp_ms"] = tl.MachineGPEnd.Sub(tl.MachineGPStart).Milliseconds()
	}
	if !tl.UserGPStart.IsZero() && !tl.UserGPEnd.IsZero() {
		durations["user_gp_ms"] = tl.UserGPEnd.Sub(tl.UserGPStart).Milliseconds()
	}

	// Build subscriber timing list
	subscriberData := make([]map[string]interface{}, 0, len(result.Subscribers))
	for _, sub := range result.Subscribers {
		subscriberData = append(subscriberData, map[string]interface{}{
			"name":        sub.Name,
			"duration_ms": sub.Duration.Milliseconds(),
		})
	}

	// Determine the event timestamp from the boot start time
	eventTimestamp := tl.BootStart
	if eventTimestamp.IsZero() {
		eventTimestamp = time.Now()
	}
	timestamp := eventTimestamp.In(time.UTC).Format("2006-01-02T15:04:05.000000Z")

	// Build a human-readable message
	msg := "Windows logon duration analysis after reboot"
	if bootToDesktop, ok := durations["boot_to_desktop_ms"]; ok {
		msg = fmt.Sprintf("Windows boot to desktop took %d ms", bootToDesktop)
	}

	custom := map[string]interface{}{
		"durations":   durations,
		"subscribers": subscriberData,
	}

	eventData := map[string]interface{}{
		"data": map[string]interface{}{
			"type": "event",
			"attributes": map[string]interface{}{
				"host":           hostnameValue,
				"title":          "Logon duration",
				"category":       "alert",
				"integration_id": "system-notable-events",
				"system-notable-events": map[string]interface{}{
					"event_type": "Logon duration",
				},
				"attributes": map[string]interface{}{
					"status":   "info",
					"priority": "3",
					"custom":   custom,
				},
				"message":   msg,
				"timestamp": timestamp,
			},
		},
	}

	jsonData, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	log.Debugf("Submitting logon duration event for host %s", hostnameValue)

	m := message.NewMessage(jsonData, nil, "", time.Now().UnixNano())
	if err := forwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeEventManagement); err != nil {
		return fmt.Errorf("failed to send event to platform: %w", err)
	}

	log.Debugf("Successfully submitted logon duration event")
	return nil
}
