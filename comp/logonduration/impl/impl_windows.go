// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package logondurationimpl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/host"

	demultiplexer "github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires defines the dependencies for the logon duration component
type Requires struct {
	Lc            compdef.Lifecycle
	Config        configcomp.Component
	Log           logcomp.Component
	EventPlatform eventplatform.Component
	Hostname      hostname.Component
	Demultiplexer demultiplexer.Component
}

type logonDurationComponent struct {
	config                 configcomp.Component
	hostname               hostname.Component
	eventPlatformForwarder eventplatform.Forwarder
	wg                     sync.WaitGroup
	ctxCancel              context.CancelFunc
	sender                 sender.Sender
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

	sender, err := reqs.Demultiplexer.GetDefaultSender()
	if err != nil {
		log.Error("Logon duration: failed to get default sender")
		return Provides{
			Comp: &logonDurationComponent{},
		}
	}

	comp := &logonDurationComponent{
		config:                 reqs.Config,
		hostname:               reqs.Hostname,
		eventPlatformForwarder: forwarder,
		sender:                 sender,
	}

	reqs.Lc.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			return comp.start()
		},
		OnStop: func(_ context.Context) error {
			return comp.stop()
		},
	})

	log.Debug("Logon duration component initialized")

	return Provides{
		Comp: comp,
	}
}

func (c *logonDurationComponent) start() error {
	ctx, cancel := context.WithCancel(context.Background())
	c.ctxCancel = cancel
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.run(ctx)
	}()
	return nil
}

func (c *logonDurationComponent) stop() error {
	if c.ctxCancel != nil {
		c.ctxCancel()
	}
	c.wg.Wait()
	return nil
}

// run executes the one-shot logon duration analysis:
//  1. Detect if a reboot occurred since last run (via persistent cache).
//  2. If no reboot, update cache and return.
//  3. If reboot, parse the ETL file, build a payload, and submit a notable event.
func (c *logonDurationComponent) run(ctx context.Context) {

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
		return
	}

	if !rebooted {
		log.Debug("Logon duration: no reboot detected since last run, skipping")
		return
	}

	log.Info("Logon duration: reboot detected, analyzing boot trace")
	etlPath, err := getETLPath()
	if err != nil {
		log.Warnf("Logon duration: failed to get ETL path: %v", err)
		return
	}
	result, err := analyzeETL(ctx, etlPath)
	if err != nil {
		log.Errorf("Logon duration: failed to analyze ETL file: %v", err)
		// Update cache even on parse failure to avoid retrying the same boot
		if cacheErr := persistentcache.Write(persistentCacheKey, currentBootTime); cacheErr != nil {
			log.Warnf("Logon duration: failed to update persistent cache: %v", cacheErr)
		}
		return
	}

	if err := c.submitEvent(result); err != nil {
		log.Errorf("Logon duration: failed to submit event: %v", err)
		return
	}

	if err := persistentcache.Write(persistentCacheKey, currentBootTime); err != nil {
		log.Warnf("Logon duration: failed to update persistent cache: %v", err)
	}

	c.submitMetrics(result)

	log.Info("Logon duration: boot analysis complete")
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

// durationBetween returns the duration between start and end.
// Returns 0 if either timestamp is unavailable (zero).
func durationBetween(start, end time.Time) time.Duration {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	return end.Sub(start)
}

