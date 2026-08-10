// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package logondurationimpl

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	pkgstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

// This file defines the group_policy_details block of the logon duration
// payload: the client-side-extension (CSE) invocations observed during each
// Group Policy pass, and the Group Policy objects that fed each one.
//
// Windows times CSE invocations but not GPOs. Event 4016 starts an invocation
// and 5016/6016/7016 end it, giving a real measured interval. Event 5312
// enumerates applicable GPOs and carries no timing fields whatsoever, so a GPO
// is emitted as an untimed reference and never inherits a CSE's duration.

const (
	// maxGPOListBytes rejects an implausibly large GPOInfoList/ApplicableGPOList
	// before any parsing work happens.
	maxGPOListBytes = 256 * 1024

	// maxGPOsPerCSE bounds the GPO references carried by one invocation. GPOs
	// are inlined per invocation rather than pooled in a shared table, so this
	// is what keeps a pathological ApplicableGPOList - 256 KB of braced GUIDs is
	// roughly 6,700 of them - from being repeated across every invocation.
	maxGPOsPerCSE = 64

	// Provider-supplied text is never emitted unbounded.
	maxCSENameBytes = 128
	maxGPONameBytes = 256
)

// cseResult is the outcome of a CSE invocation, derived from which terminal
// event closed it.
type cseResult string

const (
	// cseResultSuccess is event 5016.
	cseResultSuccess cseResult = "success"
	// cseResultWarning is event 6016.
	cseResultWarning cseResult = "warning"
	// cseResultError is event 7016.
	cseResultError cseResult = "error"
)

// gpScope is the Group Policy processing scope. It is internal only: the wire
// format expresses scope structurally, through the computer / user arrays.
type gpScope int

const (
	gpScopeComputer gpScope = iota
	gpScopeUser
	gpScopeCount
)

// GroupPolicyDetails is the drill-down for the computer_group_policy and
// user_group_policy entries in boot_timeline: the client-side extensions that
// ran during each pass and the Group Policy objects that fed each one.
//
// Only invocations measured end to end appear here. An unmatched start, an
// unmatched terminal event, or an invocation still open when the trace ends has
// no interval to place on the timeline, and a record without one is a
// collection diagnostic rather than usable latency data.
type GroupPolicyDetails struct {
	Computer []CSEInvocation `json:"computer,omitempty"`
	User     []CSEInvocation `json:"user,omitempty"`
}

// CSEInvocation is one measured client-side extension invocation. It carries no
// scope field: the array it appears in determines the scope.
type CSEInvocation struct {
	// CSEID is the extension's identity in canonical braced uppercase form.
	CSEID   string `json:"cse_id"`
	CSEName string `json:"cse_name,omitempty"`

	// OffsetMs places the invocation on the same boot-relative axis as
	// boot_timeline, login-screen idle gap collapsed out, so it nests inside its
	// parent milestone with no conversion by the consumer. Both come from
	// bootOffsetFunc for exactly that reason.
	OffsetMs int64 `json:"offset_ms"`

	// DurationMs is the wall-clock 4016 -> 5016/6016/7016 interval, the same
	// kind of measurement as the pass duration (4000 -> 8000) it is a slice of.
	DurationMs int64 `json:"duration_ms"`

	Result cseResult `json:"result"`

	// ErrorCode is the provider's status as the hexadecimal string Windows
	// formats it as, present only when non-zero. A JSON number would round-trip
	// a value above 0x7FFFFFFF as a negative signed integer.
	ErrorCode string `json:"error_code,omitempty"`

	// Async reports the extension's IsExtensionAsyncProcessing flag. When set,
	// the terminal event marks the dispatch of a worker thread, so DurationMs is
	// the cost of the dispatch rather than of the extension's work.
	Async bool `json:"async,omitempty"`

	GPOs []GPORef `json:"gpos,omitempty"`
}

