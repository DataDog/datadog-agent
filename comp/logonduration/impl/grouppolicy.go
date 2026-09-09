// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package logondurationimpl

import (
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	pkgstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

// Bounds on what one payload may carry, fixed rather than configurable: nothing
// between here and the intake enforces a payload size, and an oversize event is
// discarded there long after the send returned nil. A worst-case-size test derives one
// byte budget from all four.
const (
	// maxCSEInvocationsPerScope is a backstop: one invocation per extension per pass.
	maxCSEInvocationsPerScope = 64

	// maxGPOsPerCSE bounds the GPO references carried by one invocation. GPOs are
	// inlined per invocation rather than pooled, so one list is repeated for every
	// extension that applied the same objects. What the cap drops is reported in
	// GPOsOmitted.
	maxGPOsPerCSE = 32

	// The name caps differ because their sources do: a CSE name is provider-registered
	// and fixed, a GPO display name is chosen in AD, 256 characters at two bytes each.
	maxCSENameBytes = 128
	maxGPONameBytes = 512
)

// cseResult is the outcome of a CSE invocation, from the terminal event that closed it.
type cseResult string

const (
	cseResultSuccess cseResult = "success"
	cseResultWarning cseResult = "warning"
	cseResultError   cseResult = "error"
)

// gpScope is the Group Policy processing scope.
type gpScope int

const (
	gpScopeComputer gpScope = iota
	gpScopeUser
	gpScopeCount
)

// GroupPolicyDetails is the drill-down for boot_timeline's computer_group_policy
// and user_group_policy entries; a populated array implies its parent milestone.
type GroupPolicyDetails struct {
	Computer []CSEInvocation `json:"computer,omitempty"`
	// ComputerCSEsOmitted counts what maxCSEInvocationsPerScope cut from Computer.
	ComputerCSEsOmitted int `json:"computer_cses_omitted,omitempty"`

	User []CSEInvocation `json:"user,omitempty"`
	// UserCSEsOmitted counts what maxCSEInvocationsPerScope cut from User.
	UserCSEsOmitted int `json:"user_cses_omitted,omitempty"`
}

// CSEInvocation is one measured CSE invocation; the array it appears in sets the scope.
type CSEInvocation struct {
	CSEID   string `json:"cse_id"` // canonical braced uppercase GUID
	CSEName string `json:"cse_name,omitempty"`

	OffsetMs int64 `json:"offset_ms"`

	DurationMs int64 `json:"duration_ms"` // wall-clock 4016 -> 5016/6016/7016 interval

	Result cseResult `json:"result"`

	// Async makes DurationMs the cost of dispatching a worker thread, not of the work.
	Async bool `json:"async,omitempty"`

	GPOs []GPORef `json:"gpos,omitempty"`

	// GPOsOmitted counts distinct references maxGPOsPerCSE dropped; absent when none.
	GPOsOmitted int `json:"gpos_omitted,omitempty"`
}

// GPORef identifies one Group Policy object. No Windows event times an individual GPO.
type GPORef struct {
	ID   string `json:"id"`             // canonical braced uppercase GUID; the identity
	Name string `json:"name,omitempty"` // absent unless an ApplicableGPOList supplied it
}

// observedCSEStart is the decoded content of a 4016 extension-start event.
type observedCSEStart struct {
	guid        windows.GUID
	guidString  string
	name        string
	isAsync     bool
	gpoIDs      []string
	gposOmitted int
}

// observedCSEStop is the decoded content of a 5016/6016/7016 extension-stop event.
type observedCSEStop struct {
	eventID    uint16
	guid       windows.GUID
	guidString string
	name       string
}

// cseRecord is a completed invocation held until finalize, which decides its scope.
type cseRecord struct {
	cseID       string
	name        string
	start       time.Time
	durationMs  int64
	result      cseResult
	async       bool
	gpoIDs      []string
	gposOmitted int
}

type openCSE struct {
	rec   cseRecord
	start time.Time
}

// cseKey pairs the activity GUID with the CSE GUID: concurrent passes reuse CSE GUIDs.
type cseKey struct {
	activity windows.GUID
	cse      windows.GUID
}

// gpAccumulator collects Group Policy observations across one ETL trace. Its maps
// are unguarded: ETL callbacks arrive serially on one thread.
type gpAccumulator struct {
	// passActivity is each scope's pass activity ID, from 4000 / 4001. First write wins.
	passActivity [gpScopeCount]windows.GUID
	passPinned   [gpScopeCount]bool

	open map[cseKey]*openCSE
	// done buckets completed invocations by activity ID: an absent key excludes a pass.
	done map[windows.GUID][]cseRecord

	// gpoNames pools names from every 4016 list, so a partial walk can still be named.
	gpoNames map[string]string
}

func newGPAccumulator() *gpAccumulator {
	return &gpAccumulator{
		open:     make(map[cseKey]*openCSE),
		done:     make(map[windows.GUID][]cseRecord),
		gpoNames: make(map[string]string),
	}
}

// notePassActivity pins a pass activity ID. Zero is refused: off-pass events carry it.
func (a *gpAccumulator) notePassActivity(activityID windows.GUID, id uint16) {
	var scope gpScope
	switch id {
	case evtMachineGPStart:
		scope = gpScopeComputer
	case evtUserGPStart:
		scope = gpScopeUser
	default:
		return
	}
	if activityID == (windows.GUID{}) || a.passPinned[scope] {
		return
	}
	// One activity must not pin two scopes, or buildScope emits its bucket under both.
	for other := gpScope(0); other < gpScopeCount; other++ {
		if a.passPinned[other] && a.passActivity[other] == activityID {
			log.Debugf("Logon duration: pass activity %s already belongs to another scope", activityID)
			return
		}
	}
	a.passActivity[scope] = activityID
	a.passPinned[scope] = true
}

// startCSE records a 4016 extension-start observation.
func (a *gpAccumulator) startCSE(activityID windows.GUID, o observedCSEStart, ts time.Time) {
	key := cseKey{activity: activityID, cse: o.guid}
	if _, ok := a.open[key]; ok {
		log.Debugf("Logon duration: duplicate CSE start for %s, keeping the later one", o.guidString)
	}
	a.open[key] = &openCSE{
		start: ts,
		rec: cseRecord{
			cseID:       o.guidString,
			name:        truncateProviderText(o.name, maxCSENameBytes),
			start:       ts,
			async:       o.isAsync,
			gpoIDs:      o.gpoIDs,
			gposOmitted: o.gposOmitted,
		},
	}
}

// finishCSE completes the invocation its start opened; a stop with no start is dropped.
func (a *gpAccumulator) finishCSE(activityID windows.GUID, o observedCSEStop, ts time.Time) {
	key := cseKey{activity: activityID, cse: o.guid}
	open, ok := a.open[key]
	if !ok {
		log.Debugf("Logon duration: CSE stop for %s with no matching start", o.guidString)
		return
	}

	// Check the timestamp before consuming the start, so a bad stop leaves it open.
	if ts.Before(open.start) {
		log.Debugf("Logon duration: CSE stop for %s precedes its start", o.guidString)
		return
	}
	delete(a.open, key)
	result, ok := resultForStopEvent(o.eventID)
	if !ok {
		return
	}

	rec := open.rec
	rec.durationMs = ts.Sub(open.start).Milliseconds()
	rec.result = result
	if rec.name == "" {
		rec.name = truncateProviderText(o.name, maxCSENameBytes)
	}
	a.done[activityID] = append(a.done[activityID], rec)
}

// resultForStopEvent maps a terminal event ID to its outcome. The boolean reports
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

// mergeGPONames records one list's display names, first name winning.
func (a *gpAccumulator) mergeGPONames(names map[string]string) {
	for id, name := range names {
		if _, ok := a.gpoNames[id]; !ok {
			a.gpoNames[id] = name
		}
	}
}

// finalize assembles the block, returning nil when nothing was measured end to end.
// Still-open records have no interval and are never closed against the trace end.
func (a *gpAccumulator) finalize(tl BootTimeline) *GroupPolicyDetails {
	offsetOf := bootOffsetFunc(tl)
	computer, computerOmitted := a.buildScope(gpScopeComputer, offsetOf)
	user, userOmitted := a.buildScope(gpScopeUser, offsetOf)
	if len(computer) == 0 && len(user) == 0 {
		return nil
	}
	return &GroupPolicyDetails{
		Computer:            computer,
		ComputerCSEsOmitted: computerOmitted,
		User:                user,
		UserCSEsOmitted:     userOmitted,
	}
}

// buildScope converts the invocations belonging to one boot pass, with the count the cap cut.
func (a *gpAccumulator) buildScope(scope gpScope, offsetOf func(time.Time) int64) ([]CSEInvocation, int) {
	if !a.passPinned[scope] {
		return nil, 0
	}
	records := a.done[a.passActivity[scope]]
	if len(records) == 0 {
		return nil, 0
	}

	out := make([]CSEInvocation, 0, len(records))
	for _, r := range records {
		out = append(out, CSEInvocation{
			CSEID:       r.cseID,
			CSEName:     r.name,
			OffsetMs:    offsetOf(r.start),
			DurationMs:  r.durationMs,
			Result:      r.result,
			Async:       r.async,
			GPOs:        a.gpoRefs(r.gpoIDs),
			GPOsOmitted: r.gposOmitted,
		})
	}

	omitted := 0
	if len(out) > maxCSEInvocationsPerScope {
		omitted = len(out) - maxCSEInvocationsPerScope
		log.Warnf("Logon duration: %d Group Policy extension invocations in one pass exceeds the %d the payload carries, keeping the least healthy and the slowest",
			len(out), maxCSEInvocationsPerScope)
		out = retainMostRelevant(out)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].OffsetMs != out[j].OffsetMs {
			return out[i].OffsetMs < out[j].OffsetMs
		}
		return out[i].CSEID < out[j].CSEID
	})
	return out, omitted
}

