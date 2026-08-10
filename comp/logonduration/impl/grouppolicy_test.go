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

// Property values in these tests are the exact strings TDH produces for each
// declared out-type, confirmed against the Microsoft-Windows-GroupPolicy
// manifest: a braced uppercase GUID for win:GUID, "0x…" for win:HexInt32,
// decimal for win:UInt32, and "true"/"false" for win:Boolean.

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

// gpoInventory emits a 5312 applicable-GPO list.
func (f *gpFixture) gpoInventory(activity windows.GUID, offset time.Duration, list string) {
	f.send(activity, evtGPOListApplicable, offset, property{Name: "GPOInfoList", Value: list})
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

// gpoFragment builds a GPOInfoList value: a rootless sequence of <GPO> siblings.
func gpoFragment(entries ...string) string {
	return strings.Join(entries, "")
}

func gpoEntry(id, name string) string {
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

func TestUserPassRecoveredFromStopEventAlone(t *testing.T) {
	// Replays a real trace: 4000/8000 carried one activity, then the user pass
	// ran and emitted 8001 under a second activity with no 4001 anywhere. The
	// stop event identifies the pass just as well, so its invocations are still
	// attributed rather than discarded.
	f := newGPFixture(t)
	f.startComputerPass()
	f.endComputerPass()

	f.cseStart(gpUserActivity, 31*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpUserActivity, 32*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")
	f.send(gpUserActivity, evtUserGPEnd, 33*time.Second)

	d := f.details()
	assert.Empty(t, d.Computer)
	require.Len(t, d.User, 1)
	assert.Equal(t, cseRegistryGUID, d.User[0].CSEID)
	assert.Equal(t, int64(1000), d.User[0].DurationMs)
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
	f.gpoInventory(gpTestActivity, 12100*time.Millisecond,
		gpoFragment(gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy")))

	for i, guid := range []string{cseRegistryGUID, cseFolderRedGUID, cseAuditGUID} {
		start := time.Duration(13+i) * time.Second
		f.cseStart(gpTestActivity, start, guid, "ext", false, gpoDefaultDomainGUID)
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

func TestGPONamesFromApplicableListAloneNeedNo5312(t *testing.T) {
	// The runtime shape of ApplicableGPOList is unverified. If it turns out to
	// carry the same <GPO ID="…"><Name> entries as the 5312 inventory, then
	// names are available from 4016 alone and no inventory event is required.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false,
		gpoFragment(gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy")))
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	gpos := f.details().Computer[0].GPOs
	require.Len(t, gpos, 1, "the extension GUID inside <Extensions> is not a GPO")
	assert.Equal(t, gpoDefaultDomainGUID, gpos[0].ID)
	assert.Equal(t, "Default Domain Policy", gpos[0].Name)
}

func TestGPOInventoryResolvesNameArrivingAfterTheInvocation(t *testing.T) {
	// Names are resolved at finalize, so event ordering does not matter.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, gpoDefaultDomainGUID)
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")
	f.gpoInventory(gpTestActivity, 15*time.Second,
		gpoFragment(gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy")))

	assert.Equal(t, "Default Domain Policy", f.details().Computer[0].GPOs[0].Name)
}

func TestGPOMultiplePerCSE(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.gpoInventory(gpTestActivity, 12100*time.Millisecond, gpoFragment(
		gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy"),
		gpoEntry(gpoDomainCtlGUID, "Default Domain Controllers Policy"),
	))
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false,
		gpoDefaultDomainGUID+";"+gpoDomainCtlGUID+";"+gpoThirdGUID)
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	gpos := f.details().Computer[0].GPOs
	require.Len(t, gpos, 3)
	assert.Equal(t, "Default Domain Policy", gpos[0].Name)
	assert.Equal(t, "Default Domain Controllers Policy", gpos[1].Name)
	assert.Equal(t, gpoThirdGUID, gpos[2].ID)
	assert.Empty(t, gpos[2].Name, "no inventory entry, so the ID stands alone")
}

func TestGPODuplicateDisplayNamesStayDistinct(t *testing.T) {
	// The GUID is the identity. Two GPOs may legitimately share a display name.
	f := newGPFixture(t)
	f.startComputerPass()
	f.gpoInventory(gpTestActivity, 12100*time.Millisecond, gpoFragment(
		gpoEntry(gpoDefaultDomainGUID, "Baseline"),
		gpoEntry(gpoDomainCtlGUID, "Baseline"),
	))
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false,
		gpoDefaultDomainGUID+" "+gpoDomainCtlGUID)
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", "0x00000000")

	gpos := f.details().Computer[0].GPOs
	require.Len(t, gpos, 2)
	assert.NotEqual(t, gpos[0].ID, gpos[1].ID)
	assert.Equal(t, "Baseline", gpos[0].Name)
	assert.Equal(t, "Baseline", gpos[1].Name)
}

func TestGPOInventoryAloneEmitsNothing(t *testing.T) {
	// The name lookup is not an inventory: an entry reaches the wire only when a
	// surviving invocation references it. This is also what keeps an inventory
	// from an unrelated processing run out of the payload.
	f := newGPFixture(t)
	f.startComputerPass()
	f.gpoInventory(gpTestActivity, 12100*time.Millisecond,
		gpoFragment(gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy")))

	assert.Nil(t, f.finalize())
}

func TestGPOIDsFromList(t *testing.T) {
	// The runtime format of ApplicableGPOList has never been observed, so the
	// scan is deliberately format-agnostic.
	cases := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", "", nil},
		// gpoEntry embeds an <Extensions>[{CSE GUID}]</Extensions> element, so a
		// bare braced-GUID scan over the fragment would report the extension's
		// own GUID as an applicable GPO. The XML tier reads the ID attributes
		// instead, which is the whole reason it exists.
		{"xml fragment reads id attributes only", gpoFragment(gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy")), []string{gpoDefaultDomainGUID}},
		{"xml fragment with several entries", gpoFragment(
			gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy"),
			gpoEntry(gpoDomainCtlGUID, "Default Domain Controllers Policy"),
		), []string{gpoDefaultDomainGUID, gpoDomainCtlGUID}},
		{"semicolon delimited", gpoDefaultDomainGUID + ";" + gpoDomainCtlGUID, []string{gpoDefaultDomainGUID, gpoDomainCtlGUID}},
		{"newline delimited", gpoDefaultDomainGUID + "\n" + gpoDomainCtlGUID, []string{gpoDefaultDomainGUID, gpoDomainCtlGUID}},
		{"prose", "Applied " + gpoDefaultDomainGUID + " to the OU", []string{gpoDefaultDomainGUID}},
		{"lowercase is normalized", strings.ToLower(gpoDefaultDomainGUID), []string{gpoDefaultDomainGUID}},
		{"duplicates collapse", gpoDefaultDomainGUID + ";" + gpoDefaultDomainGUID, []string{gpoDefaultDomainGUID}},
		{"no guids", "<GPO><Name>Nameless</Name></GPO>", nil},
		{"truncated xml still yields its guid", `<GPO ID="` + gpoDefaultDomainGUID + `"><Nam`, []string{gpoDefaultDomainGUID}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, gpoIDsFromList(tc.raw))
		})
	}

	t.Run("oversize list is rejected before scanning", func(t *testing.T) {
		assert.Nil(t, gpoIDsFromList(strings.Repeat(gpoDefaultDomainGUID, maxGPOListBytes)))
	})

	t.Run("references are bounded per invocation", func(t *testing.T) {
		var b strings.Builder
		for i := 0; i < maxGPOsPerCSE+20; i++ {
			fmt.Fprintf(&b, "{%08X-0000-0000-0000-000000000000};", i)
		}
		assert.Len(t, gpoIDsFromList(b.String()), maxGPOsPerCSE)
	})
}

func TestGPONamesFromInventory(t *testing.T) {
	t.Run("rootless fragment yields every entry", func(t *testing.T) {
		names := gpoNamesFromInventory(gpoFragment(
			gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy"),
			gpoEntry(gpoDomainCtlGUID, "Default Domain Controllers Policy"),
		))
		assert.Equal(t, map[string]string{
			gpoDefaultDomainGUID: "Default Domain Policy",
			gpoDomainCtlGUID:     "Default Domain Controllers Policy",
		}, names)
	})

	t.Run("rooted variant parses too", func(t *testing.T) {
		raw := "<GPOList>" + gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy") + "</GPOList>"
		assert.Equal(t, map[string]string{gpoDefaultDomainGUID: "Default Domain Policy"}, gpoNamesFromInventory(raw))
	})

	t.Run("id attribute casing does not matter", func(t *testing.T) {
		raw := `<GPO id="` + gpoDefaultDomainGUID + `"><Name>Default Domain Policy</Name></GPO>`
		assert.Equal(t, map[string]string{gpoDefaultDomainGUID: "Default Domain Policy"}, gpoNamesFromInventory(raw))
	})

	t.Run("entry without a guid is dropped", func(t *testing.T) {
		raw := `<GPO><Name>Nameless</Name></GPO>` + gpoEntry(gpoDefaultDomainGUID, "Real")
		assert.Equal(t, map[string]string{gpoDefaultDomainGUID: "Real"}, gpoNamesFromInventory(raw))
	})

	t.Run("malformed remainder keeps what parsed", func(t *testing.T) {
		raw := gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy") + `<GPO ID="` + gpoDomainCtlGUID + `"><Nam`
		assert.Equal(t, map[string]string{gpoDefaultDomainGUID: "Default Domain Policy"}, gpoNamesFromInventory(raw))
	})

	t.Run("empty and oversize yield nothing", func(t *testing.T) {
		// A real trace carried a 5312 with an empty GPOInfoList.
		assert.Nil(t, gpoNamesFromInventory(""))
		assert.Nil(t, gpoNamesFromInventory(strings.Repeat("<GPO/>", maxGPOListBytes)))
	})
}

// --- Collection gate ---

func TestAcceptedIDsCoversGroupPolicySwitch(t *testing.T) {
	// acceptedIDs is a hard pre-parse gate: analyzeETL hands it to the ETW
	// filter and processEvent never re-checks it, so an ID the parser handles
	// but the map omits is dropped in production with nothing else to catch it.
	want := map[uint16]struct{}{
		evtMachineGPStart: {}, evtMachineGPEnd: {},
		evtUserGPStart: {}, evtUserGPEnd: {},
		evtCSEStart:       {},
		evtCSEStopSuccess: {}, evtCSEStopWarning: {}, evtCSEStopError: {},
		evtGPOListApplicable: {},
	}
	got := newCollector().providers[guidGroupPolicy].acceptedIDs
	assert.Equal(t, want, got)

	// The non-boot activity starts stay out: 4002/4003 are network-state
	// change, 4004/4005 manual gpupdate, 4006/4007 periodic refresh. None of
	// them is the boot pass boot_timeline reports.
	for _, id := range []uint16{4002, 4003, 4004, 4005, 4006, 4007} {
		assert.NotContains(t, got, id, "non-boot activity start %d must not be collected", id)
	}
}

func TestActivityIDOfNilEventIsZero(t *testing.T) {
	assert.Equal(t, windows.GUID{}, activityIDOf(nil))
}

func TestEventPropertyReaderFallsBackOnDecodeFailure(t *testing.T) {
	// A 4016 carries seven properties, three of them long strings. TDH stops at
	// the first it cannot decode, and EventProperties returns what it recovered
	// alongside the error; the reader must use that partial result rather than
	// discard every field on the event.
	e := makeEvent(guidGroupPolicy, evtCSEStart, gpTestBoot,
		property{Name: "CSEExtensionId", Value: cseRegistryGUID},
		property{Name: "CSEExtensionName", Value: "Registry"},
	)
	e.propsErr = errors.New(`failed to parse property [2] "IsExtensionAsyncProcessing"`)

	prop := eventPropertyReader(e)
	assert.Equal(t, cseRegistryGUID, prop("CSEExtensionId"))
	assert.Equal(t, "Registry", prop("CSEExtensionName"))
	assert.Empty(t, prop("ApplicableGPOList"), "a property after the failure is absent, not garbage")
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
	// TDH renders win:Boolean as "true"/"false", but the same real trace showed
	// the sibling IsMachine field rendering as "1" and "0" on other events.
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
