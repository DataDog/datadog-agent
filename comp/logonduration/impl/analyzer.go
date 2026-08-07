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
	evtLoginUIStart          uint16 = 103
	evtLoginUIDone           uint16 = 104
	evtSessionLogon          uint16 = 7001

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

	// evtGPActivityStartMax is the last of the 4000-4007 activity-start range.
	// The range covers boot (4000/4001), network state change (4002/4003),
	// manual gpupdate (4004/4005), and periodic refresh (4006/4007), and
	// alternates by parity: even is computer scope, odd is user scope.
	evtGPActivityStartMax uint16 = 4007

	// Group Policy client-side extensions. The three stop events share an
	// identical template and differ only in severity.
	evtCSEStart       uint16 = 4016
	evtCSEStopSuccess uint16 = 5016
	evtCSEStopWarning uint16 = 6016
	evtCSEStopError   uint16 = 7016

	// evtGPOListApplicable enumerates the Group Policy objects found applicable.
	// Its counterpart 5313 lists the objects filtered out and is not collected.
	evtGPOListApplicable uint16 = 5312

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
	SessionLogon                 time.Time // Winlogon Event 7001 (Session Logon)
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
	ExplorerStart                time.Time // Kernel-Process Event 1 (explorer.exe)
	ExplorerInitStart            time.Time // Shell-Core Event 9601 (Explorer_InitializingExplorerStart)
	ExplorerInitEnd              time.Time // Shell-Core Event 9602 (Explorer_InitializingExplorerStop)
	DesktopCreateStart           time.Time // Shell-Core Event 9611 (Explorer_CreateDesktopStart)
	DesktopCreateEnd             time.Time // Shell-Core Event 9612 (Explorer_CreateDesktopStop)
	DesktopVisibleStart          time.Time // Shell-Core Event 9648 (waitfordesktopvisuals step)
	DesktopVisibleEnd            time.Time // Shell-Core Event 9649 (waitfordesktopvisuals step)

	// Winlogon sub-events for detailed component timing
	LoginUIStart time.Time // Winlogon Event 103
	LoginUIDone  time.Time // Winlogon Event 104

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
	timeline    BootTimeline
	groupPolicy *gpAccumulator
	providers   map[windows.GUID]providerConfig
}

// buildProviders wires each provider's accepted event IDs together with
// its parser, creating a single source of truth for both filtering and
// dispatching.
func buildProviders(timeline *BootTimeline, gp *gpAccumulator) map[windows.GUID]providerConfig {
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
				evtLoginUIStart: {}, evtLoginUIDone: {},
				evtSessionLogon: {},
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
			// This map is a hard pre-parse gate: analyzeETL hands it to the ETW
			// filter, so an ID missing here never reaches the parser at all.
			// processEvent does not re-check it, which means no unit test that
			// drives processEvent can catch an omission - see
			// TestAcceptedIDsCoversGroupPolicySwitch.
			acceptedIDs: map[uint16]struct{}{
				evtMachineGPStart: {}, evtMachineGPEnd: {},
				evtUserGPStart: {}, evtUserGPEnd: {},
				// Non-boot activity starts, which seed the scope of any
				// extension invocation correlated to them.
				4002: {}, 4003: {}, 4004: {}, 4005: {}, 4006: {}, 4007: {},
				evtCSEStart:       {},
				evtCSEStopSuccess: {}, evtCSEStopWarning: {}, evtCSEStopError: {},
				evtGPOListApplicable: {},
			},
			parser: &groupPolicyParser{timeline: timeline, gp: gp},
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
	// GroupPolicy holds the client-side-extension invocations and Group Policy
	// object metadata observed during the trace. It is nil when the trace
	// contained no Group Policy events at all.
	GroupPolicy *GroupPolicyPayload
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

	coll := &collector{groupPolicy: newGPAccumulator()}
	coll.providers = buildProviders(&coll.timeline, coll.groupPolicy)

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
		Timeline:    coll.timeline,
		GroupPolicy: coll.groupPolicy.finalize(),
	}, nil
}

