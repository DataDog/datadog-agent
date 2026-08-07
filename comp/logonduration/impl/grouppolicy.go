// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package logondurationimpl

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	pkgstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
)

// This file defines the group_policy block of the logon duration payload: the
// client-side-extension (CSE) invocations observed during each Group Policy
// pass, and the Group Policy Objects applicable to them.
//
// Windows times CSE invocations but not GPOs. Event 4016 starts an invocation
// and 5016/6016/7016 end it, giving a real measured interval. Event 5312
// enumerates applicable GPOs and carries no timing fields whatsoever, so GPOs
// are emitted as untimed metadata and never inherit a CSE's duration.
//
// The GPO-to-CSE relationship is many-to-many: 4016 carries a *list* of
// applicable GPOs, and one GPO feeds many invocations. That is modelled by
// repeating GPO GUIDs in each invocation's ApplicableGPOIDs while the metadata
// lives once in the payload's GPOs table.

// timestampFormat is the wire format for every timestamp in the payload.
//
// It is fixed-width and always UTC, so lexicographic ordering of two formatted
// values matches chronological ordering. sortCSEInvocations relies on that.
const timestampFormat = "2006-01-02T15:04:05.000Z"

// groupPolicyPayloadVersion versions the group_policy block as a unit. Bump it
// for any rename, removal, or change in meaning; additive fields do not need it.
const groupPolicyPayloadVersion = 1

// Bounds on what a single payload may carry. These are fixed at compile time
// rather than made configurable on purpose: an oversize event is rejected by
// the intake with HTTP 413, which the logs library treats as non-retryable long
// after SendEventPlatformEventBlocking has already returned nil, so the Agent
// can never learn that a larger bound was a mistake. They are deliberately
// independent of any intake-side limit.
const (
	// maxCSEInvocationsPerPass bounds each pass separately rather than the
	// payload as a whole, so a noisy computer pass cannot starve the user pass
	// out of the payload. Windows registers roughly 30 CSEs per scope.
	maxCSEInvocationsPerPass = 64

	// maxGPOsTotal bounds the shared GPO metadata table, which is deduplicated
	// by GUID and so grows with distinct GPOs rather than with references.
	maxGPOsTotal = 400

	// maxGPOListBytes rejects an implausibly large GPOInfoList/ApplicableGPOList
	// before any parsing work happens.
	maxGPOListBytes = 256 * 1024

	// maxGPOsPerList bounds the GPOs taken from a single event.
	maxGPOsPerList = 200

	// maxApplicableGPOIDsPerCSE bounds the GPO references on one invocation.
	maxApplicableGPOIDsPerCSE = 64

	// Provider-supplied text is never emitted unbounded.
	maxCSENameBytes = 128
	maxGPONameBytes = 256
	maxGPOSOMBytes  = 512
	maxGPOVersion   = 32
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
	// cseResultDispatched is an asynchronous extension whose terminal event
	// marks the dispatch of a worker thread rather than the completion of the
	// extension's work. Such an invocation never reports a duration.
	cseResultDispatched cseResult = "dispatched"
	// cseResultUnknown is an invocation with no terminal event observed,
	// typically because the trace ended first.
	cseResultUnknown cseResult = "unknown"
)

// gpScope is the Group Policy processing scope. It is internal only: the wire
// format expresses scope structurally, through passes.computer / passes.user.
type gpScope int

const (
	gpScopeUnknown gpScope = iota
	gpScopeComputer
	gpScopeUser
	gpScopeCount
)

// scopeForActivityStart maps a Group Policy activity-start event ID to its
// scope. Events 4000-4007 alternate strictly by parity - even is computer, odd
// is user - across boot (4000/4001), network state change (4002/4003), manual
// gpupdate (4004/4005), and periodic refresh (4006/4007).
//
// This is used in preference to the IsMachine payload field, which is declared
// win:Boolean in one manifest version and win:UInt32 in another on the same
// Windows build and so cannot be decoded reliably.
func scopeForActivityStart(id uint16) (gpScope, bool) {
	if id < evtMachineGPStart || id > evtGPActivityStartMax {
		return gpScopeUnknown, false
	}
	if id%2 == 0 {
		return gpScopeComputer, true
	}
	return gpScopeUser, true
}

