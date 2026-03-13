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

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/etw"
)

// Provider GUIDs for filtering events from the ETL file.
var (
	guidKernelProcess = etw.MustParseGUID("{22FB2CD6-0E7B-422B-A0C7-2FAD1FD0E716}")
	guidKernelGeneral = etw.MustParseGUID("{A68CA8B7-004F-D7B6-A698-07E2DE0F1F5D}")
	guidWinlogon      = etw.MustParseGUID("{DBE9B383-7CF3-4331-91CC-A3CB16A3B538}")
	guidUserProfile   = etw.MustParseGUID("{89B1E9F0-5AFF-44A6-9B44-0A07A7CE5845}")
	guidGroupPolicy   = etw.MustParseGUID("{AEA1B4FA-97D1-45F2-A64C-4D69FFFD92C9}")
	guidShellCore     = etw.MustParseGUID("{30336ED4-E327-447C-9DE0-51B652C86108}")
)

// Named event IDs for each ETW provider.
const (
	// Kernel-General
	evtBootStart uint16 = 12

	// Kernel-Process
	evtProcessStart uint16 = 1

	// Winlogon
	evtWinlogonShellCmdStart uint16 = 9
	evtWinlogonShellCmdEnd   uint16 = 10
	evtWinlogonInit          uint16 = 101
	evtWinlogonInitDone      uint16 = 102
	evtLoginUIStart          uint16 = 103
	evtLoginUIDone           uint16 = 104
	evtLogonStart            uint16 = 5001
	evtLogonStop             uint16 = 5002

	// User Profile Service
	evtProfileLoadStart     uint16 = 1
	evtProfileLoadEnd       uint16 = 2
	evtProfileCreationStart uint16 = 1001
	evtProfileCreationEnd   uint16 = 1002

	// Group Policy
	evtMachineGPStart uint16 = 4000
	evtMachineGPEnd   uint16 = 8000
	evtUserGPStart    uint16 = 4001
	evtUserGPEnd      uint16 = 8001

	// Shell-Core
	evtExplorerInitStart  uint16 = 9601
	evtExplorerInitEnd    uint16 = 9602
	evtDesktopCreateStart uint16 = 9611
	evtDesktopCreateEnd   uint16 = 9612
	evtExplorerStepStart  uint16 = 9648
	evtExplorerStepEnd    uint16 = 9649
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
	ExplorerInitStart            time.Time // Shell-Core Event 9601 (Explorer_InitializingExplorerStart)
	ExplorerInitEnd              time.Time // Shell-Core Event 9602 (Explorer_InitializingExplorerStop)
	DesktopCreateStart           time.Time // Shell-Core Event 9611 (Explorer_CreateDesktopStart)
	DesktopCreateEnd             time.Time // Shell-Core Event 9612 (Explorer_CreateDesktopStop)
	DesktopVisibleStart          time.Time // Shell-Core Event 9648 (waitfordesktopvisuals step)
	DesktopVisibleEnd            time.Time // Shell-Core Event 9649 (waitfordesktopvisuals step)
	DesktopReadyStart            time.Time // Shell-Core Event 9648 (finalize step)
	DesktopReadyEnd              time.Time // Shell-Core Event 9649 (finalize step)

	// Winlogon sub-events for detailed component timing
	WinlogonInit     time.Time // Winlogon Event 101
	WinlogonInitDone time.Time // Winlogon Event 102
	LoginUIStart     time.Time // Winlogon Event 103
	LoginUIDone      time.Time // Winlogon Event 104

	// Shell-Core sub-events for detailed component timing
	DesktopStartupAppsStart time.Time // Shell-Core Event 9648 (desktopstartupapps step)
	DesktopStartupAppsEnd   time.Time // Shell-Core Event 9649 (desktopstartupapps step)
}

// eventWithProperties is satisfied by events that provide property lookup.
type eventWithProperties interface {
	GetPropertyString(name string) string
}

// processableEvent is an event that can be dispatched to parsers.
type processableEvent interface {
	eventWithProperties
	GetProviderID() windows.GUID
	GetEventID() uint16
	GetTimestamp() time.Time
}