// directPropertyLookup is optionally implemented by events that support
// looking up a single property by name (via TdhGetProperty), bypassing
// sequential parsing that can fail on schema-mismatched properties.
//
// It is only correct for string-typed properties: the underlying call returns
// the property's raw bytes, which are then read as UTF-16. A GUID, boolean, or
// integer property decodes to silent garbage this way.
type directPropertyLookup interface {
	GetPropertyByName(name string) (string, error)
}

// activityScoped is optionally implemented by events exposing the ETW activity
// ID, which correlates events belonging to one instance of an operation.
type activityScoped interface {
	GetActivityID() windows.GUID
}

// bulkPropertyLookup is optionally implemented by events that can return every
// property in one decode.
type bulkPropertyLookup interface {
	EventProperties() (map[string]interface{}, error)
}

// activityIDOf returns an event's ETW activity ID, or the zero GUID when the
// event does not expose one. Safe on a nil event: a type assertion on a nil
// interface simply reports false.
func activityIDOf(e eventWithProperties) windows.GUID {
	if scoped, ok := e.(activityScoped); ok {
		return scoped.GetActivityID()
	}
	return windows.GUID{}
}

// eventPropertyReader returns a lookup function over an event's properties.
//
// Property access has no cache: each GetPropertyString call re-parses the whole
// event schema and every property in it, so reading five fields the naive way
// costs five full TDH decodes. When the event supports a bulk read, this does
// one decode and serves every subsequent lookup from the result. A bulk read
// that fails part-way still returns the properties preceding the failure, so
// the partial result is used when it has anything in it.
func eventPropertyReader(e eventWithProperties) func(string) string {
	if e == nil {
		return func(string) string { return "" }
	}
	if bulk, ok := e.(bulkPropertyLookup); ok {
		props, err := bulk.EventProperties()
		if err != nil {
			log.Debugf("Logon duration: partial property decode (%d properties recovered): %v", len(props), err)
		}
		if len(props) > 0 {
			return func(name string) string {
				v, ok := props[name]
				if !ok {
					return ""
				}
				return fmt.Sprintf("%v", v)
			}
		}
	}
	return func(name string) string { return getEventPropString(e, name) }
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
// Tracks the first explorer.exe process start.
type kernelProcessParser struct {
	timeline *BootTimeline
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

	if strings.Contains(imageName, "explorer.exe") && p.timeline.ExplorerStart.IsZero() {
		p.timeline.ExplorerStart = ts
	}
}

// winlogonParser processes Winlogon events for logon lifecycle tracking.
type winlogonParser struct {
	timeline *BootTimeline
}

func (p *winlogonParser) Parse(_ eventWithProperties, id uint16, ts time.Time) {
	switch id {
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
	case evtSessionLogon:
		if p.timeline.SessionLogon.IsZero() {
			p.timeline.SessionLogon = ts
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

// groupPolicyParser processes Group Policy events: the aggregate pass
// milestones (4000/4001 start, 8000/8001 end), the client-side-extension
// invocations within each pass (4016 start, 5016/6016/7016 stop), and the
// applicable Group Policy object inventory (5312).
type groupPolicyParser struct {
	timeline *BootTimeline
	gp       *gpAccumulator
}

func (p *groupPolicyParser) Parse(e eventWithProperties, id uint16, ts time.Time) {
	switch id {
	case evtMachineGPStart:
		if p.timeline.MachineGPStart.IsZero() {
			p.timeline.MachineGPStart = ts
		}
		p.gp.noteActivityStart(activityIDOf(e), id)
	case evtMachineGPEnd:
		if p.timeline.MachineGPEnd.IsZero() {
			p.timeline.MachineGPEnd = ts
		}
	case evtUserGPStart:
		if p.timeline.UserGPStart.IsZero() {
			p.timeline.UserGPStart = ts
		}
		p.gp.noteActivityStart(activityIDOf(e), id)
	case evtUserGPEnd:
		if p.timeline.UserGPEnd.IsZero() {
			p.timeline.UserGPEnd = ts
		}
	case 4002, 4003, 4004, 4005, 4006, 4007:
		// Non-boot policy processing. These seed the scope of any extension
		// invocation correlated to them but are not the boot pass reported by
		// the timeline, so they do not mark the pass as observed.
		p.gp.noteActivityStart(activityIDOf(e), id)
	case evtCSEStart:
		p.parseCSEStart(e, ts)
	case evtCSEStopSuccess, evtCSEStopWarning, evtCSEStopError:
		p.parseCSEStop(e, id, ts)
	case evtGPOListApplicable:
		p.parseGPOInventory(e)
	}
}

// parseCSEStart handles event 4016, which opens an extension invocation and
// carries the list of Group Policy objects feeding it.
func (p *groupPolicyParser) parseCSEStart(e eventWithProperties, ts time.Time) {
	prop := eventPropertyReader(e)

	guid, guidString, ok := normalizeGUID(prop("CSEExtensionId"))
	if !ok {
		// Without an extension identity the invocation cannot be paired with
		// its terminal event, so there is nothing meaningful to record.
		log.Debugf("Logon duration: CSE start event has no usable CSEExtensionId")
		p.gp.parseErrors++
		return
	}

	// Absent or unrecognized, treat the extension as synchronous: that is the
	// common case, and the flag only ever suppresses a duration.
	isAsync, _ := parseETWBool(prop("IsExtensionAsyncProcessing"))

	ids, gpos, degraded := parseApplicableGPOList(prop("ApplicableGPOList"))
	if degraded {
		p.gp.parseErrors++
	}
	// Register the referenced objects so every applicable_gpo_ids value
	// resolves against the payload's GPO table even when no 5312 was observed.
	p.gp.addGPOs(gpos)
	p.gp.addGPOIDs(ids)

	p.gp.startCSE(activityIDOf(e), observedCSEStart{
		guid:             guid,
		guidString:       guidString,
		name:             prop("CSEExtensionName"),
		isAsync:          isAsync,
		applicableGPOIDs: ids,
	}, ts)
}

// parseCSEStop handles events 5016, 6016, and 7016, which close an extension
// invocation. Note the provider's own misspelling of the elapsed-time field.
func (p *groupPolicyParser) parseCSEStop(e eventWithProperties, id uint16, ts time.Time) {
	prop := eventPropertyReader(e)

	guid, guidString, ok := normalizeGUID(prop("CSEExtensionId"))
	if !ok {
		log.Debugf("Logon duration: CSE stop event %d has no usable CSEExtensionId", id)
		p.gp.parseErrors++
		return
	}

	stop := observedCSEStop{
		eventID:    id,
		guid:       guid,
		guidString: guidString,
		name:       prop("CSEExtensionName"),
	}
	if elapsed, ok := parseUint32(prop("CSEElaspedTimeInMilliSeconds")); ok {
		stop.elapsedMs = elapsed
		stop.hasElapsed = true
	}
	if code, ok := formatErrorCode(prop("ErrorCode")); ok {
		stop.errorCode = code
	}

	p.gp.finishCSE(activityIDOf(e), stop, ts)
}

// parseGPOInventory handles event 5312, the list of applicable Group Policy
// objects. It carries no timing fields, so the objects are recorded as metadata
// only.
func (p *groupPolicyParser) parseGPOInventory(e eventWithProperties) {
	prop := eventPropertyReader(e)

	gpos, err := parseGPOList(prop("GPOInfoList"))
	if err != nil {
		log.Debugf("Logon duration: parsing GPOInfoList: %v", err)
		p.gp.parseErrors++
	}
	p.gp.addGPOs(gpos)
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
