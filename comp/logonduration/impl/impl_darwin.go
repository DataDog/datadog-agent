// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

// Package logondurationimpl implements the logon duration component for macOS
package logondurationimpl

import (
	"context"
	"encoding/json"
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
	"github.com/DataDog/datadog-agent/pkg/logonduration"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/persistentcache"
	sysprobeclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires defines the dependencies for the logon duration component
type Requires struct {
	Lc             compdef.Lifecycle
	Config         configcomp.Component
	SysprobeConfig sysprobeconfig.Component
	Log            logcomp.Component
	EventPlatform  eventplatform.Component
	Hostname       hostname.Component
}

// sysProbeClient is an interface for system probe used for dependency injection and testing.
type sysProbeClient interface {
	GetLoginTimestamps(ctx context.Context) (logonduration.LoginTimestamps, error)
}

// sysProbeClientWrapper wraps the real sysprobeclient.CheckClient to implement sysProbeClient.
type sysProbeClientWrapper struct {
	socketPath string
}

// GetLoginTimestamps implements sysProbeClient.GetLoginTimestamps by delegating to the wrapped client.
func (w *sysProbeClientWrapper) GetLoginTimestamps(ctx context.Context) (logonduration.LoginTimestamps, error) {
	client := sysprobeclient.GetCheckClient(
		sysprobeclient.WithSocketPath(w.socketPath),
	)

	for {
		timestamps, err := sysprobeclient.GetCheck[logonduration.LoginTimestamps](client, sysconfig.LogonDurationModule)
		if err == nil {
			return timestamps, nil
		}

		if !errors.Is(err, sysprobeclient.ErrNotStartedYet) {
			return logonduration.LoginTimestamps{}, fmt.Errorf("failed to get login timestamps from system-probe: %w", err)
		}

		log.Debugf("Logon duration: system-probe not ready yet, retrying in 10s: %v", err)

		timer := time.NewTimer(10 * time.Second)
		select {
		case <-timer.C:
			continue
		case <-ctx.Done():
			timer.Stop()
			return logonduration.LoginTimestamps{}, ctx.Err()
		}
	}
}

type logonDurationComponent struct {
	config                 configcomp.Component
	sysprobeConfig         sysprobeconfig.Component
	hostname               hostname.Component
	eventPlatformForwarder eventplatform.Forwarder
	sysProbeClient         sysProbeClient
	wg                     sync.WaitGroup
	ctxCancel              context.CancelFunc
}

// NewComponent creates a new logon duration component for macOS
func NewComponent(reqs Requires) Provides {
	return newWithClient(reqs, &sysProbeClientWrapper{
		socketPath: reqs.SysprobeConfig.GetString("system_probe_config.sysprobe_socket"),
	})
}

// newWithClient creates a new logon duration component with a custom sysProbeClient for testing
func newWithClient(reqs Requires, client sysProbeClient) Provides {
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
		sysProbeClient:         client,
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
	loginTimestamps, err := c.sysProbeClient.GetLoginTimestamps(ctx)
	if err != nil {
		log.Warnf("Logon duration: failed to get login timestamps from system-probe: %v (took %.2fs)", err, time.Since(start).Seconds())
		return
	}
	log.Infof("Logon duration: got login timestamps from system-probe (took %.2fs)", time.Since(start).Seconds())

	// Build and submit the event
	if err := c.submitEvent(bootTime, loginTimestamps); err != nil {
		log.Errorf("Logon duration: failed to submit event: %v", err)
		return
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

func buildTimelineMilestones(bootTime time.Time, ts logonduration.LoginTimestamps) []Milestone {
	const tsFmt = "2006-01-02T15:04:05.000Z"

	milestones := []Milestone{
		{
			Name:      "Boot Start",
			OffsetS:   0,
			DurationS: ts.LoginWindowTime.Sub(bootTime).Seconds(),
			Timestamp: bootTime.UTC().Format(tsFmt),
		},
		{
			Name:      "Login Window Ready",
			OffsetS:   ts.LoginWindowTime.Sub(bootTime).Seconds(),
			DurationS: ts.LoginTime.Sub(ts.LoginWindowTime).Seconds(),
			Timestamp: ts.LoginWindowTime.UTC().Format(tsFmt),
		},
		{
			Name:      "User Login",
			OffsetS:   ts.LoginTime.Sub(bootTime).Seconds(),
			DurationS: ts.DesktopReadyTime.Sub(ts.LoginTime).Seconds(),
			Timestamp: ts.LoginTime.UTC().Format(tsFmt),
		},
		{
			Name:      "Desktop Ready",
			OffsetS:   ts.DesktopReadyTime.Sub(bootTime).Seconds(),
			DurationS: 0,
			Timestamp: ts.DesktopReadyTime.UTC().Format(tsFmt),
		},
	}

	return milestones
}

func buildCustomPayload(bootTime time.Time, ts logonduration.LoginTimestamps) map[string]interface{} {
	custom := make(map[string]interface{})

	custom["boot_timeline"] = buildTimelineMilestones(bootTime, ts)

	bootMs := ts.LoginWindowTime.Sub(bootTime).Milliseconds()
	logonMs := ts.DesktopReadyTime.Sub(ts.LoginTime).Milliseconds()

	custom["durations"] = map[string]interface{}{
		"Boot Duration (ms)":       bootMs,
		"Logon Duration (ms)":      logonMs,
		"Total Boot Duration (ms)": bootMs + logonMs,
	}

	custom["filevault_enabled"] = ts.FileVaultEnabled

	return custom
}

// submitEvent builds an Event Management v2 payload from the analysis result
// and sends it through the event platform forwarder.
func (c *logonDurationComponent) submitEvent(bootTime time.Time, ts logonduration.LoginTimestamps) error {
	// Build and submit the event
	hostnameValue := c.hostname.GetSafe(context.TODO())
	custom := buildCustomPayload(bootTime, ts)

	timestamp := bootTime.In(time.UTC).Format("2006-01-02T15:04:05.000000Z")

	msg := "macOS logon duration analysis after reboot"
	if durations, ok := custom["durations"].(map[string]interface{}); ok {
		if logonMs, ok := durations["Logon Duration (ms)"]; ok {
			msg = fmt.Sprintf("macOS logon took %d ms", logonMs)
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
					"status":   "ok",
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
