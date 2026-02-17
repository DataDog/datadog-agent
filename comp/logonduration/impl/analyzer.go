// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package logondurationimpl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tekert/goetw/etw"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Provider GUIDs for filtering events from the ETL file.
// MustParseGUID returns *GUID.
var (
	guidKernelProcess = etw.MustParseGUID("{22FB2CD6-0E7B-422B-A0C7-2FAD1FD0E716}")
	guidKernelGeneral = etw.MustParseGUID("{A68CA8B7-004F-D7B6-A698-07E2DE0F1F5D}")
	guidWinlogon      = etw.MustParseGUID("{DBE9B383-7CF3-4331-91CC-A3CB16A3B538}")
	guidUserProfile   = etw.MustParseGUID("{89B1E9F0-5AFF-44A6-9B44-0A07A7CE5845}")
	guidGroupPolicy   = etw.MustParseGUID("{AEA1B4FA-97D1-45F2-A64C-4D69FFFD92C9}")
	guidShellCore     = etw.MustParseGUID("{30336ED4-E327-447C-9DE0-51B652C86108}")
)

// BootTimeline holds all milestone timestamps collected from ETL events.
type BootTimeline struct {
	BootStart         time.Time // Kernel-General Event 12
	SmssStart         time.Time // Kernel-Process Event 1 (first smss.exe)
	WinlogonStart     time.Time // Kernel-Process Event 1 (first winlogon.exe, Session 1)
	UserSmssStart     time.Time // Kernel-Process Event 1 (smss.exe, Session 2+)
	UserWinlogonStart time.Time // Kernel-Process Event 1 (winlogon.exe, Session 2+)
	LogonStart        time.Time // Winlogon Event 5001
	SubscribersDone   time.Time // Winlogon Event 802
	ProfileStart      time.Time // User Profile Service Event 1001
	ProfileEnd        time.Time // User Profile Service Event 1002
	MachineGPStart    time.Time // GroupPolicy Event 4000
	MachineGPEnd      time.Time // GroupPolicy Event 8000
	UserGPStart       time.Time // GroupPolicy Event 4001
	UserGPEnd         time.Time // GroupPolicy Event 8001
	ShellStart        time.Time // Winlogon Event 9
	ShellStarted      time.Time // Winlogon Event 10
	UserinitStart     time.Time // Kernel-Process Event 1 (userinit.exe)
	ExplorerStart     time.Time // Kernel-Process Event 1 (explorer.exe)
	DesktopReady      time.Time // Shell-Core Event 62171
	BootToIdle        time.Time // Computed: first moment CPU ≥80% idle for 10 consecutive seconds (from PPM events)

	// Winlogon sub-events for detailed component timing
	WinlogonInit      time.Time // Winlogon Event 101
	WinlogonInitDone  time.Time // Winlogon Event 102
	ServicesWaitStart time.Time // Winlogon Event 107
	ServicesReady     time.Time // Winlogon Event 108
	SCMNotifyStart    time.Time // Winlogon Event 3
	SCMNotifyEnd      time.Time // Winlogon Event 4
	LogonScriptsStart time.Time // Winlogon Event 13
	LogonScriptsEnd   time.Time // Winlogon Event 14
}

// SubscriberInfo tracks an individual Winlogon subscriber's timing.
type SubscriberInfo struct {
	Name     string
	Start    time.Time
	End      time.Time
	Duration time.Duration
}

// eventHandlerFunc processes an accepted event for a specific provider.
type eventHandlerFunc func(e *etw.Event, id uint16, ts time.Time)

// collector accumulates events during ETL processing.
type collector struct {
	timeline       BootTimeline
	smssCount      int
	winlogonCount  int
	subscribers    []SubscriberInfo
	currentSubs    map[string]time.Time // subscriber name -> start time
	parseFunctions map[etw.GUID]eventHandlerFunc

	// Group Policy CSE tracking
	machineGPActivity string // ActivityID from event 4000
	userGPActivity    string // ActivityID from event 4001
}

