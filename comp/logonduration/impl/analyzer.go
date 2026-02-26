// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package logondurationimpl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	BootStart                    time.Time // Kernel-General Event 12
	SmssStart                    time.Time // Kernel-Process Event 1 (first smss.exe)
	WinlogonStart                time.Time // Kernel-Process Event 1 (first winlogon.exe, Session 1)
	UserSmssStart                time.Time // Kernel-Process Event 1 (smss.exe, Session 2+)
	UserWinlogonStart            time.Time // Kernel-Process Event 1 (winlogon.exe, Session 2+)
	LogonStart                   time.Time // Winlogon Event 5001
	LogonStop                    time.Time // Winlogon Event 5002
	SubscribersDone              time.Time // Winlogon Event 802
	ProfileLoadStart             time.Time // User Profile Service Event 1
	ProfileLoadEnd               time.Time // User Profile Service Event 2
	ProfileCreationStart         time.Time // User Profile Service Event 1001
	ProfileCreationEnd           time.Time // User Profile Service Event 1002
	MachineGPStart               time.Time // GroupPolicy Event 4000
	MachineGPEnd                 time.Time // GroupPolicy Event 8000
	UserGPStart                  time.Time // GroupPolicy Event 4001
	UserGPEnd                    time.Time // GroupPolicy Event 8001
	ExecuteShellCommandListStart time.Time // Winlogon Event 9
	ExecuteShellCommandListEnd   time.Time // Winlogon Event 10
	UserinitStart                time.Time // Kernel-Process Event 1 (userinit.exe)
	ExplorerStart                time.Time // Kernel-Process Event 1 (explorer.exe)
	DesktopReady                 time.Time // Shell-Core Event 62171

	// Winlogon sub-events for detailed component timing
	WinlogonInit     time.Time // Winlogon Event 101
	WinlogonInitDone time.Time // Winlogon Event 102
	LSMStart         time.Time // Winlogon Event 107
	LSMReady         time.Time // Winlogon Event 108
	ThemesLogonStart time.Time // Winlogon Event 11
	ThemesLogonEnd   time.Time // Winlogon Event 13
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
	currentSubs    map[string]time.Time
	parseFunctions map[etw.GUID]eventHandlerFunc
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
// boot timeline analysis.
func analyzeETL(ctx context.Context, etlPath string) (*AnalysisResult, error) {
	absPath, err := filepath.Abs(etlPath)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}
	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("ETL file not found: %s", absPath)
		}
		return nil, fmt.Errorf("error accessing ETL file: %w", err)
	}

	log.Debugf("Analyzing ETL file: %s", absPath)

	coll := &collector{
		currentSubs: make(map[string]time.Time),
	}
	coll.initParseFunctions()

	var totalEvents atomic.Int64

	c := etw.NewConsumer(ctx)
	defer func() {
		if err := c.Stop(); err != nil {
			log.Errorf("error stopping ETL consumer: %v", err)
		}
	}()

	c.FromTraceNames(absPath)

	// EventRecordCallback: fast filter on provider GUID + Event ID.
	// Returns true only for events we care about; the rest are skipped entirely
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
			case 9, 10, 11, 13, 101, 102, 107, 108, 802, 805, 806, 5001, 5002:
				return true
			}
			return false
		}

		// User Profile Service events
		if er.EventHeader.ProviderId.Equals(guidUserProfile) {
			switch id {
			case 1, 2, 1001, 1002:
				return true
			}
			return false
		}

		// GroupPolicy events
		if er.EventHeader.ProviderId.Equals(guidGroupPolicy) {
			switch id {
			case 4000, 4001, 8000, 8001:
				return true
			}
			return false
		}

		// Shell-Core Event 62171: Desktop Ready
		if er.EventHeader.ProviderId.Equals(guidShellCore) && id == 62171 {
			return true
		}

		return false
	}

	startTime := time.Now()

	log.Debugf("Starting ETL consumer")
	if err := c.Start(); err != nil {
		return nil, fmt.Errorf("starting ETL consumer: %w", err)
	}

	// When the trace processing goroutine finishes (file fully read), close the
	// events channel so ProcessEvents can return. Without this, ProcessEvents
	// blocks forever because the channel is only closed during Stop(), and Stop()
	// is deferred after ProcessEvents — creating a deadlock.
	go func() {
		c.Wait()               // block until all trace goroutines finish
		c.CloseEventsChannel() // unblock ProcessEvents
	}()

	log.Debugf("Processing ETL events")
	err = c.ProcessEvents(func(e *etw.Event) {
		processEvent(coll, e)
	})
	if err != nil {
		return nil, fmt.Errorf("processing ETL events: %w", err)
	}

	elapsed := time.Since(startTime)
	log.Debugf("Processed %d events in %v", totalEvents.Load(), elapsed.Round(time.Millisecond))

	if coll.timeline.BootStart.IsZero() {
		return nil, errors.New("ETL file contained no boot start event (Kernel-General 12); timeline would be invalid")
	}

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
		if coll.timeline.LSMStart.IsZero() {
			coll.timeline.LSMStart = ts
		}
	case 108:
		if coll.timeline.LSMReady.IsZero() {
			coll.timeline.LSMReady = ts
		}
	case 9:
		if coll.timeline.ExecuteShellCommandListStart.IsZero() {
			coll.timeline.ExecuteShellCommandListStart = ts
		}
	case 10:
		if coll.timeline.ExecuteShellCommandListEnd.IsZero() {
			coll.timeline.ExecuteShellCommandListEnd = ts
		}
	case 11:
		coll.timeline.ThemesLogonStart = ts
	case 13:
		coll.timeline.ThemesLogonEnd = ts
	case 5001:
		if coll.timeline.LogonStart.IsZero() {
			coll.timeline.LogonStart = ts
		}
	case 5002:
		if coll.timeline.LogonStop.IsZero() {
			coll.timeline.LogonStop = ts
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
	case 1:
		if coll.timeline.ProfileLoadStart.IsZero() {
			coll.timeline.ProfileLoadStart = ts
		}
	case 2:
		if coll.timeline.ProfileLoadEnd.IsZero() {
			coll.timeline.ProfileLoadEnd = ts
		}
	case 1001:
		if coll.timeline.ProfileCreationStart.IsZero() {
			coll.timeline.ProfileCreationStart = ts
		}
	case 1002:
		if coll.timeline.ProfileCreationEnd.IsZero() {
			coll.timeline.ProfileCreationEnd = ts
		}
	}
}

