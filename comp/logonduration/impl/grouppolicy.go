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

// This file defines the group_policy_details block of the logon duration
// payload: the client-side-extension (CSE) invocations observed during each
// Group Policy pass, and the Group Policy objects that fed each one.
//
// Windows times CSE invocations but not GPOs. Event 4016 starts an invocation
// and 5016/6016/7016 end it, giving a real measured interval. The GPOs feeding
// an invocation come from that same 4016 and carry no timing fields whatsoever,
// so a GPO is emitted as an untimed reference and never inherits a CSE's
// duration.

// Bounds on what one payload may carry. They are fixed at compile time rather
// than made configurable because an oversize event fails silently and the Agent
// can never learn that a larger bound was a mistake: this payload rides the
// event-management pipeline, which selects the stream strategy and so enforces no
// size of its own - the 5 MB that pipeline configures reaches the batch strategy
// only - and the intake's refusal arrives as an HTTP 413 that the logs library
// treats as a non-retryable drop, long after SendEventPlatformEventBlocking has
// already returned nil. So these are what stands between a large domain and a
// silently discarded event, and they are deliberately independent of the
// intake-side limit rather than sized to just fit inside it.
//
// The four are coupled through one byte budget rather than independent, and
// TestSubmitEvent_WorstCasePayloadSize computes that budget from them: raising
// any one fails there rather than quietly multiplying the event.
const (
	// maxCSEInvocationsPerScope is a backstop, not a working limit. A registered
	// extension is invoked at most once per pass per scope and Windows registers on
	// the order of 57 of them, so this cannot fire on a machine whose extension
	// list is sane. It exists because nothing else bounds the invocation count
	// while the byte budget above assumes it is bounded, and enforcing what the
	// budget already assumes costs nothing.
	maxCSEInvocationsPerScope = 64

	// maxGPOsPerCSE bounds the GPO references carried by one invocation. GPOs are
	// inlined per invocation rather than pooled in a shared table, so one
	// invocation's list is repeated in full for every extension that applied the
	// same objects; this is what keeps that multiplication bounded.
	//
	// It sits well below what a large domain can produce, deliberately. A GPORef
	// carries no timing - no Windows event reports a duration for an individual
	// GPO - so the fortieth reference costs the same bytes as the first while
	// answering nothing the first has not already answered. What the cap drops is
	// reported in GPOsOmitted rather than left to be inferred, which is what makes
	// a truncated list distinguishable from a complete one of the same length.
	maxGPOsPerCSE = 32

	// Provider-supplied text is never emitted unbounded. Both are byte limits and
	// both are sized for the worst UTF-8 case rather than the ASCII one: Active
	// Directory permits a GPO displayName of up to 256 characters, and a byte limit
	// set at the character limit would truncate ordinary names in non-Latin domains
	// while never firing in Latin ones. 512 carries a name of the full 256
	// characters in any two-byte script and about 170 in a three-byte one, which is
	// far longer than display names actually run.
	maxCSENameBytes = 128
	maxGPONameBytes = 512
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
//
// A populated array implies its parent milestone: a scope is only ever claimed
// by a pass start event, which is the same event that sets the milestone's
// timestamp. So there is no shape in which this block describes slices of a
// pass boot_timeline does not report.
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

	// Result is the outcome, taken from which terminal event closed the
	// invocation. The provider's own ErrorCode is deliberately not carried
	// alongside it: this block reports what ran and for how long, and the status
	// behind a failure is a Group Policy health question answerable from the
	// host's Group Policy Operational log.
	Result cseResult `json:"result"`

	// Async reports the extension's IsExtensionAsyncProcessing flag. When set,
	// the terminal event marks the dispatch of a worker thread, so DurationMs is
	// the cost of the dispatch rather than of the extension's work.
	Async bool `json:"async,omitempty"`

	GPOs []GPORef `json:"gpos,omitempty"`

	// GPOsOmitted counts the distinct GPO references maxGPOsPerCSE dropped from
	// GPOs, present only when the cap actually fired. Without it a capped array is
	// indistinguishable from a machine that genuinely applies exactly that many
	// objects to one extension, which makes an incomplete list unfalsifiable rather
	// than merely incomplete. It costs nothing on the wire when nothing was
	// dropped, which is the whole of the case for carrying it.
	GPOsOmitted int `json:"gpos_omitted,omitempty"`
}