// eventParser processes filtered events for a single ETW provider.
type eventParser interface {
	Parse(e eventWithProperties, id uint16, ts time.Time)
}

// providerConfig ties together the set of accepted event IDs and
// the parser for a given ETW provider.
type providerConfig struct {
	acceptedIDs map[uint16]struct{}
	parser      eventParser
}

// collector accumulates events during ETL processing.
type collector struct {
	timeline  BootTimeline
	providers map[windows.GUID]providerConfig
}

// buildProviders wires each provider's accepted event IDs together with
// its parser, creating a single source of truth for both filtering and
// dispatching.
func buildProviders(timeline *BootTimeline) map[windows.GUID]providerConfig {
	return map[windows.GUID]providerConfig{
		guidKernelGeneral: {
			acceptedIDs: map[uint16]struct{}{evtBootStart: {}},
			parser:      &kernelGeneralParser{timeline: timeline},
		},
		guidKernelProcess: {
			acceptedIDs: map[uint16]struct{}{evtProcessStart: {}},
			parser:      &kernelProcessParser{timeline: timeline},
		},
		guidWinlogon: {
			acceptedIDs: map[uint16]struct{}{
				evtWinlogonShellCmdStart: {}, evtWinlogonShellCmdEnd: {},
				evtWinlogonInit: {}, evtWinlogonInitDone: {},
				evtLoginUIStart: {}, evtLoginUIDone: {},
				evtLogonStart: {}, evtLogonStop: {},
			},
			parser: &winlogonParser{timeline: timeline},
		},
		guidUserProfile: {
			acceptedIDs: map[uint16]struct{}{
				evtProfileLoadStart: {}, evtProfileLoadEnd: {},
				evtProfileCreationStart: {}, evtProfileCreationEnd: {},
			},
			parser: &userProfileParser{timeline: timeline},
		},
		guidGroupPolicy: {
			acceptedIDs: map[uint16]struct{}{
				evtMachineGPStart: {}, evtMachineGPEnd: {},
				evtUserGPStart: {}, evtUserGPEnd: {},
			},
			parser: &groupPolicyParser{timeline: timeline},
		},
		guidShellCore: {
			acceptedIDs: map[uint16]struct{}{
				evtExplorerInitStart: {}, evtExplorerInitEnd: {},
				evtDesktopCreateStart: {}, evtDesktopCreateEnd: {},
				evtExplorerStepStart: {}, evtExplorerStepEnd: {},
			},
			parser: &shellCoreParser{timeline: timeline},
		},
	}
}

// AnalysisResult holds the structured output from ETL analysis.
type AnalysisResult struct {
	Timeline BootTimeline
}

// analyzeETL opens an ETL file, processes events, and returns a structured
// boot timeline analysis.
func analyzeETL(_ context.Context, etlPath string) (*AnalysisResult, error) {
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

	coll := &collector{}
	coll.providers = buildProviders(&coll.timeline)

	var totalEvents atomic.Int64

	filter := func(providerID windows.GUID, eventID uint16) bool {
		totalEvents.Add(1)
		cfg, ok := coll.providers[providerID]
		if !ok {
			return false
		}
		_, ok = cfg.acceptedIDs[eventID]
		return ok
	}

	startTime := time.Now()

	log.Debugf("Processing ETL events")
	err = etw.ProcessETLFile(absPath, func(e *etw.Event) {
		processEvent(coll, e)
	}, etw.WithEventRecordFilter(filter))
	if err != nil {
		return nil, fmt.Errorf("processing ETL file: %w", err)
	}

	elapsed := time.Since(startTime)
	log.Debugf("Processed %d events in %v", totalEvents.Load(), elapsed.Round(time.Millisecond))

	if coll.timeline.BootStart.IsZero() {
		return nil, errors.New("ETL file contained no boot start event (Kernel-General 12); timeline would be invalid")
	}

	return &AnalysisResult{
		Timeline: coll.timeline,
	}, nil
}

// directPropertyLookup is optionally implemented by events that support
// looking up a single property by name (via TdhGetProperty), bypassing
// sequential parsing that can fail on schema-mismatched properties.
type directPropertyLookup interface {
	GetPropertyByName(name string) (string, error)
}