// GPORef identifies one Group Policy object that fed an invocation. It carries
// no timing: no Windows event reports a duration for an individual GPO.
type GPORef struct {
	// ID is the GPO GUID in canonical braced uppercase form, and is the
	// identity. Two GPOs sharing a display name remain distinct.
	ID string `json:"id"`
	// Name is absent unless a 5312 inventory event supplied it.
	Name string `json:"name,omitempty"`
}

// observedCSEStart is the decoded content of a 4016 extension-start event.
type observedCSEStart struct {
	guid       windows.GUID
	guidString string
	name       string
	isAsync    bool
	gpoIDs     []string
}

// observedCSEStop is the decoded content of a 5016/6016/7016 extension-stop
// event. The three share an identical template.
type observedCSEStop struct {
	eventID    uint16
	guid       windows.GUID
	guidString string
	name       string
	errorCode  string
}

// cseRecord is a completed invocation held until finalize, which is where its
// scope is decided. It deliberately carries no scope of its own.
type cseRecord struct {
	cseID      string
	name       string
	start      time.Time
	durationMs int64
	result     cseResult
	errorCode  string
	async      bool
	gpoIDs     []string
}

// openCSE is an invocation whose start was observed and which is awaiting a
// terminal event.
type openCSE struct {
	rec   cseRecord
	start time.Time
}

// cseKey identifies an in-flight invocation. Including the activity ID keeps
// concurrent Group Policy passes isolated: Windows runs more than one instance
// of policy processing at a time, so the extension GUID alone is not unique.
type cseKey struct {
	activity windows.GUID
	cse      windows.GUID
}

// gpAccumulator collects Group Policy observations across one ETL trace.
type gpAccumulator struct {
	// passActivity is the activity ID of the boot pass for each scope, taken
	// from the first boundary event that names it: 4000/8000 for computer,
	// 4001/8001 for user. Both ends seed it because a real trace can carry the
	// stop without the start - an observed boot carried 8001 with no 4001 - and
	// the stop identifies the pass just as well.
	//
	// First-write-wins per scope mirrors MachineGPStart/UserGPStart in
	// groupPolicyParser.Parse, so the invocations collected here always belong
	// to the pass boot_timeline reports.
	passActivity [gpScopeCount]windows.GUID
	passPinned   [gpScopeCount]bool

	open map[cseKey]*openCSE
	// done holds completed invocations keyed by the activity ID that produced
	// them. Bucketing by scope has to wait for finalize: a pass identified only
	// by its stop event is not known until after its own invocations arrive.
	done map[windows.GUID][]cseRecord

	// gpoNames maps a GPO GUID to its display name, from 5312 inventory events.
	// It is a lookup, not an inventory: an entry only reaches the wire if a
	// surviving invocation references it.
	gpoNames map[string]string
}

func newGPAccumulator() *gpAccumulator {
	return &gpAccumulator{
		open:     make(map[cseKey]*openCSE),
		done:     make(map[windows.GUID][]cseRecord),
		gpoNames: make(map[string]string),
	}
}

// notePassActivity records the activity ID of a boot Group Policy pass.
//
// The zero GUID is refused. Events outside any pass carry it - 4117, 5324, and
// 5351 were all observed with a zero ActivityId in a real trace - so accepting
// it would pin zero for a scope and sweep every unattributed invocation into
// that pass.
func (a *gpAccumulator) notePassActivity(activityID windows.GUID, id uint16) {
	var scope gpScope
	switch id {
	case evtMachineGPStart, evtMachineGPEnd:
		scope = gpScopeComputer
	case evtUserGPStart, evtUserGPEnd:
		scope = gpScopeUser
	default:
		return
	}
	if activityID == (windows.GUID{}) || a.passPinned[scope] {
		return
	}
	a.passActivity[scope] = activityID
	a.passPinned[scope] = true
}