// parseGroupPolicy processes Group Policy events (4000/4001: start, 8000/8001: end).
func (coll *collector) parseGroupPolicy(_ *etw.Event, id uint16, ts time.Time) {
	switch id {
	case 4000:
		if coll.timeline.MachineGPStart.IsZero() {
			coll.timeline.MachineGPStart = ts
		}
	case 8000:
		if coll.timeline.MachineGPEnd.IsZero() {
			coll.timeline.MachineGPEnd = ts
		}
	case 4001:
		if coll.timeline.UserGPStart.IsZero() {
			coll.timeline.UserGPStart = ts
		}
	case 8001:
		if coll.timeline.UserGPEnd.IsZero() {
			coll.timeline.UserGPEnd = ts
		}
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
	for _, prop := range e.EventData {
		if prop.Name == name {
			return fmt.Sprintf("%v", prop.Value)
		}
	}
	for _, prop := range e.UserData {
		if prop.Name == name {
			return fmt.Sprintf("%v", prop.Value)
		}
	}
	return ""
}

// findSubscriberName extracts the subscriber name from a Winlogon 805/806 event.
func findSubscriberName(e *etw.Event) string {
	for _, key := range []string{"SubscriberName", "subscriberName", "Name", "name", "Description"} {
		if v := getEventPropString(e, key); v != "" {
			return v
		}
	}
	knownSubs := []string{"SessionEnv", "Profiles", "GPClient", "TermSrv", "Sens", "TrustedInstaller"}
	allProps := make([]etw.EventProperty, 0, len(e.EventData)+len(e.UserData))
	allProps = append(allProps, e.EventData...)
	allProps = append(allProps, e.UserData...)
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