// processEvent dispatches a filtered event to the appropriate provider parser.
func processEvent(coll *collector, e processableEvent) {
	cfg, ok := coll.providers[e.GetProviderID()]
	if !ok {
		return
	}
	cfg.parser.Parse(e, e.GetEventID(), e.GetTimestamp())
}

// --- Per-provider parser structs ---

// kernelGeneralParser processes Kernel-General events (Event 12: Boot Start).
type kernelGeneralParser struct {
	timeline *BootTimeline
}

func (p *kernelGeneralParser) Parse(_ eventWithProperties, _ uint16, ts time.Time) {
	if p.timeline.BootStart.IsZero() {
		p.timeline.BootStart = ts
	}
}

// kernelProcessParser processes Kernel-Process events (Event 1: Process Start).
// Tracks key process milestones: smss.exe, winlogon.exe, userinit.exe, explorer.exe.
type kernelProcessParser struct {
	timeline      *BootTimeline
	smssCount     int
	winlogonCount int
}

func (p *kernelProcessParser) Parse(e eventWithProperties, _ uint16, ts time.Time) {
	var imageName string
	if dl, ok := e.(directPropertyLookup); ok {
		val, err := dl.GetPropertyByName("ImageName")
		if err != nil {
			log.Debugf("GetPropertyByName(ImageName) failed: %v", err)
		}
		imageName = val
	} else {
		imageName = getEventPropString(e, "ImageName")
	}
	imageName = strings.ToLower(filepath.Base(imageName))
	log.Debugf("Parsing kernel process event: imageName: %s", imageName)

	switch {
	case strings.Contains(imageName, "smss.exe"):
		p.smssCount++
		if p.smssCount == 1 {
			p.timeline.SmssStart = ts
		} else if p.timeline.UserSmssStart.IsZero() && p.smssCount >= 3 {
			p.timeline.UserSmssStart = ts
		}
	case strings.Contains(imageName, "winlogon.exe"):
		p.winlogonCount++
		if p.winlogonCount == 1 {
			p.timeline.WinlogonStart = ts
		} else if p.timeline.UserWinlogonStart.IsZero() && p.winlogonCount >= 2 {
			p.timeline.UserWinlogonStart = ts
		}
	case strings.Contains(imageName, "userinit.exe"):
		if p.timeline.UserinitStart.IsZero() {
			p.timeline.UserinitStart = ts
		}
	case strings.Contains(imageName, "explorer.exe"):
		if p.timeline.ExplorerStart.IsZero() {
			p.timeline.ExplorerStart = ts
		}
	}
}

// winlogonParser processes Winlogon events for logon lifecycle tracking.
type winlogonParser struct {
	timeline *BootTimeline
}

func (p *winlogonParser) Parse(_ eventWithProperties, id uint16, ts time.Time) {
	switch id {
	case evtWinlogonInit:
		if p.timeline.WinlogonInit.IsZero() {
			p.timeline.WinlogonInit = ts
		}
	case evtWinlogonInitDone:
		if p.timeline.WinlogonInitDone.IsZero() {
			p.timeline.WinlogonInitDone = ts
		}
	case evtLoginUIStart:
		if p.timeline.LoginUIStart.IsZero() {
			p.timeline.LoginUIStart = ts
		}
	case evtLoginUIDone:
		if p.timeline.LoginUIDone.IsZero() {
			p.timeline.LoginUIDone = ts
		}
	case evtWinlogonShellCmdStart:
		if p.timeline.ExecuteShellCommandListStart.IsZero() {
			p.timeline.ExecuteShellCommandListStart = ts
		}
	case evtWinlogonShellCmdEnd:
		if p.timeline.ExecuteShellCommandListEnd.IsZero() {
			p.timeline.ExecuteShellCommandListEnd = ts
		}
	case evtLogonStart:
		if p.timeline.LogonStart.IsZero() {
			p.timeline.LogonStart = ts
		}
	case evtLogonStop:
		if p.timeline.LogonStop.IsZero() {
			p.timeline.LogonStop = ts
		}
	}
}

// userProfileParser processes User Profile Service events.
type userProfileParser struct {
	timeline *BootTimeline
}

