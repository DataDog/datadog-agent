// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && test

package logondurationimpl

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// Property values in these tests are in the form TDH produces for each declared
// out-type, per the Microsoft-Windows-GroupPolicy manifest: a braced GUID for
// win:GUID, decimal for win:UInt32, and "true"/"false" for win:Boolean.
//
// ErrorCode is written as "0x…" here because it is declared with the
// win:HexInt32 out-type, but the manifest pins only the prefix, not the padding
// or the case, and a real capture rendered a zero code as plain "0". So the exact
// spelling is not a contract: parseUint32 uses base 0 and accepts every form,
// which TestFormatErrorCode covers directly.

const (
	cseRegistryGUID  = "{35378EAC-683F-11D2-A89A-00C04FBBCFA2}"
	cseFolderRedGUID = "{25537BA6-77A8-11D2-9B6C-0000F8080861}"
	cseAuditGUID     = "{F3CCC681-B74C-4060-9F26-CD84525DCA2A}"

	gpoDefaultDomainGUID = "{31B2F340-016D-11D2-945F-00C04FB984F9}"
	gpoDomainCtlGUID     = "{6AC1786C-016F-11D2-945F-00C04FB984F9}"
	gpoThirdGUID         = "{A1B2C3D4-1111-2222-3333-444455556666}"
)

var (
	gpTestBoot     = time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	gpTestActivity = windows.GUID{Data1: 0xD739F467}
	gpUserActivity = windows.GUID{Data1: 0xAB12CD34}
	gpOtherRun     = windows.GUID{Data1: 0x99887766}
)

// gpFixture drives synthetic Group Policy events through the real parser.
type gpFixture struct {
	t    *testing.T
	coll *collector
}

func newGPFixture(t *testing.T) *gpFixture {
	t.Helper()
	f := &gpFixture{t: t, coll: newCollector()}
	// Kernel-General 12 is what sets this in production. Without it every
	// offset collapses to zero and the placement assertions say nothing.
	f.coll.timeline.BootStart = gpTestBoot
	return f
}

// send dispatches one Group Policy event through processEvent, so every test
// exercises the real provider routing rather than calling a parser directly.
func (f *gpFixture) send(activity windows.GUID, id uint16, offset time.Duration, props ...property) {
	f.t.Helper()
	e := makeEvent(guidGroupPolicy, id, gpTestBoot.Add(offset), props...)
	e.activityID = activity
	processEvent(f.coll, e)
}

// sendPartial dispatches an event whose TDH decode failed after the properties
// given, which is what a partial bulk decode looks like: the properties before
// the failure are recovered and everything after it is absent.
func (f *gpFixture) sendPartial(activity windows.GUID, id uint16, offset time.Duration, failedAt string, props ...property) {
	f.t.Helper()
	e := makeEvent(guidGroupPolicy, id, gpTestBoot.Add(offset), props...)
	e.activityID = activity
	e.propsErr = fmt.Errorf("failed to parse property %q", failedAt)
	processEvent(f.coll, e)
}

// startComputerPass emits the computer-scope pass start (event 4000).
func (f *gpFixture) startComputerPass() {
	f.send(gpTestActivity, evtMachineGPStart, 12*time.Second)
}

// endComputerPass emits the computer-scope pass stop (event 8000).
func (f *gpFixture) endComputerPass() {
	f.send(gpTestActivity, evtMachineGPEnd, 20*time.Second)
}

// startUserPass emits the user-scope pass start (event 4001).
func (f *gpFixture) startUserPass() {
	f.send(gpUserActivity, evtUserGPStart, 30*time.Second)
}

// cseStart emits a 4016 extension start.
func (f *gpFixture) cseStart(activity windows.GUID, offset time.Duration, guid, name string, async bool, gpoList string) {
	props := []property{
		{Name: "CSEExtensionId", Value: guid},
		{Name: "CSEExtensionName", Value: name},
		{Name: "IsExtensionAsyncProcessing", Value: strconv.FormatBool(async)},
	}
	if gpoList != "" {
		props = append(props, property{Name: "ApplicableGPOList", Value: gpoList})
	}
	f.send(activity, evtCSEStart, offset, props...)
}

// cseStop emits a 5016/6016/7016 extension stop.
//
// CSEElaspedTimeInMilliSeconds is always populated with a value that disagrees
// with the measured interval, so any test asserting a duration also proves the
// provider-reported field is not the one being emitted.
func (f *gpFixture) cseStop(activity windows.GUID, offset time.Duration, id uint16, guid, name, errorCode string) {
	f.send(activity, id, offset,
		property{Name: "CSEExtensionId", Value: guid},
		property{Name: "CSEExtensionName", Value: name},
		property{Name: "CSEElaspedTimeInMilliSeconds", Value: "999999"},
		property{Name: "ErrorCode", Value: errorCode},
	)
}

// details finalizes and requires a non-nil block.
func (f *gpFixture) details() *GroupPolicyDetails {
	f.t.Helper()
	d := f.finalize()
	require.NotNil(f.t, d, "expected group policy details")
	return d
}

// finalize returns the block as-is, including nil.
func (f *gpFixture) finalize() *GroupPolicyDetails {
	return f.coll.groupPolicy.finalize(f.coll.timeline)
}