// initParseFunctions populates the GUID → parse function dispatch map.
func (coll *collector) initParseFunctions() {
	coll.parseFunctions = map[etw.GUID]eventHandlerFunc{
		*guidKernelGeneral: coll.parseKernelGeneral,
		*guidKernelProcess: coll.parseKernelProcess,
		*guidWinlogon:      coll.parseWinlogon,
		*guidUserProfile:   coll.parseUserProfile,
		*guidGroupPolicy:   coll.parseGroupPolicy,
		*guidShellCore:     coll.parseShellCore,
	}
}

// AnalysisResult holds the structured output from ETL analysis.
type AnalysisResult struct {
	Timeline    BootTimeline
	Subscribers []SubscriberInfo
}

// analyzeETL opens an ETL file, processes events, and returns a structured
// boot timeline analysis. The caller is responsible for submitting the results.
func analyzeETL(etlPath string) (*AnalysisResult, error) {
	absPath, err := filepath.Abs(etlPath)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("ETL file not found: %s", absPath)
	}

	log.Debugf("Analyzing ETL file: %s", absPath)

	coll := &collector{
		currentSubs: make(map[string]time.Time),
	}
	coll.initParseFunctions()

	var totalEvents atomic.Int64

	c := etw.NewConsumer(context.Background())
	defer c.Stop()

	c.FromTraceNames(absPath)

	// EventRecordCallback: fast filter on provider GUID + Event ID.
	// Returns true only for events we care about; the rest are skipped entirely
	// (no property parsing, no Event creation). This is critical for performance
	// since we only need ~100 events out of potentially hundreds of thousands.
	//
	// IMPORTANT: This runs in the processing goroutine. Only the atomic counter
	// is written here — all data collection happens in ProcessEvents (main goroutine).
	c.EventRecordCallback = func(er *etw.EventRecord) bool {
		totalEvents.Add(1)
		id := er.EventHeader.EventDescriptor.Id

		// Kernel-General Event 12: Boot Start
		if er.EventHeader.ProviderId.Equals(guidKernelGeneral) && id == 12 {
			return true
		}

		// Kernel-Process Event 1: Process Start (needs ImageFileName)
		if er.EventHeader.ProviderId.Equals(guidKernelProcess) && id == 1 {
			return true
		}

		// Winlogon events we care about
		if er.EventHeader.ProviderId.Equals(guidWinlogon) {
			switch id {
			case 3, 4, 9, 10, 13, 14, 101, 102, 107, 108, 802, 805, 806, 5001:
				return true
			}
			return false
		}

		// User Profile Service events
		if er.EventHeader.ProviderId.Equals(guidUserProfile) {
			switch id {
			case 1001, 1002:
				return true
			}
			return false
		}

		// GroupPolicy events
		if er.EventHeader.ProviderId.Equals(guidGroupPolicy) {
			switch id {
			case 4000, 4001, 8000, 8001: // GP session start/end
				return true
			}
			return false
		}

		// Shell-Core Event 62171: Desktop Ready
		if er.EventHeader.ProviderId.Equals(guidShellCore) && id == 62171 {
			return true
		}

		return false // Skip all other events
	}

	// Don't set a custom EventCallback — let the default send events to the
	// internal channel. ProcessEvents below drains the channel and handles
	// all data collection in a single goroutine (no race conditions).

	startTime := time.Now()

	// Start() is non-blocking for ETL files: it opens the trace and begins
	// feeding events through the pipeline in background goroutines.
	if err := c.Start(); err != nil {
		return nil, fmt.Errorf("opening ETL file: %w", err)
	}

	// When the trace processing goroutine finishes (file fully read), close the
	// events channel so ProcessEvents can return. Without this, ProcessEvents
	// blocks forever because the channel is only closed during Stop(), and Stop()
	// is deferred after ProcessEvents — creating a deadlock.
	go func() {
		c.Wait()               // block until all trace goroutines finish
		c.CloseEventsChannel() // unblock ProcessEvents
	}()

	// ProcessEvents blocks until the events channel is closed.
	// All data collection happens here in the main goroutine.
	c.ProcessEvents(func(e *etw.Event) {
		processEvent(coll, e)
	})

	elapsed := time.Since(startTime)
	log.Debugf("Processed %d events in %v", totalEvents.Load(), elapsed.Round(time.Millisecond))

	return &AnalysisResult{
		Timeline:    coll.timeline,
		Subscribers: coll.subscribers,
	}, nil
}