// GroupPolicyPayload is the group_policy block of the event's custom payload.
type GroupPolicyPayload struct {
	Version int      `json:"version"`
	Passes  GPPasses `json:"passes"`

	// GPOs is a metadata dictionary: one entry per distinct GPO, referenced by
	// GUID from each invocation's applicable_gpo_ids. Nothing about a GPO varies
	// per invocation or per pass, so the metadata is stored once here rather
	// than repeated on every invocation that references it.
	GPOs []GPO `json:"gpos"`

	// Unattributed holds invocations whose activity ID matched no observed
	// activity-start event, leaving their scope unknown. They are emitted here
	// rather than dropped or guessed into a pass.
	Unattributed []CSEInvocation `json:"unattributed_cse_invocations,omitempty"`

	GPOsTruncated int `json:"gpos_truncated,omitempty"`
	ParseErrors   int `json:"parse_errors,omitempty"`
}

// GPPasses is a closed set of the two Group Policy scopes. Both fields are
// always emitted so a consumer can never confuse "this pass ran and invoked no
// extensions" with "this pass was never observed".
type GPPasses struct {
	Computer GPPass `json:"computer"`
	User     GPPass `json:"user"`
}

// GPPass holds the CSE invocations observed during one Group Policy pass.
type GPPass struct {
	// Observed reports whether an activity-start event for this scope was seen.
	// It is the coverage signal: false means the trace ended before the pass
	// began, which is common because the ETL capture window closes when the
	// Agent service starts.
	//
	// The pass's start, end, and duration are deliberately not repeated here.
	// They are already the computer_group_policy / user_group_policy entries in
	// boot_timeline and durations, and a second copy could drift from them.
	Observed bool `json:"observed"`

	CSEInvocations []CSEInvocation `json:"cse_invocations"`

	IncompleteInvocations   int `json:"incomplete_invocations,omitempty"`
	CSEInvocationsTruncated int `json:"cse_invocations_truncated,omitempty"`
}

// CSEInvocation is one observed client-side-extension invocation. It carries no
// scope field: the enclosing pass determines the scope.
type CSEInvocation struct {
	// CSEGUID is the extension's identity in canonical braced uppercase form,
	// the same form used for gpos[].id.
	CSEGUID string `json:"cse_guid"`
	CSEName string `json:"cse_name,omitempty"`

	Start string `json:"start,omitempty"`
	End   string `json:"end,omitempty"`

	// DurationMs is the wall-clock interval between the start and terminal
	// events. It is a pointer because an extension completing in under a
	// millisecond has a real duration of zero, which is not the same as not
	// having been measured. It is absent for asynchronous and unmatched
	// invocations, which have no measured interval.
	DurationMs *int64 `json:"duration_ms,omitempty"`

	// ReportedElapsedMs is the provider's own CSEElaspedTimeInMilliSeconds. It
	// is kept distinct from DurationMs rather than reconciled with it: the two
	// have never been compared against real traces, and treating them as
	// interchangeable would assert something unverified.
	ReportedElapsedMs *uint32 `json:"reported_elapsed_ms,omitempty"`

	Result cseResult `json:"result"`

	// ErrorCode is the provider's raw status, emitted as the hexadecimal string
	// Windows itself formats it as. A JSON number would round-trip values above
	// 0x7FFFFFFF as either a large unsigned or a negative signed integer.
	ErrorCode *string `json:"error_code,omitempty"`

	// IsAsync reports the extension's IsExtensionAsyncProcessing flag. When set,
	// the terminal event marks the dispatch of asynchronous work, not its
	// completion, and a non-zero ErrorCode is not a failure.
	IsAsync bool `json:"is_async"`

	// Complete is true only for a synchronous invocation whose start and
	// terminal events were both observed.
	Complete bool `json:"complete"`

	// MissingStart marks a terminal event observed with no matching start,
	// typically because the trace began mid-pass.
	MissingStart bool `json:"missing_start,omitempty"`

	ApplicableGPOIDs []string `json:"applicable_gpo_ids,omitempty"`
}