// startCSE records a 4016 extension-start observation.
func (a *gpAccumulator) startCSE(activityID windows.GUID, o observedCSEStart, ts time.Time) {
	key := cseKey{activity: activityID, cse: o.guid}
	if _, ok := a.open[key]; ok {
		// A second start for a key already open cannot be disambiguated. The
		// later one wins: it yields the shorter, more conservative interval when
		// the terminal event arrives, and the earlier one is discarded rather
		// than emitted as a record nobody can interpret.
		log.Debugf("Logon duration: duplicate CSE start for %s, keeping the later one", o.guidString)
	}
	a.open[key] = &openCSE{
		start: ts,
		rec: cseRecord{
			cseID:  o.guidString,
			name:   truncateProviderText(o.name, maxCSENameBytes),
			start:  ts,
			async:  o.isAsync,
			gpoIDs: o.gpoIDs,
		},
	}
}

// finishCSE records a 5016/6016/7016 extension-stop observation, completing the
// invocation its start opened.
//
// A terminal event with no matching start is dropped: without both endpoints
// there is no interval, and an invocation that cannot be placed on the timeline
// is not what this payload reports.
func (a *gpAccumulator) finishCSE(activityID windows.GUID, o observedCSEStop, ts time.Time) {
	key := cseKey{activity: activityID, cse: o.guid}
	open, ok := a.open[key]
	if !ok {
		log.Debugf("Logon duration: CSE stop for %s with no matching start", o.guidString)
		return
	}
	delete(a.open, key)

	if ts.Before(open.start) {
		log.Debugf("Logon duration: CSE stop for %s precedes its start", o.guidString)
		return
	}
	result, ok := resultForStopEvent(o.eventID)
	if !ok {
		return
	}

	rec := open.rec
	rec.durationMs = ts.Sub(open.start).Milliseconds()
	rec.result = result
	rec.errorCode = o.errorCode
	if rec.name == "" {
		rec.name = truncateProviderText(o.name, maxCSENameBytes)
	}
	a.done[activityID] = append(a.done[activityID], rec)
}

// resultForStopEvent maps a terminal event ID to its outcome. The three events
// share an identical template and differ only in severity. The boolean reports
// an ID that is not a terminal event, which Parse's dispatch makes unreachable.
func resultForStopEvent(id uint16) (cseResult, bool) {
	switch id {
	case evtCSEStopSuccess:
		return cseResultSuccess, true
	case evtCSEStopWarning:
		return cseResultWarning, true
	case evtCSEStopError:
		return cseResultError, true
	default:
		return "", false
	}
}

// mergeGPONames records the display names from a 5312 inventory event.
// First non-empty name wins, so a later inventory cannot rename a GPO.
func (a *gpAccumulator) mergeGPONames(names map[string]string) {
	for id, name := range names {
		if name == "" {
			continue
		}
		if _, ok := a.gpoNames[id]; !ok {
			a.gpoNames[id] = name
		}
	}
}

// finalize assembles the emitted block, returning nil when no invocation was
// measured end to end.
func (a *gpAccumulator) finalize(tl BootTimeline) *GroupPolicyDetails {
	// An invocation still open when the trace ends has no measured interval.
	// That is the normal tail case - the capture window closes when the Agent
	// service starts - and not an error.
	a.open = nil

	offsetOf := bootOffsetFunc(tl)
	details := &GroupPolicyDetails{
		Computer: a.buildScope(gpScopeComputer, offsetOf),
		User:     a.buildScope(gpScopeUser, offsetOf),
	}
	if len(details.Computer) == 0 && len(details.User) == 0 {
		return nil
	}
	return details
}