// processEvent dispatches a filtered event to the appropriate provider handler.
func processEvent(coll *collector, e *etw.Event) {
	parseFunc, ok := coll.parseFunctions[e.System.Provider.Guid]
	if !ok {
		return
	}
	parseFunc(e, e.System.EventID, e.System.TimeCreated.SystemTime)
}

// parseKernelGeneral processes Kernel-General events (Event 12: Boot Start).
func (coll *collector) parseKernelGeneral(_ *etw.Event, _ uint16, ts time.Time) {
	if coll.timeline.BootStart.IsZero() {
		coll.timeline.BootStart = ts
	}
}

// parseKernelProcess processes Kernel-Process events (Event 1: Process Start).
// Tracks key process milestones: smss.exe, winlogon.exe, userinit.exe, explorer.exe.
func (coll *collector) parseKernelProcess(e *etw.Event, _ uint16, ts time.Time) {
	imageName := getEventPropString(e, "ImageFileName")
	if imageName == "" {
		imageName = getEventPropString(e, "ImageName")
	}
	if imageName == "" {
		imageName = getEventPropString(e, "FileName")
	}

	imageName = strings.ToLower(filepath.Base(imageName))

	switch {
	case strings.Contains(imageName, "smss.exe"):
		coll.smssCount++
		if coll.smssCount == 1 {
			coll.timeline.SmssStart = ts
		} else if coll.timeline.UserSmssStart.IsZero() && coll.smssCount >= 3 {
			coll.timeline.UserSmssStart = ts
		}
	case strings.Contains(imageName, "winlogon.exe"):
		coll.winlogonCount++
		if coll.winlogonCount == 1 {
			coll.timeline.WinlogonStart = ts
		} else if coll.timeline.UserWinlogonStart.IsZero() && coll.winlogonCount >= 2 {
			coll.timeline.UserWinlogonStart = ts
		}
	case strings.Contains(imageName, "userinit.exe"):
		if coll.timeline.UserinitStart.IsZero() {
			coll.timeline.UserinitStart = ts
		}
	case strings.Contains(imageName, "explorer.exe"):
		if coll.timeline.ExplorerStart.IsZero() {
			coll.timeline.ExplorerStart = ts
		}
	}
}

// parseWinlogon processes Winlogon events for logon lifecycle tracking.
func (coll *collector) parseWinlogon(e *etw.Event, id uint16, ts time.Time) {
	switch id {
	case 101:
		if coll.timeline.WinlogonInit.IsZero() {
			coll.timeline.WinlogonInit = ts
		}
	case 102:
		if coll.timeline.WinlogonInitDone.IsZero() {
			coll.timeline.WinlogonInitDone = ts
		}
	case 107:
		if coll.timeline.ServicesWaitStart.IsZero() {
			coll.timeline.ServicesWaitStart = ts
		}
	case 108:
		if coll.timeline.ServicesReady.IsZero() {
			coll.timeline.ServicesReady = ts
		}
	case 3:
		coll.timeline.SCMNotifyStart = ts
	case 4:
		coll.timeline.SCMNotifyEnd = ts
	case 9:
		if coll.timeline.ShellStart.IsZero() {
			coll.timeline.ShellStart = ts
		}
	case 10:
		if coll.timeline.ShellStarted.IsZero() {
			coll.timeline.ShellStarted = ts
		}
	case 13:
		coll.timeline.LogonScriptsStart = ts
	case 14:
		coll.timeline.LogonScriptsEnd = ts
	case 5001:
		if coll.timeline.LogonStart.IsZero() {
			coll.timeline.LogonStart = ts
		}
	case 802:
		coll.timeline.SubscribersDone = ts
	case 805:
		subName := findSubscriberName(e)
		if subName != "" {
			coll.currentSubs[subName] = ts
		}
	case 806:
		subName := findSubscriberName(e)
		if subName != "" {
			if start, ok := coll.currentSubs[subName]; ok {
				dur := ts.Sub(start)
				coll.subscribers = append(coll.subscribers, SubscriberInfo{
					Name:     subName,
					Start:    start,
					End:      ts,
					Duration: dur,
				})
				delete(coll.currentSubs, subName)
			}
		}
	}
}