// GPO is the metadata for one Group Policy Object. It carries no timing: no
// Windows event reports a duration for an individual GPO.
type GPO struct {
	// ID is the GPO GUID in canonical braced uppercase form, and is the
	// identity. Two GPOs sharing a display name remain distinct entries.
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	SOM     string `json:"som,omitempty"`
	Version string `json:"version,omitempty"`
}

// observedCSEStart is the decoded content of a 4016 extension-start event.
type observedCSEStart struct {
	guid             windows.GUID
	guidString       string
	name             string
	isAsync          bool
	applicableGPOIDs []string
}

// observedCSEStop is the decoded content of a 5016/6016/7016 extension-stop
// event. The three share an identical template.
type observedCSEStop struct {
	eventID    uint16
	guid       windows.GUID
	guidString string
	name       string
	elapsedMs  uint32
	hasElapsed bool
	errorCode  string
}

// openCSE is an invocation whose start was observed and which is awaiting a
// terminal event.
type openCSE struct {
	inv   *CSEInvocation
	scope gpScope
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
	// scopes maps an activity ID to the scope of its activity-start event.
	scopes map[windows.GUID]gpScope
	// lastScope is the fallback when an event carries no usable activity ID.
	lastScope gpScope
	// observed records whether a boot activity-start event was seen per scope.
	observed [gpScopeCount]bool

	open map[cseKey]*openCSE
	// done holds completed invocations bucketed by scope at insert time, so
	// finalize builds each pass directly with no post-hoc partition.
	done [gpScopeCount][]*CSEInvocation

	gpos map[string]*GPO

	parseErrors int
	// seenAny reports whether any Group Policy event was observed at all. When
	// false the payload omits the group_policy block entirely.
	seenAny bool
}

func newGPAccumulator() *gpAccumulator {
	return &gpAccumulator{
		scopes: make(map[windows.GUID]gpScope),
		open:   make(map[cseKey]*openCSE),
		gpos:   make(map[string]*GPO),
	}
}

// noteActivityStart records the scope of a Group Policy processing instance.
//
// Only the boot activity starts (4000 and 4001) flip the observed flag. A
// mid-trace gpupdate or periodic refresh seeds the scope so its invocations can
// be attributed, but it is not the boot pass that boot_timeline reports.
func (a *gpAccumulator) noteActivityStart(activityID windows.GUID, id uint16) {
	scope, ok := scopeForActivityStart(id)
	if !ok {
		return
	}
	a.seenAny = true
	a.scopes[activityID] = scope
	a.lastScope = scope
	if id == evtMachineGPStart || id == evtUserGPStart {
		a.observed[scope] = true
	}
}

// scopeFor resolves the scope of an event by its activity ID, falling back to
// the most recent activity start when the activity ID is unavailable. The
// fallback exists because EVENT_HEADER.ActivityId is only reachable on real ETW
// events; it is correct as long as passes of the same scope do not overlap.
func (a *gpAccumulator) scopeFor(activityID windows.GUID) gpScope {
	if scope, ok := a.scopes[activityID]; ok {
		return scope
	}
	return a.lastScope
}

// startCSE records a 4016 extension-start observation.
//
// A second start for a key that is already open cannot be disambiguated, so the
// existing invocation is closed as incomplete rather than letting a later
// terminal event attach to the wrong one.
func (a *gpAccumulator) startCSE(activityID windows.GUID, o observedCSEStart, ts time.Time) {
	a.seenAny = true
	key := cseKey{activity: activityID, cse: o.guid}
	if prev, ok := a.open[key]; ok {
		a.closeOpen(prev)
	}

	scope := a.scopeFor(activityID)
	inv := &CSEInvocation{
		CSEGUID:          o.guidString,
		CSEName:          truncateProviderText(o.name, maxCSENameBytes),
		Start:            ts.UTC().Format(timestampFormat),
		Result:           cseResultUnknown,
		IsAsync:          o.isAsync,
		ApplicableGPOIDs: o.applicableGPOIDs,
	}
	a.open[key] = &openCSE{inv: inv, scope: scope, start: ts}
}

