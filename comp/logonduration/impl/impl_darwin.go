// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

// Package logondurationimpl implements the logon duration component for macOS
package logondurationimpl

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/host"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	logondurationdef "github.com/DataDog/datadog-agent/comp/logonduration/def"
	"github.com/DataDog/datadog-agent/pkg/logonduration"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// persistentCacheKey stores the last boot time to detect reboots across agent restarts.
const persistentCacheKey = "logon_duration:last_boot_time"

// Requires defines the dependencies for the logon duration component
type Requires struct {
	Lc             compdef.Lifecycle
	Config         configcomp.Component
	SysprobeConfig sysprobeconfig.Component
	Log            logcomp.Component
	EventPlatform  eventplatform.Component
	Hostname       hostname.Component
}

// Provides defines what this component provides
type Provides struct {
	Comp logondurationdef.Component
}

type logonDurationComponent struct {
	config                 configcomp.Component
	sysprobeConfig         sysprobeconfig.Component
	hostname               hostname.Component
	eventPlatformForwarder eventplatform.Forwarder
	wg                     sync.WaitGroup
	ctxCancel              context.CancelFunc
}

// NewComponent creates a new logon duration component for macOS
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

	comp := &logonDurationComponent{
		config:                 reqs.Config,
		sysprobeConfig:         reqs.SysprobeConfig,
		hostname:               reqs.Hostname,
		eventPlatformForwarder: forwarder,
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

// run executes the logon duration analysis:
//  1. Detect if a reboot occurred since last run (via persistent cache).
//  2. If no reboot, update cache and return.
//  3. If reboot, collect timestamps from system-probe and submit event.
func (c *logonDurationComponent) run(ctx context.Context) {
	rebooted, currentBootTime, err := c.detectReboot()
	if err != nil {
		log.Warnf("Logon duration: failed to detect reboot: %v", err)
		return
	}

	if !rebooted {
		log.Debug("Logon duration: no reboot detected since last run, skipping")
		return
	}

	log.Info("Logon duration: reboot detected, collecting boot/logon duration data")

	// Get boot time using gopsutil (doesn't require root)
	bootTimeSec, err := host.BootTime()
	if err != nil {
		log.Warnf("Logon duration: failed to get boot time: %v", err)
		return
	}
	bootTime := time.Unix(int64(bootTimeSec), 0)

	// Get login timestamps from system-probe (requires root)
	// This includes login window time, login time, and desktop ready time
	start := time.Now()
	loginTimestamps, err := c.getLoginTimestampsFromSystemProbe(ctx)
	if err != nil {
		log.Warnf("Logon duration: failed to get login timestamps from system-probe: %v (took %.2fs)", err, time.Since(start).Seconds())
		// Continue anyway - we can still report partial data
	} else {
		log.Infof("Logon duration: got login timestamps from system-probe (took %.2fs)", time.Since(start).Seconds())
	}

	// Build and submit the event
	if err := c.submitEvent(bootTime, loginTimestamps); err != nil {
		log.Errorf("Logon duration: failed to submit event: %v", err)
	}

	// Update persistent cache
	if err := persistentcache.Write(persistentCacheKey, currentBootTime); err != nil {
		log.Warnf("Logon duration: failed to update persistent cache: %v", err)
	}

	log.Info("Logon duration: boot analysis complete")
}

// detectReboot checks whether the system has rebooted since the last agent run
func (c *logonDurationComponent) detectReboot() (bool, string, error) {
	bootTimeSec, err := host.BootTime()
	if err != nil {
		return false, "", fmt.Errorf("getting current boot time: %w", err)
	}

	currentBootTime := time.Unix(int64(bootTimeSec), 0).UTC().Format(time.RFC3339)

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

// getLoginTimestampsFromSystemProbe retrieves login timestamps from system-probe
func (c *logonDurationComponent) getLoginTimestampsFromSystemProbe(ctx context.Context) (*logonduration.LoginTimestamps, error) {
	client := sysprobeclient.GetCheckClient(
		sysprobeclient.WithSocketPath(c.sysprobeConfig.GetString("system_probe_config.sysprobe_socket")),
	)

	// Wait for system-probe to be ready with simple retry loop
	for {
		timestamps, err := sysprobeclient.GetCheck[logonduration.LoginTimestamps](client, sysconfig.LogonDurationModule)
		if err == nil {
			return &timestamps, nil
		}

		// Only retry if system-probe hasn't started yet.
		// This error is returned for the first 5min after the Agent startup (configurable with check_system_probe_startup_time).
		if !errors.Is(err, sysprobeclient.ErrNotStartedYet) {
			return nil, fmt.Errorf("failed to get login timestamps from system-probe: %w", err)
		}

		log.Debugf("Logon duration: system-probe not ready yet, retrying in 10s: %v", err)

		// Use a timer that can be cancelled by context
		timer := time.NewTimer(10 * time.Second)
		select {
		case <-timer.C:
			continue
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		}
	}
}

// Milestone represents a single event in the boot/logon timeline.
type Milestone struct {
	Name      string  `json:"name"`
	OffsetS   float64 `json:"offset_s"`
	Timestamp string  `json:"timestamp"`
}

func buildTimelineMilestones(bootTime time.Time, loginTimestamps *logonduration.LoginTimestamps) []Milestone {
	const tsFmt = "2006-01-02T15:04:05.000Z"
	var milestones []Milestone

	// Boot Start
	milestones = append(milestones, Milestone{
		Name:      "Boot Start",
		OffsetS:   0,
		Timestamp: bootTime.UTC().Format(tsFmt),
	})

	// Login Window Time (from system-probe)
	if loginTimestamps != nil && loginTimestamps.LoginWindowTime != nil {
		milestones = append(milestones, Milestone{
			Name:      "Login Window Ready",
			OffsetS:   loginTimestamps.LoginWindowTime.Sub(bootTime).Seconds(),
			Timestamp: loginTimestamps.LoginWindowTime.UTC().Format(tsFmt),
		})
	}

	// Login Time (from system-probe)
	if loginTimestamps != nil && loginTimestamps.LoginTime != nil {
		milestones = append(milestones, Milestone{
			Name:      "User Login",
			OffsetS:   loginTimestamps.LoginTime.Sub(bootTime).Seconds(),
			Timestamp: loginTimestamps.LoginTime.UTC().Format(tsFmt),
		})
	}

	// Desktop Ready (from system-probe - Dock checkin with launchservicesd)
	if loginTimestamps != nil && loginTimestamps.DesktopReadyTime != nil {
		milestones = append(milestones, Milestone{
			Name:      "Desktop Ready",
			OffsetS:   loginTimestamps.DesktopReadyTime.Sub(bootTime).Seconds(),
			Timestamp: loginTimestamps.DesktopReadyTime.UTC().Format(tsFmt),
		})
	}

	return milestones
}

func buildCustomPayload(bootTime time.Time, loginTimestamps *logonduration.LoginTimestamps) map[string]interface{} {
	custom := make(map[string]interface{})

	custom["boot_timeline"] = buildTimelineMilestones(bootTime, loginTimestamps)

	durations := make(map[string]interface{})

	// Boot Duration: bootTime -> loginWindowTime
	if loginTimestamps != nil && loginTimestamps.LoginWindowTime != nil {
		durations["Boot Duration (ms)"] = loginTimestamps.LoginWindowTime.Sub(bootTime).Milliseconds()
	}

	// Logon Duration: loginTime -> desktopReadyTime
	if loginTimestamps != nil && loginTimestamps.LoginTime != nil && loginTimestamps.DesktopReadyTime != nil {
		durations["Logon Duration (ms)"] = loginTimestamps.DesktopReadyTime.Sub(*loginTimestamps.LoginTime).Milliseconds()
	}

	// Total Boot Duration: bootTime -> desktopReadyTime
	if loginTimestamps != nil && loginTimestamps.DesktopReadyTime != nil {
		durations["Total Boot Duration (ms)"] = loginTimestamps.DesktopReadyTime.Sub(bootTime).Milliseconds()
	}

	if len(durations) > 0 {
		custom["durations"] = durations
	}

	// Add FileVault status
	if loginTimestamps != nil && loginTimestamps.FileVaultEnabled != nil {
		custom["filevault_enabled"] = *loginTimestamps.FileVaultEnabled
	}

	return custom
}

// submitEvent logs the boot/logon duration data in a readable format
func (c *logonDurationComponent) submitEvent(bootTime time.Time, loginTimestamps *logonduration.LoginTimestamps) error {
	log.Info("Logon duration: ========== Boot/Logon Duration Data ==========")

	// Log timestamps
	log.Infof("Logon duration: boot_time=%s", bootTime.Format(time.RFC3339))

	if loginTimestamps != nil {
		if loginTimestamps.LoginWindowTime != nil {
			log.Infof("Logon duration: login_window_time=%s", loginTimestamps.LoginWindowTime.Format(time.RFC3339))
		}
		if loginTimestamps.LoginTime != nil {
			log.Infof("Logon duration: login_time=%s", loginTimestamps.LoginTime.Format(time.RFC3339))
		}
		if loginTimestamps.DesktopReadyTime != nil {
			log.Infof("Logon duration: desktop_ready_time=%s", loginTimestamps.DesktopReadyTime.Format(time.RFC3339))
		}
		if loginTimestamps.FileVaultEnabled != nil {
			log.Infof("Logon duration: filevault_enabled=%v", *loginTimestamps.FileVaultEnabled)
		}
	}

	// Log durations in seconds
	log.Info("Logon duration: ---------- Durations ----------")

	if loginTimestamps != nil && loginTimestamps.LoginWindowTime != nil {
		bootDuration := loginTimestamps.LoginWindowTime.Sub(bootTime).Seconds()
		log.Infof("Logon duration: boot_duration=%.2fs (login_window - boot)", bootDuration)
	}

	if loginTimestamps != nil && loginTimestamps.LoginTime != nil && loginTimestamps.DesktopReadyTime != nil {
		logonDuration := loginTimestamps.DesktopReadyTime.Sub(*loginTimestamps.LoginTime).Seconds()
		log.Infof("Logon duration: logon_duration=%.2fs (desktop_ready - login)", logonDuration)
	}

	if loginTimestamps != nil && loginTimestamps.DesktopReadyTime != nil {
		totalDuration := loginTimestamps.DesktopReadyTime.Sub(bootTime).Seconds()
		log.Infof("Logon duration: total_duration=%.2fs (desktop_ready - boot)", totalDuration)
	}

	log.Info("Logon duration: ================================================")
	return nil
}