// buildTimelineMilestones returns an ordered slice of boot milestones.
// Only milestones with a non-zero timestamp are included.
func buildTimelineMilestones(tl BootTimeline) []Milestone {
	const tsFmt = "2006-01-02T15:04:05.000Z"
	boot := tl.BootStart

	candidates := []struct {
		id       string
		name     string
		ts       time.Time
		duration time.Duration
	}{
		{"boot_start", "Boot Start", tl.BootStart, 0},
		{"smss_start", "SMSS Start", tl.SmssStart, 0},
		{"user_session_smss_start", "User Session SMSS Start", tl.UserSmssStart, 0},
		{"winlogon_start", "Winlogon Start", tl.WinlogonStart, 0},
		{"winlogon_init", "Winlogon Init", tl.WinlogonInit, durationBetween(tl.WinlogonInit, tl.WinlogonInitDone)},
		{"login_ui_start", "Login UI Start", tl.LoginUIStart, durationBetween(tl.LoginUIStart, tl.LoginUIDone)},
		{"computer_group_policy", "Computer Group Policy", tl.MachineGPStart, durationBetween(tl.MachineGPStart, tl.MachineGPEnd)},
		{"user_group_policy", "User Group Policy", tl.UserGPStart, durationBetween(tl.UserGPStart, tl.UserGPEnd)},
		{"user_session_winlogon_start", "User Session Winlogon Start", tl.UserWinlogonStart, 0},
		{"user_logon", "User Logon", tl.LogonStart, durationBetween(tl.LogonStart, tl.LogonStop)},
		{"profile_loaded", "Profile Loaded", tl.ProfileLoadStart, durationBetween(tl.ProfileLoadStart, tl.ProfileLoadEnd)},
		{"profile_created", "Profile Created", tl.ProfileCreationStart, durationBetween(tl.ProfileCreationStart, tl.ProfileCreationEnd)},
		{"execute_shell_commands", "Execute Shell Commands", tl.ExecuteShellCommandListStart, durationBetween(tl.ExecuteShellCommandListStart, tl.ExecuteShellCommandListEnd)},
		{"userinit_exe", "Userinit.exe", tl.UserinitStart, durationBetween(tl.UserinitStart, tl.ExplorerStart)},
		{"explorer_exe_start", "Explorer.exe Start", tl.ExplorerStart, 0},
		{"explorer_initializing", "Explorer Initializing", tl.ExplorerInitStart, durationBetween(tl.ExplorerInitStart, tl.ExplorerInitEnd)},
		{"desktop_created", "Desktop Created", tl.DesktopCreateStart, durationBetween(tl.DesktopCreateStart, tl.DesktopCreateEnd)},
		{"desktop_visible", "Desktop Visible", tl.DesktopVisibleStart, durationBetween(tl.DesktopVisibleStart, tl.DesktopVisibleEnd)},
		{"desktop_startup_apps", "Desktop Startup Apps", tl.DesktopStartupAppsStart, durationBetween(tl.DesktopStartupAppsStart, tl.DesktopStartupAppsEnd)},
		{"desktop_ready", "Desktop Ready", tl.DesktopReadyStart, durationBetween(tl.DesktopReadyStart, tl.DesktopReadyEnd)},
	}

	hasBootRef := !boot.IsZero()

	var milestones []Milestone
	for _, c := range candidates {
		if c.ts.IsZero() {
			continue
		}
		var offset float64
		if hasBootRef {
			offset = c.ts.Sub(boot).Seconds()
		}
		milestones = append(milestones, Milestone{
			Id:        c.id,
			Name:      c.name,
			OffsetS:   offset,
			Timestamp: c.ts.UTC().Format(tsFmt),
			DurationS: c.duration.Seconds(),
		})
	}
	return milestones
}

func buildCustomPayload(tl BootTimeline) map[string]interface{} {
	custom := make(map[string]interface{})

	milestones := buildTimelineMilestones(tl)
	custom["boot_timeline"] = milestones

	durations := make(map[string]interface{})

	var bootMs, logonMs int64
	var haveBoot, haveLogon bool

	if !tl.BootStart.IsZero() && !tl.LoginUIStart.IsZero() {
		bootMs = getDurationMilliseconds(tl.BootStart, tl.LoginUIStart)
		durations["boot_duration_ms"] = bootMs
		haveBoot = true
	}

	if !tl.LogonStart.IsZero() && !tl.DesktopVisibleStart.IsZero() {
		logonMs = getDurationMilliseconds(tl.LogonStart, tl.DesktopVisibleStart)
		durations["logon_duration_ms"] = logonMs
		haveLogon = true
	}

	// Total Boot Duration is the sum of Boot Duration and Logon Duration
	// This is to ensure that the time spent idling in the login UI is not included in the total boot duration.
	if haveBoot && haveLogon {
		durations["total_boot_duration_ms"] = bootMs + logonMs
	}

	for _, milestone := range milestones {
		if milestone.DurationS > 0 {
			durations[milestone.Id] = milestone.DurationS * 1000
		}
	}

	if len(durations) > 0 {
		custom["durations"] = durations
	}

	return custom
}