// finishCSE records a 5016/6016/7016 extension-stop observation.
//
// A terminal event with no matching start is emitted as its own invocation
// rather than dropped: the elapsed time the provider reports is real data even
// though the interval is not observable.
func (a *gpAccumulator) finishCSE(activityID windows.GUID, o observedCSEStop, ts time.Time) {
	a.seenAny = true
	key := cseKey{activity: activityID, cse: o.guid}

	open, ok := a.open[key]
	if !ok {
		inv := &CSEInvocation{
			CSEGUID:      o.guidString,
			CSEName:      truncateProviderText(o.name, maxCSENameBytes),
			End:          ts.UTC().Format(timestampFormat),
			Result:       resultForStopEvent(o.eventID),
			MissingStart: true,
		}
		applyStopDetails(inv, o)
		scope := a.scopeFor(activityID)
		a.done[scope] = append(a.done[scope], inv)
		return
	}
	delete(a.open, key)

	inv := open.inv
	inv.End = ts.UTC().Format(timestampFormat)
	applyStopDetails(inv, o)

	if inv.IsAsync {
		// The terminal event marks thread dispatch, not completion, so there is
		// no total extension duration to report. Microsoft documents the audit
		// extension returning E_PENDING here by design, so the error code is
		// recorded but is not treated as a failure.
		inv.Result = cseResultDispatched
	} else {
		inv.Result = resultForStopEvent(o.eventID)
		if !open.start.IsZero() && !ts.Before(open.start) {
			ms := ts.Sub(open.start).Milliseconds()
			inv.DurationMs = &ms
			inv.Complete = true
		}
	}

	a.done[open.scope] = append(a.done[open.scope], inv)
}

// applyStopDetails copies the provider-reported fields off a terminal event.
func applyStopDetails(inv *CSEInvocation, o observedCSEStop) {
	if o.hasElapsed {
		elapsed := o.elapsedMs
		inv.ReportedElapsedMs = &elapsed
	}
	if o.errorCode != "" {
		code := o.errorCode
		inv.ErrorCode = &code
	}
	if inv.CSEName == "" {
		inv.CSEName = truncateProviderText(o.name, maxCSENameBytes)
	}
}

// resultForStopEvent maps a terminal event ID to its outcome. The three events
// share an identical template and differ only in severity.
func resultForStopEvent(id uint16) cseResult {
	switch id {
	case evtCSEStopSuccess:
		return cseResultSuccess
	case evtCSEStopWarning:
		return cseResultWarning
	case evtCSEStopError:
		return cseResultError
	default:
		return cseResultUnknown
	}
}

// closeOpen moves an invocation with no terminal event into its scope's bucket.
func (a *gpAccumulator) closeOpen(open *openCSE) {
	open.inv.Result = cseResultUnknown
	open.inv.Complete = false
	a.done[open.scope] = append(a.done[open.scope], open.inv)
}

// addGPOs merges parsed GPO metadata into the shared table, keyed on GUID.
// Entries seen only as a reference from an invocation's applicable list arrive
// here with no metadata and are stored as an ID alone.
func (a *gpAccumulator) addGPOs(gpos []GPO) {
	a.seenAny = true
	for _, g := range gpos {
		if g.ID == "" {
			continue
		}
		existing, ok := a.gpos[g.ID]
		if !ok {
			entry := g
			a.gpos[g.ID] = &entry
			continue
		}
		if g.Name != "" {
			existing.Name = g.Name
		}
		if g.SOM != "" {
			existing.SOM = g.SOM
		}
		if g.Version != "" {
			existing.Version = g.Version
		}
	}
}

// addGPOIDs registers GPOs known only by GUID, so a reference from an
// invocation's applicable list always resolves against the payload's GPO table
// even when no inventory event supplied the metadata. Existing entries keep
// whatever metadata they already have.
func (a *gpAccumulator) addGPOIDs(ids []string) {
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := a.gpos[id]; !ok {
			a.gpos[id] = &GPO{ID: id}
		}
	}
}