// gpoFragment builds an ApplicableGPOList value: a rootless sequence of <GPO>
// siblings, which is what a boot capture showed the provider emitting.
func gpoFragment(entries ...string) string {
	return strings.Join(entries, "")
}

// gpoEntry is one entry in the shape a real 4016 carries: ID attribute and Name
// only.
func gpoEntry(id, name string) string {
	return fmt.Sprintf(`<GPO ID="%s"><Name>%s</Name></GPO>`, id, name)
}

// gpoRichEntry is the fuller entry shape the 5312 inventory carries, which
// embeds the applying extensions' own GUIDs. 5312 is not collected, so this is
// never what the parser sees in production - it is here to prove the walk reads
// ID attributes rather than scanning for braces, which is the precondition that
// makes the fallback scan safe.
func gpoRichEntry(id, name string) string {
	return fmt.Sprintf(
		`<GPO ID="%s"><Name>%s</Name><Version>65539</Version><SOM>DC=corp,DC=example</SOM><FSPath>\\corp\SysVol</FSPath><Extensions>[{35378EAC-683F-11D2-A89A-00C04FBBCFA2}]</Extensions></GPO>`,
		id, name)
}

// --- Pairing and outcomes ---

func TestCSEPairingOutcomes(t *testing.T) {
	// The three terminal events share one template and differ only in severity.
	cases := []struct {
		name      string
		eventID   uint16
		errorCode string
		want      cseResult
		wantCode  string
	}{
		{"success", evtCSEStopSuccess, "0x00000000", cseResultSuccess, ""},
		{"warning", evtCSEStopWarning, "0x00000534", cseResultWarning, "0x00000534"},
		{"error", evtCSEStopError, "0x8007054B", cseResultError, "0x8007054B"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newGPFixture(t)
			f.startComputerPass()
			f.cseStart(gpTestActivity, 12500*time.Millisecond, cseRegistryGUID, "Registry", false, "")
			f.cseStop(gpTestActivity, 13750*time.Millisecond, tc.eventID, cseRegistryGUID, "Registry", tc.errorCode)

			invs := f.details().Computer
			require.Len(t, invs, 1)
			inv := invs[0]

			assert.Equal(t, cseRegistryGUID, inv.CSEID)
			assert.Equal(t, "Registry", inv.CSEName)
			assert.Equal(t, tc.want, inv.Result)
			assert.Equal(t, tc.wantCode, inv.ErrorCode)
			assert.False(t, inv.Async)

			assert.Equal(t, int64(12500), inv.OffsetMs, "offset is boot-relative")
			assert.Equal(t, int64(1250), inv.DurationMs,
				"duration is the measured interval, not the provider's CSEElaspedTimeInMilliSeconds")
		})
	}
}

func TestCSESuccessCarriesNoErrorCode(t *testing.T) {
	// A zero status means success and carries no information, so it is omitted
	// rather than emitted as "0x00000000".
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	raw, err := json.Marshal(f.details().Computer[0])
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "error_code")
}

