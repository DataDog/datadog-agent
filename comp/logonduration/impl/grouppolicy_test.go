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

// Property values here are in the form TDH produces for each out-type declared by
// the Microsoft-Windows-GroupPolicy manifest: a braced GUID for win:GUID, decimal
// for win:UInt32, "0x…" for win:HexInt32, and "true"/"false" for win:Boolean.

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
	// Kernel-General 12 sets this in production; without it every offset is zero.
	f.coll.timeline.BootStart = gpTestBoot
	return f
}

// send dispatches one Group Policy event through processEvent, exercising the real provider routing.
func (f *gpFixture) send(activity windows.GUID, id uint16, offset time.Duration, props ...property) {
	f.t.Helper()
	e := makeEvent(guidGroupPolicy, id, gpTestBoot.Add(offset), props...)
	e.activityID = activity
	processEvent(f.coll, e)
}

// sendPartial dispatches an event whose TDH decode failed after the properties given.
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

// cseStop emits a 5016/6016/7016 extension stop. CSEExtensionId is declared last on this template.
func (f *gpFixture) cseStop(activity windows.GUID, offset time.Duration, id uint16, guid, name string) {
	f.send(activity, id, offset,
		property{Name: "CSEExtensionId", Value: guid},
		property{Name: "CSEExtensionName", Value: name},
		property{Name: "CSEElaspedTimeInMilliSeconds", Value: "999999"},
		property{Name: "ErrorCode", Value: "0x8000000A"},
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

// gpoFragment builds an ApplicableGPOList value: a rootless sequence of <GPO> siblings.
func gpoFragment(entries ...string) string {
	return strings.Join(entries, "")
}

// gpoEntry is one entry in the shape a real 4016 carries: ID attribute and Name only.
func gpoEntry(id, name string) string {
	return fmt.Sprintf(`<GPO ID="%s"><Name>%s</Name></GPO>`, id, name)
}

// gpoRichEntry is the fuller entry shape the 5312 inventory carries.
// Its <Extensions>[{CSE GUID}]</Extensions> holds a GUID a brace scan would misread as a GPO.
func gpoRichEntry(id, name string) string {
	return fmt.Sprintf(
		`<GPO ID="%s"><Name>%s</Name><Version>65539</Version><SOM>DC=corp,DC=example</SOM><FSPath>\\corp\SysVol</FSPath><Extensions>[{35378EAC-683F-11D2-A89A-00C04FBBCFA2}]</Extensions></GPO>`,
		id, name)
}

// --- Pairing and outcomes ---

func TestCSEPairingOutcomes(t *testing.T) {
	cases := []struct {
		name    string
		eventID uint16
		want    cseResult
	}{
		{"success", evtCSEStopSuccess, cseResultSuccess},
		{"warning", evtCSEStopWarning, cseResultWarning},
		{"error", evtCSEStopError, cseResultError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newGPFixture(t)
			f.startComputerPass()
			f.cseStart(gpTestActivity, 12500*time.Millisecond, cseRegistryGUID, "Registry", false, "")
			f.cseStop(gpTestActivity, 13750*time.Millisecond, tc.eventID, cseRegistryGUID, "Registry")

			invs := f.details().Computer
			require.Len(t, invs, 1)
			inv := invs[0]

			assert.Equal(t, cseRegistryGUID, inv.CSEID)
			assert.Equal(t, "Registry", inv.CSEName)
			assert.Equal(t, tc.want, inv.Result)
			assert.False(t, inv.Async)

			assert.Equal(t, int64(12500), inv.OffsetMs, "offset is boot-relative")
			assert.Equal(t, int64(1250), inv.DurationMs,
				"duration is the measured interval, not the provider's CSEElaspedTimeInMilliSeconds")
		})
	}
}

func TestCSEZeroDurationIsReported(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 13*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

	invs := f.details().Computer
	require.Len(t, invs, 1)
	inv := invs[0]
	assert.Equal(t, int64(0), inv.DurationMs)

	raw, err := json.Marshal(inv)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"duration_ms":0`)
}

func TestCSEAsyncIsFlagged(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseAuditGUID, "Audit Policy", true, "")
	f.cseStop(gpTestActivity, 13100*time.Millisecond, evtCSEStopSuccess, cseAuditGUID, "Audit Policy")

	invs := f.details().Computer
	require.Len(t, invs, 1)
	inv := invs[0]
	assert.True(t, inv.Async)
	assert.Equal(t, cseResultSuccess, inv.Result)
	assert.Equal(t, int64(100), inv.DurationMs)
}

func TestCSECrossActivityIsolation(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.startUserPass()

	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStart(gpUserActivity, 31*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 15*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")
	f.cseStop(gpUserActivity, 31500*time.Millisecond, evtCSEStopSuccess, cseRegistryGUID, "Registry")

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
		f.cseStop(gpTestActivity, start+100*time.Millisecond, evtCSEStopSuccess, guid, "ext")
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
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

	assert.Nil(t, f.finalize())
}

func TestCSEMissingTerminalIsOmitted(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, gpoDefaultDomainGUID)

	assert.Nil(t, f.finalize())
}

func TestCSETraceEndsMidInvocation(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")
	f.cseStart(gpTestActivity, 15*time.Second, cseFolderRedGUID, "Folder Redirection", false, "")

	invs := f.details().Computer
	require.Len(t, invs, 1)
	assert.Equal(t, cseRegistryGUID, invs[0].CSEID)
}

func TestCSEDuplicateOpenStartsNeverGuess(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStart(gpTestActivity, 14*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 16*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

	invs := f.details().Computer
	require.Len(t, invs, 1)
	assert.Equal(t, int64(2000), invs[0].DurationMs, "measured from the later start")
	assert.Equal(t, int64(14000), invs[0].OffsetMs)
}

func TestCSEStopBeforeStartIsDropped(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 16*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 15*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

	assert.Nil(t, f.finalize())
}

func TestCSEMissingExtensionIDIsDropped(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.send(gpTestActivity, evtCSEStart, 13*time.Second,
		property{Name: "CSEExtensionName", Value: "Registry"})
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

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
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

	f.send(gpOtherRun, 4004, 40*time.Second)
	f.cseStart(gpOtherRun, 41*time.Second, cseFolderRedGUID, "Folder Redirection", false, "")
	f.cseStop(gpOtherRun, 42*time.Second, evtCSEStopSuccess, cseFolderRedGUID, "Folder Redirection")

	d := f.details()
	require.Len(t, d.Computer, 1, "only the boot pass invocation is reported")
	assert.Equal(t, cseRegistryGUID, d.Computer[0].CSEID)
	assert.Empty(t, d.User)
}

func TestInvocationBackstopKeepsTheLeastHealthyAndTheSlowest(t *testing.T) {
	const extra = 10

	f := newGPFixture(t)
	f.startComputerPass()

	cseID := func(i int) string { return fmt.Sprintf("{%08X-683F-11D2-A89A-00C04FBBCFA2}", i) }

	for i := 0; i < maxCSEInvocationsPerScope+extra; i++ {
		start := 13*time.Second + time.Duration(i)*100*time.Millisecond
		// Duration grows with i, so index 0 is the fastest and also the only failure.
		stop := start + time.Duration(i+1)*time.Millisecond
		result := evtCSEStopSuccess
		if i == 0 {
			result = evtCSEStopError
		}
		f.cseStart(gpTestActivity, start, cseID(i), "Extension", false, "")
		f.cseStop(gpTestActivity, stop, result, cseID(i), "Extension")
	}

	d := f.details()
	require.Len(t, d.Computer, maxCSEInvocationsPerScope)
	assert.Equal(t, extra, d.ComputerCSEsOmitted, "the count the payload carries is what the cap cut")
	assert.Zero(t, d.UserCSEsOmitted, "the count belongs to the scope that overflowed, not to the block")

	raw, err := json.Marshal(d)
	require.NoError(t, err)
	assert.Contains(t, string(raw), fmt.Sprintf(`"computer_cses_omitted":%d`, extra),
		"a truncated pass says so on the wire")
	assert.NotContains(t, string(raw), "user_cses_omitted", "the untruncated scope stays silent")

	kept := make(map[string]CSEInvocation, len(d.Computer))
	for _, inv := range d.Computer {
		kept[inv.CSEID] = inv
	}

	require.Contains(t, kept, cseID(0), "a non-success outcome is kept regardless of how fast it was")
	assert.Equal(t, cseResultError, kept[cseID(0)].Result)

	for i := 1; i <= extra; i++ {
		assert.NotContains(t, kept, cseID(i), "the fastest successful invocations are the ones dropped")
	}
	for i := extra + 1; i < maxCSEInvocationsPerScope+extra; i++ {
		assert.Contains(t, kept, cseID(i))
	}

	for i := 1; i < len(d.Computer); i++ {
		assert.LessOrEqual(t, d.Computer[i-1].OffsetMs, d.Computer[i].OffsetMs,
			"the block is ordered chronologically no matter how selection ranked it")
	}
}

func TestCSEWithNoBootPassIsOmitted(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.startUserPass()
	f.cseStart(gpOtherRun, 40*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpOtherRun, 41*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

	assert.Nil(t, f.finalize())
}

func TestPassActivityPinnedFirstWriteWins(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.send(gpOtherRun, evtMachineGPStart, 40*time.Second)

	f.cseStart(gpOtherRun, 41*time.Second, cseFolderRedGUID, "Folder Redirection", false, "")
	f.cseStop(gpOtherRun, 42*time.Second, evtCSEStopSuccess, cseFolderRedGUID, "Folder Redirection")
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

	invs := f.details().Computer
	require.Len(t, invs, 1)
	assert.Equal(t, cseRegistryGUID, invs[0].CSEID)
}

func TestZeroActivityIDNeverPins(t *testing.T) {
	f := newGPFixture(t)
	f.send(windows.GUID{}, evtMachineGPStart, 12*time.Second)
	f.cseStart(windows.GUID{}, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(windows.GUID{}, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

	assert.Nil(t, f.finalize())
}

func TestPassStopAloneNeverPins(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.endComputerPass()

	f.cseStart(gpUserActivity, 31*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpUserActivity, 32*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")
	f.send(gpUserActivity, evtUserGPEnd, 33*time.Second)

	assert.Nil(t, f.finalize())
	assert.False(t, f.coll.timeline.UserGPEnd.IsZero(), "the stop still sets its timeline field")
	assert.True(t, f.coll.timeline.UserGPStart.IsZero(), "no user pass start was observed")
}

func TestPassActivityIsNotSharedBetweenScopes(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.send(gpTestActivity, evtUserGPStart, 30*time.Second)

	f.cseStart(gpTestActivity, 31*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 32*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

	d := f.details()
	require.Len(t, d.Computer, 1)
	assert.Empty(t, d.User, "the user scope never claimed the computer pass's activity")
}

// --- Offsets share the boot_timeline axis ---

func TestCSEOffsetsShareTheBootTimelineAxis(t *testing.T) {
	f := newGPFixture(t)
	f.coll.timeline.LoginUIDone = gpTestBoot.Add(10 * time.Second)
	f.coll.timeline.SessionLogon = gpTestBoot.Add(70 * time.Second)

	f.send(gpUserActivity, evtUserGPStart, 71*time.Second)
	f.cseStart(gpUserActivity, 72*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpUserActivity, 73*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")
	f.send(gpUserActivity, evtUserGPEnd, 75*time.Second)

	invs := f.details().User
	require.Len(t, invs, 1)
	inv := invs[0]

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

func assertNested(t *testing.T, inv CSEInvocation, parent Milestone) {
	t.Helper()
	assert.GreaterOrEqualf(t, float64(inv.OffsetMs), parent.OffsetMs,
		"%s starts at or after %s", inv.CSEName, parent.ID)
	assert.LessOrEqualf(t, float64(inv.OffsetMs+inv.DurationMs), parent.OffsetMs+parent.DurationMs,
		"%s ends at or before %s", inv.CSEName, parent.ID)
}

func TestCSEOffsetsKeepPassesInOrderAcrossTheLoginScreen(t *testing.T) {
	f := newGPFixture(t)
	f.coll.timeline.LoginUIDone = gpTestBoot.Add(10 * time.Second)
	f.coll.timeline.SessionLogon = gpTestBoot.Add(30 * time.Second)

	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")
	f.cseStart(gpTestActivity, 15*time.Second, cseFolderRedGUID, "Folder Redirection", false, "")
	f.cseStop(gpTestActivity, 17*time.Second, evtCSEStopSuccess, cseFolderRedGUID, "Folder Redirection")
	f.endComputerPass()

	f.send(gpUserActivity, evtUserGPStart, 31*time.Second)
	f.cseStart(gpUserActivity, 31500*time.Millisecond, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpUserActivity, 32*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")
	f.send(gpUserActivity, evtUserGPEnd, 33*time.Second)

	d := f.details()
	require.Len(t, d.Computer, 2)
	require.Len(t, d.User, 1)

	milestones := make(map[string]Milestone)
	for _, m := range buildTimelineMilestones(f.coll.timeline) {
		milestones[m.ID] = m
	}
	computer, user := milestones["computer_group_policy"], milestones["user_group_policy"]

	// 2000ms before the pass and 10000ms after it are elided; the pass itself is not.
	assert.Equal(t, float64(10000), computer.OffsetMs)
	assert.Equal(t, float64(19000), user.OffsetMs)

	for _, inv := range d.Computer {
		assertNested(t, inv, computer)
	}
	for _, inv := range d.User {
		assertNested(t, inv, user)
	}

	lastComputer := d.Computer[len(d.Computer)-1]
	assert.Less(t, lastComputer.OffsetMs+lastComputer.DurationMs, d.User[0].OffsetMs,
		"the computer pass and its extensions finish before the user pass begins")
}

// --- Group Policy objects ---

func TestGPOInlinedPerInvocation(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()

	list := gpoFragment(gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy"))
	for i, guid := range []string{cseRegistryGUID, cseFolderRedGUID, cseAuditGUID} {
		start := time.Duration(13+i) * time.Second
		f.cseStart(gpTestActivity, start, guid, "ext", false, list)
		f.cseStop(gpTestActivity, start+500*time.Millisecond, evtCSEStopSuccess, guid, "ext")
	}

	invs := f.details().Computer
	require.Len(t, invs, 3)
	for _, inv := range invs {
		require.Len(t, inv.GPOs, 1)
		assert.Equal(t, gpoDefaultDomainGUID, inv.GPOs[0].ID)
		assert.Equal(t, "Default Domain Policy", inv.GPOs[0].Name)
	}
}

func TestOmittedGPOCountReachesTheInvocation(t *testing.T) {
	const extra = 7

	f := newGPFixture(t)
	f.startComputerPass()

	entries := make([]string, 0, maxGPOsPerCSE+extra)
	for i := 0; i < maxGPOsPerCSE+extra; i++ {
		entries = append(entries, gpoEntry(fmt.Sprintf("{%08X-016D-11D2-945F-00C04FB984F9}", i), "Policy"))
	}

	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, gpoFragment(entries...))
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

	invs := f.details().Computer
	require.Len(t, invs, 1)
	assert.Len(t, invs[0].GPOs, maxGPOsPerCSE)
	assert.Equal(t, extra, invs[0].GPOsOmitted)
}

func TestOmittedGPOCountAbsentWhenNothingDropped(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false,
		gpoFragment(gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy")))
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

	invs := f.details().Computer
	require.Len(t, invs, 1)
	assert.Zero(t, invs[0].GPOsOmitted)
}

func TestGPONamesComeFromTheApplicableList(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false,
		gpoFragment(gpoRichEntry(gpoDefaultDomainGUID, "Default Domain Policy")))
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

	invs := f.details().Computer
	require.Len(t, invs, 1)
	gpos := invs[0].GPOs
	require.Len(t, gpos, 1, "the extension GUID inside <Extensions> is not a GPO")
	assert.Equal(t, gpoDefaultDomainGUID, gpos[0].ID)
	assert.Equal(t, "Default Domain Policy", gpos[0].Name)
}

func TestGPONamesSurviveUnescapedAmpersand(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false,
		gpoFragment(
			gpoEntry(gpoDefaultDomainGUID, "R&D Baseline"),
			gpoEntry(gpoDomainCtlGUID, "Sales & Marketing"),
			gpoEntry(gpoThirdGUID, "Plain Name"),
		))
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

	invs := f.details().Computer
	require.Len(t, invs, 1)
	assert.Equal(t, []GPORef{
		{ID: gpoDefaultDomainGUID, Name: "R&D Baseline"},
		{ID: gpoDomainCtlGUID, Name: "Sales & Marketing"},
		{ID: gpoThirdGUID, Name: "Plain Name"},
	}, invs[0].GPOs)
}

func TestGPONameSharedFromAnotherInvocationsList(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()

	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, gpoDefaultDomainGUID)
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

	f.cseStart(gpTestActivity, 15*time.Second, cseFolderRedGUID, "Folder Redirection", false,
		gpoFragment(gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy")))
	f.cseStop(gpTestActivity, 16*time.Second, evtCSEStopSuccess, cseFolderRedGUID, "Folder Redirection")

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
	f.cseStop(gpTestActivity, 12200*time.Millisecond, evtCSEStopSuccess, cseAuditGUID, "Audit")

	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false,
		gpoDefaultDomainGUID+";"+gpoDomainCtlGUID+";"+gpoThirdGUID)
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

	invs := f.details().Computer
	require.Len(t, invs, 2)
	require.Equal(t, cseRegistryGUID, invs[1].CSEID, "the chronological sort puts Registry second")
	gpos := invs[1].GPOs
	require.Len(t, gpos, 3)
	assert.Equal(t, "Default Domain Policy", gpos[0].Name)
	assert.Equal(t, "Default Domain Controllers Policy", gpos[1].Name)
	assert.Equal(t, gpoThirdGUID, gpos[2].ID)
	assert.Empty(t, gpos[2].Name, "no list named this one, so the ID stands alone")
}

func TestGPODuplicateDisplayNamesStayDistinct(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, gpoFragment(
		gpoEntry(gpoDefaultDomainGUID, "Baseline"),
		gpoEntry(gpoDomainCtlGUID, "Baseline"),
	))
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

	invs := f.details().Computer
	require.Len(t, invs, 1)
	gpos := invs[0].GPOs
	require.Len(t, gpos, 2)
	assert.NotEqual(t, gpos[0].ID, gpos[1].ID)
	assert.Equal(t, "Baseline", gpos[0].Name)
	assert.Equal(t, "Baseline", gpos[1].Name)
}

func TestGPONamesWithoutASurvivingInvocationEmitNothing(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false,
		gpoFragment(gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy")))

	assert.Nil(t, f.finalize())
}

func TestGPORefsFromList(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantIDs   []string
		wantNames map[string]string
	}{
		{name: "empty"},
		{
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
			ids, names, omitted := gpoRefsFromList(tc.raw)
			assert.Equal(t, tc.wantIDs, ids)
			assert.Equal(t, tc.wantNames, names)
			assert.Zero(t, omitted, "no case here reaches the per-invocation cap")
		})
	}

	t.Run("a list past any plausible size still yields references", func(t *testing.T) {
		raw := strings.Repeat("x", 256*1024) + gpoEntry(gpoDefaultDomainGUID, "Late Policy")
		ids, names, omitted := gpoRefsFromList(raw)
		assert.Equal(t, []string{gpoDefaultDomainGUID}, ids)
		assert.Equal(t, map[string]string{gpoDefaultDomainGUID: "Late Policy"}, names)
		assert.Zero(t, omitted)
	})

	t.Run("references are bounded per invocation and the remainder is counted", func(t *testing.T) {
		const extra = 20
		var b strings.Builder
		for i := 0; i < maxGPOsPerCSE+extra; i++ {
			fmt.Fprintf(&b, "{%08X-0000-0000-0000-000000000000};", i)
		}
		ids, _, omitted := gpoRefsFromList(b.String())
		assert.Len(t, ids, maxGPOsPerCSE)
		assert.Equal(t, extra, omitted, "every distinct reference past the cap is counted")
	})

	t.Run("names are bounded with the ids they belong to", func(t *testing.T) {
		const extra = 20
		var b strings.Builder
		for i := 0; i < maxGPOsPerCSE+extra; i++ {
			b.WriteString(gpoEntry(fmt.Sprintf("{%08X-0000-0000-0000-000000000000}", i), "Policy"))
		}
		ids, names, omitted := gpoRefsFromList(b.String())
		assert.Len(t, ids, maxGPOsPerCSE)
		assert.Len(t, names, maxGPOsPerCSE, "a name is only kept for an ID that was taken")
		assert.Equal(t, extra, omitted)
	})

	t.Run("a reference repeated past the cap counts once", func(t *testing.T) {
		var b strings.Builder
		for i := 0; i < maxGPOsPerCSE; i++ {
			fmt.Fprintf(&b, "{%08X-0000-0000-0000-000000000000};", i)
		}
		const beyondCap = "{FFFFFFFF-0000-0000-0000-000000000000};"
		for i := 0; i < 50; i++ {
			b.WriteString(beyondCap)
		}
		ids, _, omitted := gpoRefsFromList(b.String())
		assert.Len(t, ids, maxGPOsPerCSE)
		assert.Equal(t, 1, omitted, "one GPO was lost, not fifty")
	})
}

// --- Collection gate ---

func TestAcceptedGroupPolicyIDsSnapshot(t *testing.T) {
	want := map[uint16]struct{}{
		evtMachineGPStart: {}, evtMachineGPEnd: {},
		evtUserGPStart: {}, evtUserGPEnd: {},
		evtCSEStart:       {},
		evtCSEStopSuccess: {}, evtCSEStopWarning: {}, evtCSEStopError: {},
	}
	got := newCollector().providers[guidGroupPolicy].acceptedIDs
	assert.Equal(t, want, got)

	// 4002/4003 network-state change, 4004/4005 manual, 4006/4007 periodic refresh: each
	// its own pass, not boot. 5312/5313 are the GPO inventories 4016's own list replaces.
	for _, id := range []uint16{4002, 4003, 4004, 4005, 4006, 4007, 5312, 5313} {
		assert.NotContains(t, got, id, "event %d must not be collected", id)
	}
}

func TestActivityIDOfNilEventIsZero(t *testing.T) {
	assert.Equal(t, windows.GUID{}, activityIDOf(nil))
}

func TestEventPropertyReaderUsesThePartialBulkDecode(t *testing.T) {
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
	t.Run("a partial start keeps its identity and interval", func(t *testing.T) {
		f := newGPFixture(t)
		f.startComputerPass()
		f.sendPartial(gpTestActivity, evtCSEStart, 13*time.Second, "IsExtensionAsyncProcessing",
			property{Name: "CSEExtensionId", Value: cseRegistryGUID},
			property{Name: "CSEExtensionName", Value: "Registry"},
		)
		f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry")

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

func TestParseETWBool(t *testing.T) {
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

	multibyte := strings.Repeat("日", maxCSENameBytes)
	got = truncateProviderText(multibyte, maxCSENameBytes)
	assert.LessOrEqual(t, len(got), maxCSENameBytes)
	assert.True(t, utf8.ValidString(got))
}

func TestGPONamesSurviveNonLatinScripts(t *testing.T) {
	cases := []struct {
		name  string
		runes string
		count int
	}{
		{name: "three-byte script", runes: "本", count: 120},
		{name: "two-byte script", runes: "П", count: 256},
		{name: "latin at the AD character limit", runes: "A", count: 256},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := strings.Repeat(tc.runes, tc.count)
			_, names, _ := gpoRefsFromList(gpoEntry(gpoDefaultDomainGUID, want))

			got := names[gpoDefaultDomainGUID]
			assert.Equal(t, want, got, "an ordinary %s name must reach the wire whole", tc.name)
			assert.NotContains(t, got, "...", "nothing about this name should have been dropped")
		})
	}
}

func TestGPONameTruncationKeepsValidUTF8(t *testing.T) {
	oversize := strings.Repeat("本", maxGPONameBytes)
	_, names, _ := gpoRefsFromList(gpoEntry(gpoDefaultDomainGUID, oversize))

	got := names[gpoDefaultDomainGUID]
	assert.LessOrEqual(t, len(got), maxGPONameBytes)
	assert.True(t, utf8.ValidString(got), "a cap that splits a rune emits invalid UTF-8")
	assert.True(t, strings.HasSuffix(got, "..."))
}