// submitEvent builds an Event Management v2 payload from the analysis result
// and sends it through the event platform forwarder.
func (c *logonDurationComponent) submitEvent(result *AnalysisResult) error {
	tl := result.Timeline

	custom := buildCustomPayload(tl)

	eventTimestamp := tl.BootStart
	if eventTimestamp.IsZero() {
		eventTimestamp = time.Now()
	}

	msg := "Windows logon duration analysis after reboot"
	if durations, ok := custom["durations"].(map[string]interface{}); ok {
		if totalMs, ok := durations["logon_duration_ms"]; ok {
			msg = fmt.Sprintf("Windows logon took %d ms", totalMs)
		}
	}

	return sendEvent(c.eventPlatformForwarder, eventInput{
		Hostname:  c.hostname.GetSafe(context.TODO()),
		Message:   msg,
		Timestamp: eventTimestamp,
		Custom:    custom,
	})
}

func (c *logonDurationComponent) submitMetrics(result *AnalysisResult) {
	if c.sender == nil {
		return
	}
	tl := result.Timeline
	hostname := c.hostname.GetSafe(context.TODO())

	// Total boot duration
	if !tl.BootStart.IsZero() && !tl.LoginUIStart.IsZero() &&
		!tl.LogonStart.IsZero() && !tl.DesktopVisibleStart.IsZero() {
		totalMs := float64(getDurationMilliseconds(tl.BootStart, tl.LoginUIStart) +
			getDurationMilliseconds(tl.LogonStart, tl.DesktopVisibleStart))
		c.sender.Distribution("eudm.boot_duration", totalMs, hostname, []string{"phase:total"})
	}

	// Per-phase durations
	phases := []struct {
		name       string
		start, end time.Time
	}{
		{"boot", tl.BootStart, tl.LoginUIStart},
		{"logon", tl.LogonStart, tl.DesktopVisibleStart},
		{"winlogon_init", tl.WinlogonInit, tl.WinlogonInitDone},
		{"login_ui", tl.LoginUIStart, tl.LoginUIDone},
		{"computer_group_policy", tl.MachineGPStart, tl.MachineGPEnd},
		{"user_group_policy", tl.UserGPStart, tl.UserGPEnd},
		{"user_logon", tl.LogonStart, tl.LogonStop},
		{"profile_load", tl.ProfileLoadStart, tl.ProfileLoadEnd},
		{"profile_create", tl.ProfileCreationStart, tl.ProfileCreationEnd},
		{"execute_shell_commands", tl.ExecuteShellCommandListStart, tl.ExecuteShellCommandListEnd},
		{"userinit", tl.UserinitStart, tl.ExplorerStart},
		{"explorer_initializing", tl.ExplorerInitStart, tl.ExplorerInitEnd},
		{"desktop_created", tl.DesktopCreateStart, tl.DesktopCreateEnd},
		{"desktop_visible", tl.DesktopVisibleStart, tl.DesktopVisibleEnd},
		{"desktop_startup_apps", tl.DesktopStartupAppsStart, tl.DesktopStartupAppsEnd},
		{"desktop_ready", tl.DesktopReadyStart, tl.DesktopReadyEnd},
	}

	for _, p := range phases {
		if p.start.IsZero() || p.end.IsZero() {
			continue
		}
		ms := float64(p.end.Sub(p.start).Milliseconds())
		c.sender.Distribution("eudm.boot_duration", ms, hostname, []string{fmt.Sprintf("phase:%s", p.name)})
	}

	c.sender.Commit()
}