func (p *userProfileParser) Parse(_ eventWithProperties, id uint16, ts time.Time) {
	switch id {
	case evtProfileLoadStart:
		if p.timeline.ProfileLoadStart.IsZero() {
			p.timeline.ProfileLoadStart = ts
		}
	case evtProfileLoadEnd:
		if p.timeline.ProfileLoadEnd.IsZero() {
			p.timeline.ProfileLoadEnd = ts
		}
	case evtProfileCreationStart:
		if p.timeline.ProfileCreationStart.IsZero() {
			p.timeline.ProfileCreationStart = ts
		}
	case evtProfileCreationEnd:
		if p.timeline.ProfileCreationEnd.IsZero() {
			p.timeline.ProfileCreationEnd = ts
		}
	}
}

// groupPolicyParser processes Group Policy events (4000/4001: start, 8000/8001: end).
type groupPolicyParser struct {
	timeline *BootTimeline
}

func (p *groupPolicyParser) Parse(_ eventWithProperties, id uint16, ts time.Time) {
	switch id {
	case evtMachineGPStart:
		if p.timeline.MachineGPStart.IsZero() {
			p.timeline.MachineGPStart = ts
		}
	case evtMachineGPEnd:
		if p.timeline.MachineGPEnd.IsZero() {
			p.timeline.MachineGPEnd = ts
		}
	case evtUserGPStart:
		if p.timeline.UserGPStart.IsZero() {
			p.timeline.UserGPStart = ts
		}
	case evtUserGPEnd:
		if p.timeline.UserGPEnd.IsZero() {
			p.timeline.UserGPEnd = ts
		}
	}
}

// shellCoreParser processes Shell-Core events for Explorer startup tracking.
type shellCoreParser struct {
	timeline *BootTimeline
}

func (p *shellCoreParser) Parse(e eventWithProperties, id uint16, ts time.Time) {
	switch id {
	case evtExplorerInitStart:
		if p.timeline.ExplorerInitStart.IsZero() {
			p.timeline.ExplorerInitStart = ts
		}
	case evtExplorerInitEnd:
		if p.timeline.ExplorerInitEnd.IsZero() {
			p.timeline.ExplorerInitEnd = ts
		}
	case evtDesktopCreateStart:
		if p.timeline.DesktopCreateStart.IsZero() {
			p.timeline.DesktopCreateStart = ts
		}
	case evtDesktopCreateEnd:
		if p.timeline.DesktopCreateEnd.IsZero() {
			p.timeline.DesktopCreateEnd = ts
		}
	case evtExplorerStepStart:
		stepName := strings.ToLower(explorerStepName(e))
		switch stepName {
		case "waitfordesktopvisuals":
			if p.timeline.DesktopVisibleStart.IsZero() {
				p.timeline.DesktopVisibleStart = ts
			}
		case "finalize":
			if p.timeline.DesktopReadyStart.IsZero() {
				p.timeline.DesktopReadyStart = ts
			}
		case "desktopstartupapps":
			if p.timeline.DesktopStartupAppsStart.IsZero() {
				p.timeline.DesktopStartupAppsStart = ts
			}
		}
	case evtExplorerStepEnd:
		stepName := strings.ToLower(explorerStepName(e))
		switch stepName {
		case "waitfordesktopvisuals":
			if p.timeline.DesktopVisibleEnd.IsZero() {
				p.timeline.DesktopVisibleEnd = ts
			}
		case "finalize":
			if p.timeline.DesktopReadyEnd.IsZero() {
				p.timeline.DesktopReadyEnd = ts
			}
		case "desktopstartupapps":
			if p.timeline.DesktopStartupAppsEnd.IsZero() {
				p.timeline.DesktopStartupAppsEnd = ts
			}
		}
	}
}

// getEventPropString finds a named property in the Event and returns its string value.
func getEventPropString(e eventWithProperties, name string) string {
	return e.GetPropertyString(name)
}

// explorerStepName extracts the step name from the "psz" property of a
// Shell-Core 9648/9649 Explorer_Startup_Step event.
func explorerStepName(e eventWithProperties) string {
	return getEventPropString(e, "psz")
}