// buildScope converts the invocations belonging to one boot pass.
//
// Records under any other activity ID are simply never visited, which is how a
// gpupdate, a periodic refresh, and an unrecognized activity are all excluded:
// an absent map key rather than a filtering rule.
func (a *gpAccumulator) buildScope(scope gpScope, offsetOf func(time.Time) int64) []CSEInvocation {
	if !a.passPinned[scope] {
		return nil
	}
	records := a.done[a.passActivity[scope]]
	if len(records) == 0 {
		return nil
	}

	out := make([]CSEInvocation, 0, len(records))
	for _, r := range records {
		out = append(out, CSEInvocation{
			CSEID:      r.cseID,
			CSEName:    r.name,
			OffsetMs:   offsetOf(r.start),
			DurationMs: r.durationMs,
			Result:     r.result,
			ErrorCode:  r.errorCode,
			Async:      r.async,
			GPOs:       a.gpoRefs(r.gpoIDs),
		})
	}

	// Chronological, with the extension GUID breaking ties so the order is
	// stable across runs.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].OffsetMs != out[j].OffsetMs {
			return out[i].OffsetMs < out[j].OffsetMs
		}
		return out[i].CSEID < out[j].CSEID
	})
	return out
}

// gpoRefs resolves an invocation's GPO references against the name lookup. It
// runs at finalize so a 5312 arriving after its 4016 still supplies names.
func (a *gpAccumulator) gpoRefs(ids []string) []GPORef {
	if len(ids) == 0 {
		return nil
	}
	refs := make([]GPORef, 0, len(ids))
	for _, id := range ids {
		refs = append(refs, GPORef{ID: id, Name: a.gpoNames[id]})
	}
	return refs
}

// bracedGUIDPattern matches a GUID in braced form anywhere in a string.
var bracedGUIDPattern = regexp.MustCompile(
	`\{[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}\}`)

// gpoIDsFromList extracts the Group Policy object references carried by a 4016
// event's ApplicableGPOList.
//
// The property is declared win:UnicodeString rather than a TDH array, so TDH
// hands it over as one opaque string, and its runtime format has never been
// observed first-hand. Two tiers cover the plausible shapes:
//
// If the value is the same <GPO ID="…"> fragment that 5312 carries, the IDs come
// from the attributes. Scanning such a fragment for braced GUIDs instead would
// also harvest the extension GUIDs inside each entry's <Extensions> element and
// report them as applicable GPOs.
//
// Otherwise the delimiter is unknown, so the value is scanned for braced GUIDs.
// That recovers the references whether the provider emits a semicolon-separated
// list, one per line, or prose, and assumes only that the GUIDs appear in braced
// form - which every reported variant satisfies.
func gpoIDsFromList(raw string) []string {
	if raw == "" {
		return nil
	}
	if len(raw) > maxGPOListBytes {
		log.Debugf("Logon duration: ApplicableGPOList of %d bytes exceeds the parsable size", len(raw))
		return nil
	}

	var ids []string
	forEachGPOElement(raw, func(id string, _ gpoXML) {
		ids = appendUniqueGPOID(ids, id)
	})
	if len(ids) > 0 {
		return ids
	}

	for _, match := range bracedGUIDPattern.FindAllString(raw, -1) {
		if _, normalized, ok := normalizeGUID(match); ok {
			ids = appendUniqueGPOID(ids, normalized)
		}
	}
	return ids
}

// appendUniqueGPOID adds an ID unless it is empty, already present, or the
// per-invocation bound has been reached. The list is short enough that a linear
// scan beats maintaining a set.
func appendUniqueGPOID(ids []string, id string) []string {
	if id == "" || len(ids) >= maxGPOsPerCSE {
		return ids
	}
	for _, existing := range ids {
		if existing == id {
			return ids
		}
	}
	return append(ids, id)
}

// gpoXML is the decoded body of one <GPO> element. The GUID lives in an
// attribute and is read separately, case-insensitively.
type gpoXML struct {
	Name string `xml:"Name"`
}

// gpoNamesFromInventory extracts GUID-to-display-name pairs from a GPOInfoList
// or an ApplicableGPOList that turns out to carry the same shape.
func gpoNamesFromInventory(raw string) map[string]string {
	if raw == "" || len(raw) > maxGPOListBytes {
		return nil
	}

	names := make(map[string]string)
	forEachGPOElement(raw, func(id string, entry gpoXML) {
		if name := truncateProviderText(entry.Name, maxGPONameBytes); name != "" {
			names[id] = name
		}
	})
	return names
}