// parseUserProfile processes User Profile Service events (1001: start, 1002: end).
func (coll *collector) parseUserProfile(_ *etw.Event, id uint16, ts time.Time) {
	switch id {
	case 1001:
		if coll.timeline.ProfileStart.IsZero() {
			coll.timeline.ProfileStart = ts
		}
	case 1002:
		coll.timeline.ProfileEnd = ts
	}
}

// parseGroupPolicy processes Group Policy events (4000/4001: start, 8000/8001: end).
func (coll *collector) parseGroupPolicy(e *etw.Event, id uint16, ts time.Time) {
	activityID := e.System.Correlation.ActivityID

	switch id {
	case 4000:
		if coll.timeline.MachineGPStart.IsZero() {
			coll.timeline.MachineGPStart = ts
		}
		if activityID != "" {
			coll.machineGPActivity = activityID
		}
	case 8000:
		if coll.timeline.MachineGPEnd.IsZero() {
			coll.timeline.MachineGPEnd = ts
		}
	case 4001:
		if coll.timeline.UserGPStart.IsZero() {
			coll.timeline.UserGPStart = ts
		}
		if activityID != "" {
			coll.userGPActivity = activityID
		}
	case 8001:
		coll.timeline.UserGPEnd = ts
	}
}

// parseShellCore processes Shell-Core events (Event 62171: Desktop Ready).
func (coll *collector) parseShellCore(_ *etw.Event, _ uint16, ts time.Time) {
	if coll.timeline.DesktopReady.IsZero() {
		coll.timeline.DesktopReady = ts
	}
}

// getEventPropString finds a named property in the Event and returns its string value.
func getEventPropString(e *etw.Event, name string) string {
	// Check EventData first
	for _, prop := range e.EventData {
		if prop.Name == name {
			return fmt.Sprintf("%v", prop.Value)
		}
	}
	// Then UserData
	for _, prop := range e.UserData {
		if prop.Name == name {
			return fmt.Sprintf("%v", prop.Value)
		}
	}
	return ""
}

// getEventPropUint64 finds a named property in the Event and returns its uint64 value.
// Returns 0 if the property is not found or cannot be parsed.
func getEventPropUint64(e *etw.Event, name string) uint64 {
	s := getEventPropString(e, name)
	if s == "" {
		return 0
	}
	s = strings.TrimSpace(s)
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// findSubscriberName extracts the subscriber name from a Winlogon 805/806 event.
func findSubscriberName(e *etw.Event) string {
	// Try known property names for subscriber identification
	for _, key := range []string{"SubscriberName", "subscriberName", "Name", "name", "Description"} {
		if v := getEventPropString(e, key); v != "" {
			return v
		}
	}
	// Fallback: search all property values for known subscriber names
	knownSubs := []string{"SessionEnv", "Profiles", "GPClient", "TermSrv", "Sens", "TrustedInstaller"}
	allProps := append(e.EventData, e.UserData...)
	for _, prop := range allProps {
		val := fmt.Sprintf("%v", prop.Value)
		for _, sub := range knownSubs {
			if strings.Contains(val, sub) {
				return sub
			}
		}
	}
	return ""
}