func TestCSEZeroDurationIsReported(t *testing.T) {
	// An extension completing in under a millisecond has a real duration of
	// zero. It must survive marshalling rather than being elided as empty.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 13*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	inv := f.details().Computer[0]
	assert.Equal(t, int64(0), inv.DurationMs)

	raw, err := json.Marshal(inv)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"duration_ms":0`)
}

func TestCSEAsyncIsFlagged(t *testing.T) {
	// For an asynchronous extension the terminal event marks the dispatch of a
	// worker thread. The interval is still real - it is the cost of the
	// dispatch - and async is what tells a consumer to read it that way. The
	// outcome still comes from the terminal event ID; Microsoft documents the
	// audit extension returning E_PENDING here by design.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseAuditGUID, "Audit Policy", true, "")
	f.cseStop(gpTestActivity, 13100*time.Millisecond, evtCSEStopSuccess, cseAuditGUID, "Audit Policy", "0x8000000A")

	inv := f.details().Computer[0]
	assert.True(t, inv.Async)
	assert.Equal(t, cseResultSuccess, inv.Result)
	assert.Equal(t, "0x8000000A", inv.ErrorCode)
	assert.Equal(t, int64(100), inv.DurationMs)
}

func TestCSECrossActivityIsolation(t *testing.T) {
	// The same extension runs in both passes. Pairing on the extension GUID
	// alone would cross them, so the activity ID is part of the key.
	f := newGPFixture(t)
	f.startComputerPass()
	f.startUserPass()

	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStart(gpUserActivity, 31*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 15*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")
	f.cseStop(gpUserActivity, 31500*time.Millisecond, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	d := f.details()
	require.Len(t, d.Computer, 1)
	require.Len(t, d.User, 1)
	assert.Equal(t, int64(2000), d.Computer[0].DurationMs)
	assert.Equal(t, int64(500), d.User[0].DurationMs)
}

func TestInvocationsAreOrderedChronologically(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	for i, guid := range []string{cseAuditGUID, cseRegistryGUID, cseFolderRedGUID} {
		start := time.Duration(20-i) * time.Second
		f.cseStart(gpTestActivity, start, guid, "ext", false, "")
		f.cseStop(gpTestActivity, start+100*time.Millisecond, evtCSEStopSuccess, guid, "ext", "0x00000000")
	}

	invs := f.details().Computer
	require.Len(t, invs, 3)
	for i := 1; i < len(invs); i++ {
		assert.LessOrEqual(t, invs[i-1].OffsetMs, invs[i].OffsetMs)
	}
	assert.Equal(t, cseFolderRedGUID, invs[0].CSEID)
}

// --- Records with no measurable interval are dropped ---

func TestCSEMissingStartIsOmitted(t *testing.T) {
	// A terminal event with no start has no interval, so it cannot be placed on
	// the timeline. It is a collection diagnostic, not latency data.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	assert.Nil(t, f.finalize())
}

func TestCSEMissingTerminalIsOmitted(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, gpoDefaultDomainGUID)

	assert.Nil(t, f.finalize())
}

func TestCSETraceEndsMidInvocation(t *testing.T) {
	// The capture window closes when the Agent service starts, so a trailing
	// open invocation is the normal tail case. The completed one survives.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")
	f.cseStart(gpTestActivity, 15*time.Second, cseFolderRedGUID, "Folder Redirection", false, "")

	invs := f.details().Computer
	require.Len(t, invs, 1)
	assert.Equal(t, cseRegistryGUID, invs[0].CSEID)
}

func TestCSEDuplicateOpenStartsNeverGuess(t *testing.T) {
	// Two starts for one key cannot be disambiguated. The later one wins - it
	// gives the shorter, more conservative interval - and exactly one record is
	// emitted rather than one real and one invented.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStart(gpTestActivity, 14*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 16*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	invs := f.details().Computer
	require.Len(t, invs, 1)
	assert.Equal(t, int64(2000), invs[0].DurationMs, "measured from the later start")
	assert.Equal(t, int64(14000), invs[0].OffsetMs)
}

func TestCSEStopBeforeStartIsDropped(t *testing.T) {
	// A negative interval is not a duration.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 16*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 15*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	assert.Nil(t, f.finalize())
}

func TestCSEMissingExtensionIDIsDropped(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.send(gpTestActivity, evtCSEStart, 13*time.Second,
		property{Name: "CSEExtensionName", Value: "Registry"})
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	assert.Nil(t, f.finalize())
}

func TestDetailsOmittedWhenNoCompleteInvocation(t *testing.T) {
	f := newGPFixture(t)
	assert.Nil(t, f.finalize(), "a trace with no Group Policy events emits no block")

	f.startComputerPass()
	f.endComputerPass()
	assert.Nil(t, f.finalize(), "a pass that invoked no extension emits no block")
}

// --- Attribution: only the boot pass, never a guess ---

func TestGPUpdateInvocationsAreExcluded(t *testing.T) {
	// A gpupdate, a network-state change, and a periodic refresh each run their
	// own policy processing instance with its own activity ID. Their extension
	// invocations are not part of the boot pass, and counting them there would
	// let the child durations sum past the parent pass duration.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	// Manual processing (4004) is not collected at all, and its extension
	// invocations carry an activity ID that matches no boot pass.
	f.send(gpOtherRun, 4004, 40*time.Second)
	f.cseStart(gpOtherRun, 41*time.Second, cseFolderRedGUID, "Folder Redirection", false, "")
	f.cseStop(gpOtherRun, 42*time.Second, evtCSEStopSuccess, cseFolderRedGUID, "Folder Redirection", "0x00000000")

	d := f.details()
	require.Len(t, d.Computer, 1, "only the boot pass invocation is reported")
	assert.Equal(t, cseRegistryGUID, d.Computer[0].CSEID)
	assert.Empty(t, d.User)
}

func TestCSEWithNoBootPassIsOmitted(t *testing.T) {
	// An activity ID matching no boot pass is never attributed to whichever
	// pass happened to start most recently.
	f := newGPFixture(t)
	f.startComputerPass()
	f.startUserPass()
	f.cseStart(gpOtherRun, 40*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpOtherRun, 41*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	assert.Nil(t, f.finalize())
}

func TestPassActivityPinnedFirstWriteWins(t *testing.T) {
	// Mirrors MachineGPStart's own first-write-wins rule, so the invocations
	// collected always belong to the pass boot_timeline reports.
	f := newGPFixture(t)
	f.startComputerPass()
	f.send(gpOtherRun, evtMachineGPStart, 40*time.Second)

	f.cseStart(gpOtherRun, 41*time.Second, cseFolderRedGUID, "Folder Redirection", false, "")
	f.cseStop(gpOtherRun, 42*time.Second, evtCSEStopSuccess, cseFolderRedGUID, "Folder Redirection", "0x00000000")
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	invs := f.details().Computer
	require.Len(t, invs, 1)
	assert.Equal(t, cseRegistryGUID, invs[0].CSEID)
}

func TestZeroActivityIDNeverPins(t *testing.T) {
	// Events outside any pass carry a zero ActivityId - 4117, 5324, and 5351
	// were all observed that way in a real trace. Pinning zero for a scope would
	// sweep every unattributed invocation into that pass.
	f := newGPFixture(t)
	f.send(windows.GUID{}, evtMachineGPStart, 12*time.Second)
	f.cseStart(windows.GUID{}, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(windows.GUID{}, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	assert.Nil(t, f.finalize())
}

func TestPassStopAloneNeverPins(t *testing.T) {
	// Only a pass start claims a scope. A stop event arriving without its start
	// leaves the scope unclaimed, so its invocations are not attributed - which is
	// what keeps this block from describing slices of a pass boot_timeline has no
	// milestone for, since the milestone comes from the same start event.
	f := newGPFixture(t)
	f.startComputerPass()
	f.endComputerPass()

	f.cseStart(gpUserActivity, 31*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpUserActivity, 32*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")
	f.send(gpUserActivity, evtUserGPEnd, 33*time.Second)

	assert.Nil(t, f.finalize())
	assert.False(t, f.coll.timeline.UserGPEnd.IsZero(), "the stop still sets its timeline field")
	assert.True(t, f.coll.timeline.UserGPStart.IsZero(), "no user pass start was observed")
}

func TestPassActivityIsNotSharedBetweenScopes(t *testing.T) {
	// One activity cannot be both passes. If it were accepted for the second
	// scope, buildScope would read the same bucket twice and emit every
	// invocation under computer and user alike.
	f := newGPFixture(t)
	f.startComputerPass()
	f.send(gpTestActivity, evtUserGPStart, 30*time.Second)

	f.cseStart(gpTestActivity, 31*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 32*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	d := f.details()
	require.Len(t, d.Computer, 1)
	assert.Empty(t, d.User, "the user scope never claimed the computer pass's activity")
}

// --- Offsets share the boot_timeline axis ---

func TestCSEOffsetsShareTheBootTimelineAxis(t *testing.T) {
	// buildTimelineMilestones collapses the login-screen idle gap out of
	// post-logon offsets. An invocation placed on a raw boot-relative axis would
	// land outside its own parent milestone, so both go through bootOffsetFunc.
	f := newGPFixture(t)
	f.coll.timeline.LoginUIDone = gpTestBoot.Add(10 * time.Second)
	f.coll.timeline.SessionLogon = gpTestBoot.Add(70 * time.Second)

	f.send(gpUserActivity, evtUserGPStart, 71*time.Second)
	f.cseStart(gpUserActivity, 72*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpUserActivity, 73*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")
	f.send(gpUserActivity, evtUserGPEnd, 75*time.Second)

	inv := f.details().User[0]

	var parent Milestone
	for _, m := range buildTimelineMilestones(f.coll.timeline) {
		if m.ID == "user_group_policy" {
			parent = m
		}
	}
	require.Equal(t, "user_group_policy", parent.ID, "parent milestone must exist")

	// The 60s login-screen gap is collapsed out of both.
	assert.Equal(t, float64(11000), parent.OffsetMs)
	assert.Equal(t, int64(12000), inv.OffsetMs)

	assert.GreaterOrEqual(t, float64(inv.OffsetMs), parent.OffsetMs,
		"invocation starts at or after its pass")
	assert.LessOrEqual(t, float64(inv.OffsetMs+inv.DurationMs), parent.OffsetMs+parent.DurationMs,
		"invocation ends at or before its pass")
}

// --- Group Policy objects ---

func TestGPOInlinedPerInvocation(t *testing.T) {
	// GPOs are repeated on every invocation that references them rather than
	// pooled in a shared table, so a consumer rendering the tree needs no join.
	f := newGPFixture(t)
	f.startComputerPass()

	list := gpoFragment(gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy"))
	for i, guid := range []string{cseRegistryGUID, cseFolderRedGUID, cseAuditGUID} {
		start := time.Duration(13+i) * time.Second
		f.cseStart(gpTestActivity, start, guid, "ext", false, list)
		f.cseStop(gpTestActivity, start+500*time.Millisecond, evtCSEStopSuccess, guid, "ext", "0x00000000")
	}

	invs := f.details().Computer
	require.Len(t, invs, 3)
	for _, inv := range invs {
		require.Len(t, inv.GPOs, 1)
		assert.Equal(t, gpoDefaultDomainGUID, inv.GPOs[0].ID)
		assert.Equal(t, "Default Domain Policy", inv.GPOs[0].Name)
	}
}

func TestGPONamesComeFromTheApplicableList(t *testing.T) {
	// A boot capture confirms ApplicableGPOList carries ID and Name, so 4016 is
	// the only event needed and the 5312 inventory is not collected. The fuller
	// inventory shape is fed here to prove the walk reads ID attributes rather
	// than scanning: a scan would report the GUID inside <Extensions>, which is an
	// extension's own identity and not an applicable GPO.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false,
		gpoFragment(gpoRichEntry(gpoDefaultDomainGUID, "Default Domain Policy")))
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	gpos := f.details().Computer[0].GPOs
	require.Len(t, gpos, 1, "the extension GUID inside <Extensions> is not a GPO")
	assert.Equal(t, gpoDefaultDomainGUID, gpos[0].ID)
	assert.Equal(t, "Default Domain Policy", gpos[0].Name)
}

func TestGPONamesSurviveUnescapedAmpersand(t *testing.T) {
	// Windows concatenates ApplicableGPOList without escaping display names, so a
	// GPO named "R&D Baseline" arrives carrying a bare ampersand - a boot capture
	// carried exactly that, in the first position of every list. Non-strict
	// decoding leaves the malformed entity alone, so the whole list still parses.
	//
	// Under strict parsing the walk would abort on that entry: with the offending
	// name first the fallback recovers the IDs and the payload shows GUIDs with no
	// names at all, and with it mid-list the walk returns a prefix and the tail is
	// dropped outright. Both positions are covered here for that reason.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false,
		gpoFragment(
			gpoEntry(gpoDefaultDomainGUID, "R&D Baseline"),
			gpoEntry(gpoDomainCtlGUID, "Sales & Marketing"),
			gpoEntry(gpoThirdGUID, "Plain Name"),
		))
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	assert.Equal(t, []GPORef{
		{ID: gpoDefaultDomainGUID, Name: "R&D Baseline"},
		{ID: gpoDomainCtlGUID, Name: "Sales & Marketing"},
		{ID: gpoThirdGUID, Name: "Plain Name"},
	}, f.details().Computer[0].GPOs)
}

func TestGPONameSharedFromAnotherInvocationsList(t *testing.T) {
	// The name lookup spans the trace and resolves at finalize, so an invocation
	// whose own list carried no name still gets one from a list that did - even a
	// later one. That is what supplies names on the degraded path, where the walk
	// ended early and the fallback scan recovered GUIDs without their names.
	f := newGPFixture(t)
	f.startComputerPass()

	// This invocation's list is a bare GUID: no name available from it.
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, gpoDefaultDomainGUID)
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	// A later invocation names the same object.
	f.cseStart(gpTestActivity, 15*time.Second, cseFolderRedGUID, "Folder Redirection", false,
		gpoFragment(gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy")))
	f.cseStop(gpTestActivity, 16*time.Second, evtCSEStopSuccess, cseFolderRedGUID, "Folder Redirection", "0x00000000")

	invs := f.details().Computer
	require.Len(t, invs, 2)
	assert.Equal(t, "Default Domain Policy", invs[0].GPOs[0].Name,
		"the earlier invocation picks up the later list's name")
}

func TestGPOMultiplePerCSE(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 12100*time.Millisecond, cseAuditGUID, "Audit", false, gpoFragment(
		gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy"),
		gpoEntry(gpoDomainCtlGUID, "Default Domain Controllers Policy"),
	))
	f.cseStop(gpTestActivity, 12200*time.Millisecond, evtCSEStopSuccess, cseAuditGUID, "Audit", "0x00000000")

	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false,
		gpoDefaultDomainGUID+";"+gpoDomainCtlGUID+";"+gpoThirdGUID)
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	gpos := f.details().Computer[1].GPOs
	require.Len(t, gpos, 3)
	assert.Equal(t, "Default Domain Policy", gpos[0].Name)
	assert.Equal(t, "Default Domain Controllers Policy", gpos[1].Name)
	assert.Equal(t, gpoThirdGUID, gpos[2].ID)
	assert.Empty(t, gpos[2].Name, "no list named this one, so the ID stands alone")
}

func TestGPODuplicateDisplayNamesStayDistinct(t *testing.T) {
	// The GUID is the identity. Two GPOs may legitimately share a display name.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, gpoFragment(
		gpoEntry(gpoDefaultDomainGUID, "Baseline"),
		gpoEntry(gpoDomainCtlGUID, "Baseline"),
	))
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	gpos := f.details().Computer[0].GPOs
	require.Len(t, gpos, 2)
	assert.NotEqual(t, gpos[0].ID, gpos[1].ID)
	assert.Equal(t, "Baseline", gpos[0].Name)
	assert.Equal(t, "Baseline", gpos[1].Name)
}

func TestGPONamesWithoutASurvivingInvocationEmitNothing(t *testing.T) {
	// The name lookup is not an inventory: an entry reaches the wire only when a
	// surviving invocation references it. Here the list was parsed and its name
	// recorded, but the invocation never closed, so nothing is emitted.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false,
		gpoFragment(gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy")))

	assert.Nil(t, f.finalize())
}

func TestGPORefsFromList(t *testing.T) {
	// A boot capture confirms the XML shape, and the scan stays as a fallback for
	// a fragment the walk cannot finish or a value with another delimiter.
	cases := []struct {
		name      string
		raw       string
		wantIDs   []string
		wantNames map[string]string
	}{
		{name: "empty"},
		{
			// The fuller inventory shape embeds <Extensions>[{CSE GUID}]</Extensions>.
			// A bare braced-GUID scan over it would report the extension's own GUID
			// as an applicable GPO; the walk reads ID attributes instead.
			name:      "xml fragment reads id attributes only",
			raw:       gpoFragment(gpoRichEntry(gpoDefaultDomainGUID, "Default Domain Policy")),
			wantIDs:   []string{gpoDefaultDomainGUID},
			wantNames: map[string]string{gpoDefaultDomainGUID: "Default Domain Policy"},
		},
		{
			name: "real 4016 shape yields ids and names",
			raw: gpoFragment(
				gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy"),
				gpoEntry(gpoDomainCtlGUID, "Default Domain Controllers Policy"),
			),
			wantIDs: []string{gpoDefaultDomainGUID, gpoDomainCtlGUID},
			wantNames: map[string]string{
				gpoDefaultDomainGUID: "Default Domain Policy",
				gpoDomainCtlGUID:     "Default Domain Controllers Policy",
			},
		},
		{
			name:      "rooted variant parses too",
			raw:       "<GPOList>" + gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy") + "</GPOList>",
			wantIDs:   []string{gpoDefaultDomainGUID},
			wantNames: map[string]string{gpoDefaultDomainGUID: "Default Domain Policy"},
		},
		{
			name:      "id attribute casing does not matter",
			raw:       `<GPO id="` + gpoDefaultDomainGUID + `"><Name>Default Domain Policy</Name></GPO>`,
			wantIDs:   []string{gpoDefaultDomainGUID},
			wantNames: map[string]string{gpoDefaultDomainGUID: "Default Domain Policy"},
		},
		{
			name:      "entry without a guid is dropped",
			raw:       `<GPO><Name>Nameless</Name></GPO>` + gpoEntry(gpoDefaultDomainGUID, "Real"),
			wantIDs:   []string{gpoDefaultDomainGUID},
			wantNames: map[string]string{gpoDefaultDomainGUID: "Real"},
		},
		// An unescaped ampersand does not end the walk: non-strict decoding leaves
		// the malformed entity in the character data. Both positions are covered
		// because strict decoding would fail differently in each - IDs kept and
		// names lost when it is first, tail dropped when it is not.
		{
			name: "unescaped ampersand first keeps every id and name",
			raw: gpoFragment(
				gpoEntry(gpoDefaultDomainGUID, "R&D Baseline"),
				gpoEntry(gpoDomainCtlGUID, "Plain Name"),
			),
			wantIDs: []string{gpoDefaultDomainGUID, gpoDomainCtlGUID},
			wantNames: map[string]string{
				gpoDefaultDomainGUID: "R&D Baseline",
				gpoDomainCtlGUID:     "Plain Name",
			},
		},
		{
			name: "unescaped ampersand mid-list keeps every id and name",
			raw: gpoFragment(
				gpoEntry(gpoDefaultDomainGUID, "Plain Name"),
				gpoEntry(gpoDomainCtlGUID, "R&D Baseline"),
				gpoEntry(gpoThirdGUID, "Sales & Marketing"),
			),
			wantIDs: []string{gpoDefaultDomainGUID, gpoDomainCtlGUID, gpoThirdGUID},
			wantNames: map[string]string{
				gpoDefaultDomainGUID: "Plain Name",
				gpoDomainCtlGUID:     "R&D Baseline",
				gpoThirdGUID:         "Sales & Marketing",
			},
		},
		// Non-strict decoding forgives malformed entities and missing end tags but
		// not broken tag syntax, so a display name whose "<" does not open a
		// well-formed tag really does end the walk. (A name containing "<Test>"
		// would not: that parses as a nested element and only costs the text after
		// it.) These are the rows that exercise the fallback on a partial walk -
		// without a completeness signal the parser would return only the entries
		// before the break and drop the rest of the list outright.
		{
			name: "unparsable angle bracket first recovers every id",
			raw: gpoFragment(
				gpoEntry(gpoDefaultDomainGUID, "Legacy <Test Baseline"),
				gpoEntry(gpoDomainCtlGUID, "Plain Name"),
			),
			wantIDs: []string{gpoDefaultDomainGUID, gpoDomainCtlGUID},
		},
		{
			name: "unparsable angle bracket mid-list recovers the tail",
			raw: gpoFragment(
				gpoEntry(gpoDefaultDomainGUID, "Plain Name"),
				gpoEntry(gpoDomainCtlGUID, "Legacy <Test Baseline"),
				gpoEntry(gpoThirdGUID, "Third Policy"),
			),
			wantIDs:   []string{gpoDefaultDomainGUID, gpoDomainCtlGUID, gpoThirdGUID},
			wantNames: map[string]string{gpoDefaultDomainGUID: "Plain Name"},
		},
		{
			// A mismatched end tag is the subtler case: the offending entry decodes
			// without error and is reported, and the walk only fails on the token
			// after it. So ids is non-empty at the break and the completeness signal
			// is the only thing that saves the tail.
			name: "mismatched end tag mid-list recovers the tail",
			raw: `<GPO ID="` + gpoDefaultDomainGUID + `"><Name>Plain Name</Nam></GPO>` +
				gpoEntry(gpoDomainCtlGUID, "Second Policy"),
			wantIDs:   []string{gpoDefaultDomainGUID, gpoDomainCtlGUID},
			wantNames: map[string]string{gpoDefaultDomainGUID: "Plain Name"},
		},
		{
			name:    "truncated xml still yields its guid",
			raw:     `<GPO ID="` + gpoDefaultDomainGUID + `"><Nam`,
			wantIDs: []string{gpoDefaultDomainGUID},
		},
		{name: "semicolon delimited", raw: gpoDefaultDomainGUID + ";" + gpoDomainCtlGUID, wantIDs: []string{gpoDefaultDomainGUID, gpoDomainCtlGUID}},
		{name: "newline delimited", raw: gpoDefaultDomainGUID + "\n" + gpoDomainCtlGUID, wantIDs: []string{gpoDefaultDomainGUID, gpoDomainCtlGUID}},
		{name: "prose", raw: "Applied " + gpoDefaultDomainGUID + " to the OU", wantIDs: []string{gpoDefaultDomainGUID}},
		{name: "lowercase is normalized", raw: strings.ToLower(gpoDefaultDomainGUID), wantIDs: []string{gpoDefaultDomainGUID}},
		{name: "duplicates collapse", raw: gpoDefaultDomainGUID + ";" + gpoDefaultDomainGUID, wantIDs: []string{gpoDefaultDomainGUID}},
		{name: "no guids", raw: "<GPO><Name>Nameless</Name></GPO>"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ids, names := gpoRefsFromList(tc.raw)
			assert.Equal(t, tc.wantIDs, ids)
			assert.Equal(t, tc.wantNames, names)
		})
	}

	t.Run("oversize list is rejected before scanning", func(t *testing.T) {
		ids, names := gpoRefsFromList(strings.Repeat("x", maxGPOListBytes+1))
		assert.Nil(t, ids)
		assert.Nil(t, names)
	})

	t.Run("references are bounded per invocation", func(t *testing.T) {
		var b strings.Builder
		for i := 0; i < maxGPOsPerCSE+20; i++ {
			fmt.Fprintf(&b, "{%08X-0000-0000-0000-000000000000};", i)
		}
		ids, _ := gpoRefsFromList(b.String())
		assert.Len(t, ids, maxGPOsPerCSE)
	})

	t.Run("names are bounded with the ids they belong to", func(t *testing.T) {
		var b strings.Builder
		for i := 0; i < maxGPOsPerCSE+20; i++ {
			b.WriteString(gpoEntry(fmt.Sprintf("{%08X-0000-0000-0000-000000000000}", i), "Policy"))
		}
		ids, names := gpoRefsFromList(b.String())
		assert.Len(t, ids, maxGPOsPerCSE)
		assert.Len(t, names, maxGPOsPerCSE, "a name is only kept for an ID that was taken")
	})
}

// --- Collection gate ---

func TestAcceptedGroupPolicyIDsSnapshot(t *testing.T) {
	// acceptedIDs is a hard pre-parse gate: analyzeETL hands it to the ETW filter
	// and processEvent never re-checks it, so an ID the parser handles but the map
	// omits is dropped in production with nothing else to catch it.
	//
	// This is a snapshot, not a derivation. It catches an ID being removed from
	// the map, and it documents the intended set - but because want is a literal
	// rather than the switch's own cases, it cannot catch a new case being added
	// to Parse without a matching entry here. Closing that would take an AST walk
	// over Parse; until then the gate is a two-place edit by construction.
	want := map[uint16]struct{}{
		evtMachineGPStart: {}, evtMachineGPEnd: {},
		evtUserGPStart: {}, evtUserGPEnd: {},
		evtCSEStart:       {},
		evtCSEStopSuccess: {}, evtCSEStopWarning: {}, evtCSEStopError: {},
	}
	got := newCollector().providers[guidGroupPolicy].acceptedIDs
	assert.Equal(t, want, got)

	// Documenting what the set deliberately excludes, and why. 4002/4003 are
	// network-state change, 4004/4005 manual processing, 4006/4007 periodic
	// refresh; each has its own start/stop pair, so none is the boot pass
	// boot_timeline reports. 5312/5313 are the applicable and filtered-out GPO
	// inventories, which 4016's own list makes redundant.
	for _, id := range []uint16{4002, 4003, 4004, 4005, 4006, 4007, 5312, 5313} {
		assert.NotContains(t, got, id, "event %d must not be collected", id)
	}
}

func TestActivityIDOfNilEventIsZero(t *testing.T) {
	assert.Equal(t, windows.GUID{}, activityIDOf(nil))
}

func TestEventPropertyReaderUsesThePartialBulkDecode(t *testing.T) {
	// A 4016 carries seven properties. TDH stops at the first it cannot decode,
	// and EventProperties returns what it recovered alongside the error. The
	// per-property path returns "" for the whole event in that case, so reading
	// the partial bulk result is the only way these values survive.
	e := makeEvent(guidGroupPolicy, evtCSEStart, gpTestBoot,
		property{Name: "CSEExtensionId", Value: cseRegistryGUID},
		property{Name: "CSEExtensionName", Value: "Registry"},
	)
	e.propsErr = errors.New(`failed to parse property [2] "IsExtensionAsyncProcessing"`)
	require.Empty(t, getEventPropString(e, "CSEExtensionName"),
		"the per-property path yields nothing on a failed decode")

	prop := eventPropertyReader(e)
	assert.Equal(t, cseRegistryGUID, prop("CSEExtensionId"))
	assert.Equal(t, "Registry", prop("CSEExtensionName"))
	assert.Empty(t, prop("ApplicableGPOList"), "a property after the failure is absent, not garbage")
}

func TestPartialDecodeDegradesAStartButDropsAStop(t *testing.T) {
	// The two templates order CSEExtensionId differently: first on 4016, last on
	// 5016/6016/7016. Since a partial decode recovers only the properties before
	// the failure, the same failure costs a start its trailing fields but costs a
	// stop the identity itself - and with it the whole invocation.
	t.Run("a partial start keeps its identity and interval", func(t *testing.T) {
		f := newGPFixture(t)
		f.startComputerPass()
		// Decode died at IsExtensionAsyncProcessing, so the async flag and the GPO
		// list are gone but the GUID and name are not.
		f.sendPartial(gpTestActivity, evtCSEStart, 13*time.Second, "IsExtensionAsyncProcessing",
			property{Name: "CSEExtensionId", Value: cseRegistryGUID},
			property{Name: "CSEExtensionName", Value: "Registry"},
		)
		f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

		invs := f.details().Computer
		require.Len(t, invs, 1)
		assert.Equal(t, int64(1000), invs[0].DurationMs)
		assert.False(t, invs[0].Async, "the flag was never decoded, so it reads as synchronous")
		assert.Empty(t, invs[0].GPOs)
	})

	t.Run("a partial stop loses the invocation", func(t *testing.T) {
		f := newGPFixture(t)
		f.startComputerPass()
		f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
		// CSEExtensionId is the last property on this template, so a decode that
		// died at CSEExtensionName never reaches it. Without the identity the stop
		// cannot be paired, and the start is dropped at finalize.
		f.sendPartial(gpTestActivity, evtCSEStopSuccess, 14*time.Second, "CSEExtensionName",
			property{Name: "CSEElaspedTimeInMilliSeconds", Value: "999999"},
			property{Name: "ErrorCode", Value: "0x00000000"},
		)

		assert.Nil(t, f.finalize(), "both endpoints were in the trace, but the pairing key was not")
	})
}

// --- Scalar helpers ---

func TestNormalizeGUID(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{cseRegistryGUID, cseRegistryGUID, true},
		{strings.ToLower(cseRegistryGUID), cseRegistryGUID, true},
		{"35378EAC-683F-11D2-A89A-00C04FBBCFA2", cseRegistryGUID, true},
		{"  " + cseRegistryGUID + "  ", cseRegistryGUID, true},
		{"", "", false},
		{"not-a-guid", "", false},
		{"{35378EAC-683F-11D2-A89A}", "", false},
	}
	for _, tc := range cases {
		_, got, ok := normalizeGUID(tc.in)
		assert.Equal(t, tc.ok, ok, "input %q", tc.in)
		assert.Equal(t, tc.want, got, "input %q", tc.in)
	}
}

func TestFormatErrorCode(t *testing.T) {
	// ErrorCode is declared win:HexInt32, so TDH renders it as "0x…". A base-10
	// parse would fail on every non-zero code; base 0 also accepts a decimal
	// rendering, so both forms round-trip to the same canonical output.
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"0x8000000A", "0x8000000A", true},
		{"0x0000054B", "0x0000054B", true},
		{"1355", "0x0000054B", true},
		{"0xFFFFFFFF", "0xFFFFFFFF", true},
		{"0x00000000", "", false},
		{"0", "", false},
		{"", "", false},
		{"garbage", "", false},
		{"0x100000000", "", false},
	}
	for _, tc := range cases {
		got, ok := formatErrorCode(tc.in)
		assert.Equal(t, tc.ok, ok, "input %q", tc.in)
		assert.Equal(t, tc.want, got, "input %q", tc.in)
	}
}

func TestParseETWBool(t *testing.T) {
	// TDH renders win:Boolean as "true"/"false". The numeric spellings are
	// accepted because the provider declares the sibling IsMachine as win:Boolean
	// on version 0 of the pass boundary events and win:UInt32 on version 1, so a
	// same-named field really can arrive either way across builds.
	cases := []struct {
		in    string
		want  bool
		valid bool
	}{
		{"true", true, true}, {"TRUE", true, true}, {"True", true, true},
		{"false", false, true}, {"FALSE", false, true},
		{"1", true, true}, {"-1", true, true}, {"0", false, true},
		{" true ", true, true},
		{"", false, false}, {"yes", false, false}, {"2", false, false},
	}
	for _, tc := range cases {
		got, valid := parseETWBool(tc.in)
		assert.Equal(t, tc.valid, valid, "input %q", tc.in)
		assert.Equal(t, tc.want, got, "input %q", tc.in)
	}
}

func TestTruncateProviderText(t *testing.T) {
	assert.Equal(t, "Registry", truncateProviderText("  Registry  ", maxCSENameBytes))

	long := strings.Repeat("a", maxCSENameBytes+50)
	got := truncateProviderText(long, maxCSENameBytes)
	assert.LessOrEqual(t, len(got), maxCSENameBytes)
	assert.True(t, strings.HasSuffix(got, "..."))

	// Truncation must not split a multi-byte rune.
	multibyte := strings.Repeat("日", maxCSENameBytes)
	got = truncateProviderText(multibyte, maxCSENameBytes)
	assert.LessOrEqual(t, len(got), maxCSENameBytes)
	assert.True(t, utf8.ValidString(got))
}