// retainMostRelevant cuts an over-long list to maxCSEInvocationsPerScope: non-success
// outcomes first, then the longest durations. It must run before the caller's
// chronological sort - sort-then-truncate would drop whatever ran late in the pass.
func retainMostRelevant(invocations []CSEInvocation) []CSEInvocation {
	ranked := make([]CSEInvocation, len(invocations))
	copy(ranked, invocations)
	sort.SliceStable(ranked, func(i, j int) bool {
		iOK, jOK := ranked[i].Result == cseResultSuccess, ranked[j].Result == cseResultSuccess
		if iOK != jOK {
			return jOK
		}
		if ranked[i].DurationMs != ranked[j].DurationMs {
			return ranked[i].DurationMs > ranked[j].DurationMs
		}
		return ranked[i].CSEID < ranked[j].CSEID
	})
	return ranked[:maxCSEInvocationsPerScope]
}

// gpoRefs resolves an invocation's GPO references against the shared name lookup.
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

// gpoRefsFromList extracts the GPO references carried by a 4016 event's
// ApplicableGPOList: the IDs in document order and the display names recovered
// alongside them. TDH hands the property over as one opaque string holding a
// rootless <GPO ID="{GUID}"><Name>…</Name></GPO> sequence.
//
// The braced-GUID scan is the fallback for a fragment the walk could not
// finish. It runs whenever the walk ended early rather than only when it found
// nothing, because a malformation mid-list leaves every entry behind it
// unparsed. Only ever call it with ApplicableGPOList — the scan reports every
// braced GUID it sees.
func gpoRefsFromList(raw string) (ids []string, names map[string]string, omitted int) {
	if raw == "" {
		return nil, nil, 0
	}

	var refs gpoCollector
	complete := forEachGPOElement(raw, refs.add)
	if len(refs.ids) > 0 && complete {
		return refs.ids, refs.names, refs.omitted
	}

	// Appending keeps document order; the collector drops what the walk already found.
	for rest := raw; ; {
		loc := bracedGUIDPattern.FindStringIndex(rest)
		if loc == nil {
			break
		}
		if _, normalized, ok := normalizeGUID(rest[loc[0]:loc[1]]); ok {
			refs.add(normalized, "")
		}
		rest = rest[loc[1]:]
	}
	return refs.ids, refs.names, refs.omitted
}

