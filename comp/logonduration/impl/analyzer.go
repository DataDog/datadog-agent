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

	// Group Policy client-side extensions. The three stop events share an
	// identical template and differ only in severity.
	//
	// The applicable-GPO inventory events 5312 and 5313 are deliberately not
	// collected: 4016's own ApplicableGPOList carries both the GUID and the
	// display name of every object feeding that invocation, so an inventory adds
	// nothing an emitted invocation does not already have.
	evtCSEStart       uint16 = 4016
	evtCSEStopSuccess uint16 = 5016
	evtCSEStopWarning uint16 = 6016
	evtCSEStopError   uint16 = 7016

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
			// TestAcceptedGroupPolicyIDsSnapshot.
			acceptedIDs: map[uint16]struct{}{
				evtMachineGPStart: {}, evtMachineGPEnd: {},
				evtUserGPStart: {}, evtUserGPEnd: {},
				evtCSEStart:       {},
				evtCSEStopSuccess: {}, evtCSEStopWarning: {}, evtCSEStopError: {},
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
	// GroupPolicy holds the client-side-extension invocations measured during
	// each boot Group Policy pass. It is nil when none were.
	GroupPolicy *GroupPolicyDetails
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
		GroupPolicy: coll.groupPolicy.finalize(coll.timeline),
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
//
// A bulk read that fails having recovered nothing serves empty directly rather
// than falling through, because the per-property path routes back into the same
// decode and would re-run the identical failure on every lookup.
func eventPropertyReader(e eventWithProperties) func(string) string {
	if e == nil {
		return func(string) string { return "" }
	}
	if bulk, ok := e.(bulkPropertyLookup); ok {
		props, err := bulk.EventProperties()
		if err != nil {
			log.Debugf("Logon duration: partial property decode (%d properties recovered): %v", len(props), err)
			if len(props) == 0 {
				return func(string) string { return "" }
			}
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
// milestones (4000/4001 start, 8000/8001 end) and the client-side-extension
// invocations within each pass (4016 start, 5016/6016/7016 stop).
type groupPolicyParser struct {
	timeline *BootTimeline
	gp       *gpAccumulator
}

func (p *groupPolicyParser) Parse(e eventWithProperties, id uint16, ts time.Time) {
	switch id {
	// Only the pass start events seed the activity table. Taking it from the
	// same event that sets the milestone is what keeps the two consistent: a
	// scope can never claim invocations without also reporting the pass they
	// belong to.
	case evtMachineGPStart:
		if p.timeline.MachineGPStart.IsZero() {
			p.timeline.MachineGPStart = ts
		}
		p.gp.notePassActivity(activityIDOf(e), id)
	case evtMachineGPEnd:
		if p.timeline.MachineGPEnd.IsZero() {
			p.timeline.MachineGPEnd = ts
		}
	case evtUserGPStart:
		if p.timeline.UserGPStart.IsZero() {
			p.timeline.UserGPStart = ts
		}
		p.gp.notePassActivity(activityIDOf(e), id)
	case evtUserGPEnd:
		if p.timeline.UserGPEnd.IsZero() {
			p.timeline.UserGPEnd = ts
		}
	case evtCSEStart:
		p.parseCSEStart(e, ts)
	case evtCSEStopSuccess, evtCSEStopWarning, evtCSEStopError:
		p.parseCSEStop(e, id, ts)
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
		return
	}

	// Absent or unrecognized, treat the extension as synchronous: that is the
	// common case, and the flag only annotates the duration's meaning.
	isAsync, _ := parseETWBool(prop("IsExtensionAsyncProcessing"))

	// ApplicableGPOList carries the GUID and the display name of every object
	// feeding this invocation, so one walk yields both. Names go into the
	// accumulator's shared lookup rather than onto the record: a list this walk
	// could not finish contributes IDs without their names, and another
	// invocation that listed the same object successfully fills them in.
	ids, names, omitted := gpoRefsFromList(prop("ApplicableGPOList"))
	p.gp.mergeGPONames(names)

	p.gp.startCSE(activityIDOf(e), observedCSEStart{
		guid:        guid,
		guidString:  guidString,
		name:        prop("CSEExtensionName"),
		isAsync:     isAsync,
		gpoIDs:      ids,
		gposOmitted: omitted,
	}, ts)
}

// parseCSEStop handles events 5016, 6016, and 7016, which close an extension
// invocation.
//
// The provider's own CSEElaspedTimeInMilliSeconds is deliberately not read. The
// emitted duration is the measured 4016-to-terminal interval, which is the same
// kind of wall-clock measurement as the pass duration it is a slice of; a
// second, differently-derived number would leave consumers without a clear
// authoritative field.
//
// ErrorCode is not read either. The event ID already carries the outcome, and
// the status behind a failure is a Group Policy health question rather than
// latency data - see CSEInvocation.Result.
//
// One asymmetry with 4016 is worth knowing about. These templates declare
// CSEExtensionId last, where 4016 declares it first, and EventProperties
// recovers only the properties preceding a decode failure. So a partial decode
// of a 4016 merely loses the async flag and the GPO list, while a partial
// decode here loses the identity and therefore the whole invocation - its start
// is left open and dropped at finalize. GetPropertyByName is not the fallback:
// it reads the property's raw bytes as UTF-16, which turns a 16-byte win:GUID
// into eight junk runes. Recovering it would need a raw-bytes accessor in the
// etw package. The first three properties are two fixed-width integers and a
// string, so the exposure is small.
func (p *groupPolicyParser) parseCSEStop(e eventWithProperties, id uint16, ts time.Time) {
	prop := eventPropertyReader(e)

	guid, guidString, ok := normalizeGUID(prop("CSEExtensionId"))
	if !ok {
		log.Debugf("Logon duration: CSE stop event %d has no usable CSEExtensionId", id)
		return
	}

	p.gp.finishCSE(activityIDOf(e), observedCSEStop{
		eventID:    id,
		guid:       guid,
		guidString: guidString,
		name:       prop("CSEExtensionName"),
	}, ts)
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