// GPORef identifies one Group Policy object that fed an invocation. It carries
// no timing: no Windows event reports a duration for an individual GPO.
type GPORef struct {
	// ID is the GPO GUID in canonical braced uppercase form, and is the
	// identity. Two GPOs sharing a display name remain distinct.
	ID string `json:"id"`
	// Name is absent unless an ApplicableGPOList supplied it, which a list the
	// parser could not walk to the end may not.
	Name string `json:"name,omitempty"`
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

// observedCSEStop is the decoded content of a 5016/6016/7016 extension-stop
// event. The three share an identical template.
type observedCSEStop struct {
	eventID    uint16
	guid       windows.GUID
	guidString string
	name       string
}

// cseRecord is a completed invocation held until finalize, which is where its
// scope is decided. It deliberately carries no scope of its own.
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

// gpAccumulator collects Group Policy observations across one ETL trace. Its
// maps are not guarded: ProcessETLFile drives one trace handle, so callbacks
// arrive serially on the calling thread.
type gpAccumulator struct {
	// passActivity is the activity ID of the boot pass for each scope, taken
	// from the pass start event that names it: 4000 for computer, 4001 for user.
	// Only the starts seed it, so a scope cannot claim invocations unless the
	// same event also set its milestone.
	//
	// First-write-wins per scope mirrors MachineGPStart/UserGPStart in
	// groupPolicyParser.Parse, so the invocations collected here always belong
	// to the pass boot_timeline reports.
	passActivity [gpScopeCount]windows.GUID
	passPinned   [gpScopeCount]bool

	open map[cseKey]*openCSE
	// done holds completed invocations keyed by the activity ID that produced
	// them, and is bucketed into scopes at finalize. Keying on the activity is
	// what lets buildScope exclude a gpupdate or a periodic refresh by simply
	// never visiting its key, rather than by testing each record against a rule.
	done map[windows.GUID][]cseRecord

	// gpoNames maps a GPO GUID to its display name, harvested from every 4016
	// ApplicableGPOList in the trace. It is a lookup, not an inventory: an entry
	// only reaches the wire if a surviving invocation references it. Sharing it
	// across invocations is what supplies names for a list whose walk ended
	// early, since the GUID is recovered by the fallback scan but the name is
	// not.
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
// The zero GUID is refused. Events outside any pass carry it - a real boot
// carried 16 such events, including both occurrences of 4117, which fires on the
// same thread that goes on to emit 4000 or 4001 - so accepting it would pin zero
// for a scope and sweep every unattributed invocation into that pass.
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
	// Two scopes sharing one activity would make buildScope read the same bucket
	// twice and emit every invocation under both. Real passes each get their own
	// ID, so this only refuses input that was never going to be interpretable.
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
		// A second start for a key already open cannot be disambiguated. The
		// later one wins: it yields the shorter, more conservative interval when
		// the terminal event arrives, and the earlier one is discarded rather
		// than emitted as a record nobody can interpret.
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

	// Validate before consuming the start, so a stop that cannot close this
	// invocation leaves it open for one that can.
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

// mergeGPONames records the display names recovered from one ApplicableGPOList.
// First name wins, so a later list cannot rename a GPO.
func (a *gpAccumulator) mergeGPONames(names map[string]string) {
	for id, name := range names {
		if _, ok := a.gpoNames[id]; !ok {
			a.gpoNames[id] = name
		}
	}
}

// finalize assembles the emitted block, returning nil when no invocation was
// measured end to end.
func (a *gpAccumulator) finalize(tl BootTimeline) *GroupPolicyDetails {
	// Anything still in a.open has no measured interval and is simply never
	// read: only done feeds the output. That is the normal tail case - the
	// capture window closes when the Agent service starts - and not an error.

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

	if len(out) > maxCSEInvocationsPerScope {
		log.Warnf("Logon duration: %d Group Policy extension invocations in one pass exceeds the %d the payload carries, keeping the least healthy and the slowest",
			len(out), maxCSEInvocationsPerScope)
		out = retainMostRelevant(out)
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

// retainMostRelevant cuts an implausibly long invocation list down to
// maxCSEInvocationsPerScope, keeping what a reader would ask for first: every
// outcome that was not a plain success, then the longest durations.
//
// Selection is by value and not by position because the caller orders the result
// chronologically. Truncating that order instead would drop whichever extensions
// happened to run late in the pass, which on a pass slow enough to reach this cap
// is as likely as not to be the slow one the payload exists to identify.
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

// gpoRefs resolves an invocation's GPO references against the name lookup. It
// runs at finalize so a name recovered from a later invocation's list is still
// available to an earlier one that could not supply its own.
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

// gpoRefsFromList extracts the Group Policy object references carried by a 4016
// event's ApplicableGPOList: the IDs in document order, and the display names
// the same walk recovered alongside them.
//
// The property is declared win:UnicodeString rather than a TDH array, so TDH
// hands it over as one opaque string. A boot capture confirms the runtime format
// is a rootless <GPO ID="{GUID}"><Name>…</Name></GPO> sequence carrying ID and
// Name only, so the IDs come from the attributes and the names come free.
//
// The braced-GUID scan is the fallback for a fragment the walk could not finish,
// and it also recovers references from a delimited or prose value. It runs
// whenever the walk ended early, not only when the walk found nothing: a
// malformation in the middle of a list leaves every entry behind it unparsed, so
// trusting a prefix would drop the tail of the list outright. The recovered tail
// carries IDs without names, which is the right way round - the GUID is the
// identity - and the accumulator's shared lookup may still supply a name from
// another invocation that listed the same object.
//
// It must only ever be called with ApplicableGPOList. The scan reports every
// braced GUID it sees, and 5312's GPOInfoList embeds an
// <Extensions>[{CSE GUID}]</Extensions> element in each of its entries, which
// the scan would report as applicable GPOs. Not collecting 5312 at all is what
// makes that structural rather than a rule to remember.
//
// Both tiers walk the whole value rather than stopping at maxGPOsPerCSE, and the
// input carries no size limit of its own. The parse is a single linear pass over
// a string TDH has already materialized, so stopping early would save little; it
// would cost the omitted count its meaning, since a reference cannot be known to
// be new without having looked at the ones behind it. Rejecting an oversize value
// outright would be worse still: it emits an invocation with no GPOs at all,
// which reads as an extension that applied none - the precise opposite of the
// truth.
func gpoRefsFromList(raw string) (ids []string, names map[string]string, omitted int) {
	if raw == "" {
		return nil, nil, 0
	}

	var refs gpoCollector
	complete := forEachGPOElement(raw, refs.add)
	if len(refs.ids) > 0 && complete {
		return refs.ids, refs.names, refs.omitted
	}

	// Appending keeps document order, and the collector drops what the walk
	// already found, so a partial walk contributes its names and the scan
	// contributes the IDs it could not reach.
	//
	// The scan advances match by match rather than calling FindAllString, which
	// would materialize every match in the value before the cap could discard all
	// but maxGPOsPerCSE of them.
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

// gpoCollector accumulates the GPO references of one invocation: the first
// maxGPOsPerCSE distinct IDs in document order, the display names recovered
// alongside them, and a count of the distinct references it had to leave behind.
//
// The seen set is what makes that count exact rather than an estimate of it. A
// repeated GUID is not a loss and must not be counted as one, so establishing
// that a reference beyond the cap is new means comparing it against every
// reference already observed, not only against the ones that were kept. It is
// transient, freed with the event, and strictly smaller than the property string
// the caller already holds.
type gpoCollector struct {
	ids     []string
	names   map[string]string
	seen    map[string]struct{}
	omitted int
}

// add records one reference. An empty ID is not a reference, a repeat is not a
// loss, and only an ID that was kept is given a name - which is what bounds the
// names by the same cap as the IDs.
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

// gpoXML is the decoded body of one <GPO> element. The GUID lives in an
// attribute and is read separately, case-insensitively.
type gpoXML struct {
	Name string `xml:"Name"`
}

// forEachGPOElement walks a <GPO> list fragment, calling fn once per entry that
// carries a usable GUID, and reports whether it reached the end of the input.
//
// The fragment has no root element - a bare sequence of sibling
// <GPO ID="{GUID}"> entries, as confirmed by a boot capture - so xml.Unmarshal
// would stop after the first one and a token loop is used instead. A rooted
// variant parses too, and every path yields fewer entries rather than guessing.
//
// Strict parsing is off because the provider concatenates these fragments
// without escaping display names: a GPO named "R&D Baseline" arrives carrying a
// bare ampersand, which is not well-formed XML. A boot capture carried exactly
// that. Strictly, DecodeElement fails on the offending entry and the walk
// abandons every entry behind it, so one such name costs the names of all the
// GPOs that follow it.
//
// Non-strict mode leaves malformed entities alone and invents missing end tags,
// but it does not relax tag syntax: a literal "<" in a display name still ends
// the walk, and so does a mismatched end tag. That is what the return value is
// for - the caller cannot tell a complete list from a prefix without it.
//
// An entry with no resolvable GUID is skipped: the GUID is the identity, and a
// display name is neither unique nor stable enough to stand in for it.
func forEachGPOElement(raw string, fn func(id, name string)) bool {
	decoder := xml.NewDecoder(strings.NewReader(raw))
	decoder.Strict = false
	for {
		token, err := decoder.Token()
		if err != nil {
			// io.EOF is the only clean end of input. Any other error means a
			// malformed remainder whose entries were never seen, and the decoder's
			// error is sticky, so the walk cannot resume past it.
			return errors.Is(err, io.EOF)
		}
		if token == nil {
			return true
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "GPO" {
			// Keep walking rather than skipping the subtree: the fragment has no
			// root, so a wrapper element may enclose the entries we want.
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

// gpoIDAttr reads a <GPO> element's ID attribute. A boot capture shows the
// provider emits uppercase "ID"; the match stays case-insensitive because that
// casing is not part of any documented contract.
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

// parseETWBool interprets a formatted win:Boolean property. TDH renders these as
// "true"/"false", and IsExtensionAsyncProcessing - the only such property read
// here - is declared win:Boolean on the single version of 4016 the provider
// ships, so that is what it will be.
//
// The numeric spellings are accepted anyway, because the provider does declare
// the same field name with two different types elsewhere: the sibling IsMachine
// is win:Boolean on version 0 of 4000/4001/8000/8001 and win:UInt32 on version 1,
// so it renders "true"/"false" or "1"/"0" depending on which version a build
// emits. Nothing guarantees 4016 will not gain a version with the same skew.
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
	if maxBytes <= len(ellipsis) {
		// No room for the marker, and TruncateUTF8 would be handed a negative
		// limit. Every caller passes a constant far above this.
		return pkgstrings.TruncateUTF8(s, maxBytes)
	}
	return pkgstrings.TruncateUTF8(s, maxBytes-len(ellipsis)) + ellipsis
}
