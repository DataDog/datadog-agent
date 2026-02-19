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

	"github.com/DataDog/gopsutil/host"

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
	config                 configcomp.Component
	hostname               hostname.Component
	eventPlatformForwarder eventplatform.Forwarder
}

// NewComponent creates a new logon duration component
func NewComponent(reqs Requires) Provides {
	if !reqs.Config.GetBool("logon_duration.enabled") {
		log.Debug("Logon duration component is disabled")

		// disable the autologger so it doesn't run on next boot
		exists, err := checkAutologgerExists(autologgerSessionName)
		if err != nil {
			log.Warnf("Logon duration: failed to check autologger: %v", err)
		} else if exists {
			err = toggleAutologger(autologgerSessionName, false)
			if err != nil {
				log.Warnf("Logon duration: failed to disable autologger: %v", err)
			} else {
				log.Info("Logon duration: disabled autologger for next boot")
			}
		}
		return Provides{
			Comp: &logonDurationComponent{},
		}
	}

	// verify autologger exists
	exists, err := checkAutologgerExists(autologgerSessionName)
	if err != nil {
		log.Warnf("Logon duration: failed to check autologger: %v", err)
		return Provides{
			Comp: &logonDurationComponent{},
		}
	} else if !exists {
		log.Warn("Logon duration: autologger not found; boot traces will not be collected until it is created")
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

	comp := &logonDurationComponent{
		config:                 reqs.Config,
		hostname:               reqs.Hostname,
		eventPlatformForwarder: forwarder,
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

	// Stop the active trace session
	if err := stopAutologger(autologgerSessionName); err != nil {
		log.Debugf("Logon duration: could not stop autologger session (may not be running): %v", err)
	}

	// Ensure autologger is enabled for the next boot
	if err := toggleAutologger(autologgerSessionName, true); err != nil {
		log.Warnf("Logon duration: failed to enable autologger for next boot: %v", err)
	}

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
	etlPath := getETLPath()
	if etlPath == "" {
		log.Warn("Logon duration: no ETL path found, skipping analysis")
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

// getLastBootTime returns the system's last boot time
func getLastBootTime() (string, error) {
	bootTime, err := host.BootTime()
	if err != nil {
		return "", fmt.Errorf("getting boot time: %w", err)
	}
	t := time.Unix(int64(bootTime), 0)
	return t.UTC().Format(time.RFC3339), nil
}

func getDurationMilliseconds(start, end time.Time) int64 {
	return end.Sub(start).Milliseconds()
}

// Milestone represents a single event in the boot/logon timeline.
type Milestone struct {
	Name      string  `json:"name"`
	OffsetS   float64 `json:"offset_s"`
	Timestamp string  `json:"timestamp"`
}

// buildTimelineMilestones returns an ordered slice of boot milestones.
// Only milestones with a non-zero timestamp are included.
func buildTimelineMilestones(tl BootTimeline) []Milestone {
	const tsFmt = "2006-01-02T15:04:05.000Z"
	boot := tl.BootStart

	candidates := []struct {
		name string
		ts   time.Time
	}{
		{"boot_start", tl.BootStart},
		{"smss_start", tl.SmssStart},
		{"user_session_smss", tl.UserSmssStart},
		{"winlogon_start", tl.WinlogonStart},
		{"winlogon_init", tl.WinlogonInit},
		{"services_ready", tl.ServicesReady},
		{"machine_gp_start", tl.MachineGPStart},
		{"machine_gp_complete", tl.MachineGPEnd},
		{"user_session_winlogon", tl.UserWinlogonStart},
		{"logon_start", tl.LogonStart},
		{"profile_load_start", tl.ProfileStart},
		{"shell_start", tl.ShellStart},
		{"userinit_start", tl.UserinitStart},
		{"shell_started", tl.ShellStarted},
		{"explorer_start", tl.ExplorerStart},
		{"desktop_ready", tl.DesktopReady},
		{"user_gp_start", tl.UserGPStart},
		{"user_gp_complete", tl.UserGPEnd},
	}

	var milestones []Milestone
	for _, c := range candidates {
		if c.ts.IsZero() {
			continue
		}
		milestones = append(milestones, Milestone{
			Name:      c.name,
			OffsetS:   c.ts.Sub(boot).Seconds(),
			Timestamp: c.ts.UTC().Format(tsFmt),
		})
	}
	return milestones
}

func buildCustomPayload(tl BootTimeline) map[string]interface{} {
	custom := make(map[string]interface{})

	custom["boot_timeline"] = buildTimelineMilestones(tl)

	durations := make(map[string]interface{})
	if !tl.LogonStart.IsZero() && !tl.DesktopReady.IsZero() {
		durations["total_logon_ms"] = getDurationMilliseconds(tl.LogonStart, tl.DesktopReady)
	}
	if !tl.ProfileStart.IsZero() && !tl.ProfileEnd.IsZero() {
		durations["profile_load_ms"] = getDurationMilliseconds(tl.ProfileStart, tl.ProfileEnd)
	}
	if !tl.MachineGPStart.IsZero() && !tl.MachineGPEnd.IsZero() {
		durations["machine_gp_ms"] = getDurationMilliseconds(tl.MachineGPStart, tl.MachineGPEnd)
	}
	if !tl.UserGPStart.IsZero() && !tl.UserGPEnd.IsZero() {
		durations["user_gp_ms"] = getDurationMilliseconds(tl.UserGPStart, tl.UserGPEnd)
	}
	if !tl.ShellStart.IsZero() && !tl.DesktopReady.IsZero() {
		durations["shell_to_desktop_ms"] = getDurationMilliseconds(tl.ShellStart, tl.DesktopReady)
	}
	if len(durations) > 0 {
		custom["durations"] = durations
	}

	return custom
}

// submitEvent builds an Event Management v2 payload from the analysis result
// and sends it through the event platform forwarder.
func (c *logonDurationComponent) submitEvent(result *AnalysisResult) error {
	hostnameValue := c.hostname.GetSafe(context.TODO())
	tl := result.Timeline

	custom := buildCustomPayload(tl)

	eventTimestamp := tl.BootStart
	if eventTimestamp.IsZero() {
		eventTimestamp = time.Now()
	}
	timestamp := eventTimestamp.In(time.UTC).Format("2006-01-02T15:04:05.000000Z")

	msg := "Windows logon duration analysis after reboot"
	if durations, ok := custom["durations"].(map[string]interface{}); ok {
		if totalMs, ok := durations["total_logon_ms"]; ok {
			msg = fmt.Sprintf("Windows logon took %d ms", totalMs)
		}
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

	log.Debugf("Logon duration event payload: %s", string(jsonData))
	log.Debugf("Submitting logon duration event for host %s", hostnameValue)

	m := message.NewMessage(jsonData, nil, "", time.Now().UnixNano())
	if err := c.eventPlatformForwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeEventManagement); err != nil {
		return fmt.Errorf("failed to send event to platform: %w", err)
	}

	log.Debugf("Successfully submitted logon duration event")
	return nil
}