// finalize drains any invocation still awaiting a terminal event, then sorts,
// truncates, and assembles the emitted payload. It returns nil when no Group
// Policy event was observed at all.
func (a *gpAccumulator) finalize() *GroupPolicyPayload {
	if !a.seenAny {
		return nil
	}

	// An invocation still open when the trace ends was genuinely observed; it
	// simply has no terminal event. Report it rather than dropping it.
	for _, key := range sortedCSEKeys(a.open) {
		a.closeOpen(a.open[key])
	}
	a.open = make(map[cseKey]*openCSE)

	payload := &GroupPolicyPayload{
		Version:     groupPolicyPayloadVersion,
		ParseErrors: a.parseErrors,
	}
	payload.Passes.Computer = a.buildPass(gpScopeComputer)
	payload.Passes.User = a.buildPass(gpScopeUser)

	unattributed, _, _ := finalizeInvocations(a.done[gpScopeUnknown])
	payload.Unattributed = unattributed

	payload.GPOs, payload.GPOsTruncated = a.buildGPOs()
	return payload
}

func (a *gpAccumulator) buildPass(scope gpScope) GPPass {
	invocations, incomplete, truncated := finalizeInvocations(a.done[scope])
	return GPPass{
		Observed:                a.observed[scope],
		CSEInvocations:          invocations,
		IncompleteInvocations:   incomplete,
		CSEInvocationsTruncated: truncated,
	}
}

// finalizeInvocations sorts one bucket into a deterministic order, applies the
// per-pass bound, and reports how many invocations were incomplete and how many
// the bound dropped. The returned slice is never nil, so it serializes as an
// empty array rather than null.
func finalizeInvocations(src []*CSEInvocation) (out []CSEInvocation, incomplete, truncated int) {
	out = make([]CSEInvocation, 0, len(src))
	sortCSEInvocations(src)

	for _, inv := range src {
		if !inv.Complete {
			incomplete++
		}
	}
	if len(src) > maxCSEInvocationsPerPass {
		truncated = len(src) - maxCSEInvocationsPerPass
		src = src[:maxCSEInvocationsPerPass]
	}
	for _, inv := range src {
		out = append(out, *inv)
	}
	return out, incomplete, truncated
}

// sortCSEInvocations orders invocations chronologically, with untimed records
// last and the extension GUID breaking ties, so the emitted order and the set
// that survives truncation are both stable across runs.
//
// Start is compared as a string: timestampFormat is fixed-width UTC, so
// lexicographic and chronological order coincide.
func sortCSEInvocations(invs []*CSEInvocation) {
	sort.SliceStable(invs, func(i, j int) bool {
		a, b := invs[i], invs[j]
		if (a.Start == "") != (b.Start == "") {
			return a.Start != ""
		}
		if a.Start != b.Start {
			return a.Start < b.Start
		}
		return a.CSEGUID < b.CSEGUID
	})
}

// buildGPOs flattens the GPO table into a GUID-ordered slice and applies the
// table bound.
func (a *gpAccumulator) buildGPOs() ([]GPO, int) {
	ids := make([]string, 0, len(a.gpos))
	for id := range a.gpos {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	truncated := 0
	if len(ids) > maxGPOsTotal {
		truncated = len(ids) - maxGPOsTotal
		ids = ids[:maxGPOsTotal]
	}

	out := make([]GPO, 0, len(ids))
	for _, id := range ids {
		out = append(out, *a.gpos[id])
	}
	return out, truncated
}

// sortedCSEKeys returns the open-invocation keys in a stable order so the
// drained records land in the same sequence on every run.
func sortedCSEKeys(open map[cseKey]*openCSE) []cseKey {
	keys := make([]cseKey, 0, len(open))
	for k := range open {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].activity != keys[j].activity {
			return keys[i].activity.String() < keys[j].activity.String()
		}
		return keys[i].cse.String() < keys[j].cse.String()
	})
	return keys
}

// normalizeGUID parses a GUID from provider text and returns it alongside its
// canonical braced uppercase rendering, which is the form used on the wire and
// the join key between an invocation's applicable_gpo_ids and gpos[].id.
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

// formatErrorCode validates a provider status value and renders it in a
// canonical form, so the payload never carries unparsed provider text and never
// varies with how TDH happened to format the value.
//
// Parsing must use base 0, not base 10: ErrorCode is declared with the
// win:HexInt32 out-type, so TDH formats it as "0x8000000A" and a base-10 parse
// would fail on every non-zero code.
func formatErrorCode(s string) (string, bool) {
	v, ok := parseUint32(s)
	if !ok {
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
// as "true"/"false", but the exact token has not been confirmed against a live
// event on every Windows build, so the numeric spellings are accepted too.
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