// gpoCollector accumulates one invocation's GPO references: the first maxGPOsPerCSE
// distinct IDs in document order, their names, and a count of the rest. The seen set
// keeps that count exact - a repeated GUID past the cap is not a fresh loss.
type gpoCollector struct {
	ids     []string
	names   map[string]string
	seen    map[string]struct{}
	omitted int
}

// add records one reference: an empty ID is not one, and a repeat is not a loss.
func (c *gpoCollector) add(id, name string) {
	if id == "" {
		return
	}
	if _, dup := c.seen[id]; dup {
		return
	}
	if c.seen == nil {
		c.seen = make(map[string]struct{})
	}
	c.seen[id] = struct{}{}

	if len(c.ids) >= maxGPOsPerCSE {
		c.omitted++
		return
	}
	c.ids = append(c.ids, id)
	if name == "" {
		return
	}
	if c.names == nil {
		c.names = make(map[string]string)
	}
	c.names[id] = name
}

// gpoXML is the decoded body of one <GPO> element; its ID attribute is read separately.
type gpoXML struct {
	Name string `xml:"Name"`
}

// forEachGPOElement walks a rootless <GPO> list fragment, calling fn once per entry
// with a usable GUID, and reports whether it reached the end - the caller cannot
// otherwise tell a complete list from a prefix. A token loop is used because
// xml.Unmarshal would stop after the first entry, and Strict is off because the
// provider does not escape display names: a GPO named "R&D Baseline" arrives
// carrying a bare ampersand.
func forEachGPOElement(raw string, fn func(id, name string)) bool {
	decoder := xml.NewDecoder(strings.NewReader(raw))
	decoder.Strict = false
	for {
		token, err := decoder.Token()
		if err != nil {
			// io.EOF is the only clean end of input, and the error is sticky.
			return errors.Is(err, io.EOF)
		}
		if token == nil {
			return true
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "GPO" {
			// No root element, so a wrapper may enclose the entries; Skip would eat them.
			continue
		}

		var entry gpoXML
		if err := decoder.DecodeElement(&entry, &start); err != nil {
			return false
		}
		if _, id, ok := normalizeGUID(gpoIDAttr(start)); ok {
			fn(id, truncateProviderText(entry.Name, maxGPONameBytes))
		}
	}
}

// gpoIDAttr reads a <GPO> element's ID attribute, case-insensitively.
func gpoIDAttr(start xml.StartElement) string {
	for _, attr := range start.Attr {
		if strings.EqualFold(attr.Name.Local, "id") {
			return attr.Value
		}
	}
	return ""
}

// normalizeGUID parses a GUID from provider text, returning its canonical braced
// uppercase form too. Braces are added because windows.GUIDFromString requires them.
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

// parseETWBool interprets a formatted win:Boolean. Numeric spellings are accepted:
// the provider types the same field win:Boolean on one version, win:UInt32 on another.
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

// truncateProviderText bounds a provider string, reserving room for the ellipsis.
func truncateProviderText(s string, maxBytes int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxBytes {
		return s
	}
	const ellipsis = "..."
	if maxBytes <= len(ellipsis) {
		// Subtracting the ellipsis would hand TruncateUTF8 a negative limit, which panics.
		return pkgstrings.TruncateUTF8(s, maxBytes)
	}
	return pkgstrings.TruncateUTF8(s, maxBytes-len(ellipsis)) + ellipsis
}