// forEachGPOElement walks a <GPO> list fragment, calling fn once per entry that
// carries a usable GUID.
//
// The fragment has no root element - it is a bare sequence of sibling
// <GPO ID="{GUID}"> entries - so xml.Unmarshal would stop after the first one
// and a token loop is used instead. That also parses a rooted variant, which
// matters because no first-party capture confirms the runtime shape: the field
// type comes from the provider manifest and the structure from third-party
// captures. Every path here yields fewer entries rather than guessing.
//
// An entry with no resolvable GUID is skipped: the GUID is the identity, and a
// display name is neither unique nor stable enough to stand in for it.
func forEachGPOElement(raw string, fn func(id string, entry gpoXML)) {
	decoder := xml.NewDecoder(strings.NewReader(raw))
	for {
		token, err := decoder.Token()
		if err != nil || token == nil {
			// End of input, or a malformed remainder. Keep what parsed.
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "GPO" {
			// Keep walking rather than skipping the subtree: the fragment has no
			// root, so a wrapper element may enclose the entries we want.
			continue
		}

		var entry gpoXML
		if err := decoder.DecodeElement(&entry, &start); err != nil {
			break
		}
		if _, id, ok := normalizeGUID(gpoIDAttr(start)); ok {
			fn(id, entry)
		}
	}
}

// gpoIDAttr reads a <GPO> element's ID attribute. The casing the provider uses
// is not confirmed against a live capture, so the match is case-insensitive.
func gpoIDAttr(start xml.StartElement) string {
	for _, attr := range start.Attr {
		if strings.EqualFold(attr.Name.Local, "id") {
			return attr.Value
		}
	}
	return ""
}

// normalizeGUID parses a GUID from provider text and returns it alongside its
// canonical braced uppercase rendering, which is the form used on the wire.
//
// Braces are added when absent because windows.GUIDFromString requires them.
func normalizeGUID(s string) (windows.GUID, string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return windows.GUID{}, "", false
	}
	if !strings.HasPrefix(s, "{") {
		s = "{" + s + "}"
	}
	g, err := windows.GUIDFromString(s)
	if err != nil {
		return windows.GUID{}, "", false
	}
	return g, g.String(), true
}

// formatErrorCode renders a non-zero provider status in a canonical form, so
// the payload never carries unparsed provider text and never varies with how
// TDH happened to format the value. A zero status means success and carries no
// information, so it reports false alongside an unparsable value: neither is
// emitted.
//
// Parsing must use base 0, not base 10: ErrorCode is declared with the
// win:HexInt32 out-type, so TDH formats it as "0x8000000A".
func formatErrorCode(s string) (string, bool) {
	v, ok := parseUint32(s)
	if !ok || v == 0 {
		return "", false
	}
	return fmt.Sprintf("0x%08X", v), true
}

// parseUint32 parses an unsigned value that may be rendered in decimal or, for
// hex-out-type properties, as "0x...".
func parseUint32(s string) (uint32, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseUint(s, 0, 32)
	if err != nil {
		return 0, false
	}
	return uint32(v), true
}

// parseETWBool interprets a formatted win:Boolean property. TDH renders these
// as "true"/"false", but the same trace was observed rendering the sibling
// IsMachine field as "1" and "0" on some events, so the numeric spellings are
// accepted too.
func parseETWBool(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "-1":
		return true, true
	case "false", "0":
		return false, true
	default:
		return false, false
	}
}

// truncateProviderText bounds a provider-supplied string, reserving room for
// the ellipsis so the result never exceeds the limit.
func truncateProviderText(s string, maxBytes int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxBytes {
		return s
	}
	const ellipsis = "..."
	return pkgstrings.TruncateUTF8(s, maxBytes-len(ellipsis)) + ellipsis
}
