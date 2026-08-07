// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && test

package logondurationimpl

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// Property values in these tests are the exact strings TDH produces for each
// declared out-type: a braced uppercase GUID, "0x…" for win:HexInt32, decimal
// for win:UInt32, and "true"/"false" for win:Boolean. Asserting against the
// real formatting is what catches a base-10 error-code parse, which is the most
// likely bug in this collection path.

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
)

// gpFixture drives synthetic Group Policy events through the real parser.
type gpFixture struct {
	t    *testing.T
	coll *collector
}

func newGPFixture(t *testing.T) *gpFixture {
	t.Helper()
	return &gpFixture{t: t, coll: newCollector()}
}

// send dispatches one Group Policy event through processEvent, so every test
// exercises the real provider routing rather than calling a parser directly.
func (f *gpFixture) send(activity windows.GUID, id uint16, offset time.Duration, props ...property) {
	f.t.Helper()
	e := makeEvent(guidGroupPolicy, id, gpTestBoot.Add(offset), props...)
	e.activityID = activity
	processEvent(f.coll, e)
}

// startComputerPass emits the computer-scope activity start (event 4000).
func (f *gpFixture) startComputerPass() {
	f.send(gpTestActivity, evtMachineGPStart, 12*time.Second)
}

// startUserPass emits the user-scope activity start (event 4001).
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
func (f *gpFixture) cseStop(activity windows.GUID, offset time.Duration, id uint16, guid, name string, elapsedMs uint32, errorCode string) {
	f.send(activity, id, offset,
		property{Name: "CSEExtensionId", Value: guid},
		property{Name: "CSEExtensionName", Value: name},
		property{Name: "CSEElaspedTimeInMilliSeconds", Value: strconv.FormatUint(uint64(elapsedMs), 10)},
		property{Name: "ErrorCode", Value: errorCode},
	)
}

// gpoInventory emits a 5312 applicable-GPO list.
func (f *gpFixture) gpoInventory(activity windows.GUID, offset time.Duration, list string) {
	f.send(activity, evtGPOListApplicable, offset, property{Name: "GPOInfoList", Value: list})
}

func (f *gpFixture) payload() *GroupPolicyPayload {
	f.t.Helper()
	p := f.coll.groupPolicy.finalize()
	require.NotNil(f.t, p, "expected a group policy payload")
	return p
}

// gpoFragment builds a GPOInfoList value: a rootless sequence of <GPO> siblings.
func gpoFragment(entries ...string) string {
	return strings.Join(entries, "")
}

func gpoEntry(id, name, som string) string {
	return fmt.Sprintf(
		`<GPO ID="%s"><Name>%s</Name><Version>65539</Version><SOM>%s</SOM><FSPath>\\corp\SysVol</FSPath><Extensions>[{35378EAC-683F-11D2-A89A-00C04FBBCFA2}]</Extensions></GPO>`,
		id, name, som)
}

func findInvocation(t *testing.T, invs []CSEInvocation, guid string) CSEInvocation {
	t.Helper()
	for _, inv := range invs {
		if inv.CSEGUID == guid {
			return inv
		}
	}
	t.Fatalf("no invocation for %s in %+v", guid, invs)
	return CSEInvocation{}
}

// --- Scope resolution ---

func TestGPScopeFromActivityStart(t *testing.T) {
	// The 4000-4007 range alternates by parity across boot, network change,
	// manual gpupdate, and periodic refresh.
	for _, tc := range []struct {
		id   uint16
		want gpScope
	}{
		{4000, gpScopeComputer}, {4001, gpScopeUser},
		{4002, gpScopeComputer}, {4003, gpScopeUser},
		{4004, gpScopeComputer}, {4005, gpScopeUser},
		{4006, gpScopeComputer}, {4007, gpScopeUser},
	} {
		got, ok := scopeForActivityStart(tc.id)
		assert.True(t, ok, "event %d should be an activity start", tc.id)
		assert.Equal(t, tc.want, got, "event %d scope", tc.id)
	}

	for _, id := range []uint16{3999, 4008, 8000, evtCSEStart} {
		_, ok := scopeForActivityStart(id)
		assert.False(t, ok, "event %d should not be an activity start", id)
	}
}

func TestGPNonBootActivityStartDoesNotMarkPassObserved(t *testing.T) {
	f := newGPFixture(t)
	// A mid-trace gpupdate seeds the scope but is not the boot pass the
	// timeline reports, so the pass must not be reported as observed.
	f.send(gpTestActivity, 4004, 40*time.Second)
	f.cseStart(gpTestActivity, 41*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 42*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", 1000, "0x0")

	p := f.payload()
	assert.False(t, p.Passes.Computer.Observed, "gpupdate must not mark the boot pass observed")
	assert.Len(t, p.Passes.Computer.CSEInvocations, 1, "its invocation is still attributed to computer scope")
}

// --- Pairing ---

func TestCSEPairingOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		stopID    uint16
		errorCode string
		wantCode  string
		want      cseResult
	}{
		// The emitted code is re-rendered in a canonical width, so "0x0"
		// becomes "0x00000000" rather than being passed through verbatim.
		{"success", evtCSEStopSuccess, "0x0", "0x00000000", cseResultSuccess},
		{"warning", evtCSEStopWarning, "0x00000534", "0x00000534", cseResultWarning},
		{"error", evtCSEStopError, "0x8007000E", "0x8007000E", cseResultError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGPFixture(t)
			f.startComputerPass()
			f.cseStart(gpTestActivity, 12500*time.Millisecond, cseRegistryGUID, "Registry", false, "")
			f.cseStop(gpTestActivity, 13750*time.Millisecond, tc.stopID, cseRegistryGUID, "Registry", 1248, tc.errorCode)

			p := f.payload()
			require.Len(t, p.Passes.Computer.CSEInvocations, 1)
			inv := p.Passes.Computer.CSEInvocations[0]

			assert.Equal(t, cseRegistryGUID, inv.CSEGUID)
			assert.Equal(t, "Registry", inv.CSEName)
			assert.Equal(t, tc.want, inv.Result)
			assert.True(t, inv.Complete)
			assert.False(t, inv.IsAsync)
			assert.False(t, inv.MissingStart)
			assert.Equal(t, "2026-01-15T08:00:12.500Z", inv.Start)
			assert.Equal(t, "2026-01-15T08:00:13.750Z", inv.End)

			// Wall-clock delta and the provider's own measurement are reported
			// separately and must not be reconciled.
			require.NotNil(t, inv.DurationMs)
			assert.Equal(t, int64(1250), *inv.DurationMs)
			require.NotNil(t, inv.ReportedElapsedMs)
			assert.Equal(t, uint32(1248), *inv.ReportedElapsedMs)

			// ErrorCode is parsed with base 0 and re-rendered canonically. A
			// base-10 parse would drop every non-zero code here.
			require.NotNil(t, inv.ErrorCode)
			assert.Equal(t, tc.wantCode, *inv.ErrorCode)
		})
	}
}

func TestCSEZeroDurationIsReported(t *testing.T) {
	// A sub-millisecond extension has a real duration of zero, which is not the
	// same as unmeasured. The pointer field is what preserves that distinction.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 13*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", 0, "0x0")

	inv := f.payload().Passes.Computer.CSEInvocations[0]
	require.NotNil(t, inv.DurationMs)
	assert.Equal(t, int64(0), *inv.DurationMs)

	encoded, err := json.Marshal(inv)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"duration_ms":0`, "a measured zero must survive serialization")
}

func TestCSEAsyncDispatchNeverClaimsDuration(t *testing.T) {
	// For an asynchronous extension the stop event marks thread dispatch, not
	// completion, and E_PENDING there is documented as expected behaviour.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseAuditGUID, "Audit Policy Configuration", true, "")
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseAuditGUID, "Audit Policy Configuration", 5, "0x8000000A")

	inv := f.payload().Passes.Computer.CSEInvocations[0]
	assert.True(t, inv.IsAsync)
	assert.Equal(t, cseResultDispatched, inv.Result)
	assert.False(t, inv.Complete)
	assert.Nil(t, inv.DurationMs, "an async dispatch has no total extension duration")
	assert.NotEqual(t, cseResultError, inv.Result, "E_PENDING on an async dispatch is not a failure")
	require.NotNil(t, inv.ErrorCode)
	assert.Equal(t, "0x8000000A", *inv.ErrorCode)
}

func TestCSEMissingStart(t *testing.T) {
	// A stop with no matching start still carries the provider's own elapsed
	// time, which is real data even though the interval was not observed.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", 900, "0x0")

	p := f.payload()
	require.Len(t, p.Passes.Computer.CSEInvocations, 1)
	inv := p.Passes.Computer.CSEInvocations[0]

	assert.True(t, inv.MissingStart)
	assert.False(t, inv.Complete)
	assert.Empty(t, inv.Start)
	assert.Nil(t, inv.DurationMs)
	require.NotNil(t, inv.ReportedElapsedMs)
	assert.Equal(t, uint32(900), *inv.ReportedElapsedMs)
	assert.Equal(t, 1, p.Passes.Computer.IncompleteInvocations)
}

func TestCSEMissingTerminalRetainsGPOReferences(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false,
		gpoFragment(gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy", "DC=corp")))

	p := f.payload()
	require.Len(t, p.Passes.Computer.CSEInvocations, 1)
	inv := p.Passes.Computer.CSEInvocations[0]

	assert.Equal(t, cseResultUnknown, inv.Result)
	assert.False(t, inv.Complete)
	assert.Empty(t, inv.End)
	assert.Nil(t, inv.DurationMs)
	assert.Equal(t, []string{gpoDefaultDomainGUID}, inv.ApplicableGPOIDs)
	assert.Equal(t, 1, p.Passes.Computer.IncompleteInvocations)
}

func TestCSEDuplicateOpenStartsNeverGuess(t *testing.T) {
	// Two starts for the same key cannot be told apart, so the first is closed
	// as incomplete rather than letting the later stop attach to the wrong one.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStart(gpTestActivity, 14*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 15*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", 1000, "0x0")

	p := f.payload()
	require.Len(t, p.Passes.Computer.CSEInvocations, 2, "both starts must be reported")

	first, second := p.Passes.Computer.CSEInvocations[0], p.Passes.Computer.CSEInvocations[1]
	assert.Equal(t, "2026-01-15T08:00:13.000Z", first.Start)
	assert.False(t, first.Complete, "the ambiguous first start is not completed by a later stop")
	assert.Equal(t, cseResultUnknown, first.Result)

	assert.Equal(t, "2026-01-15T08:00:14.000Z", second.Start)
	assert.True(t, second.Complete)
	require.NotNil(t, second.DurationMs)
	assert.Equal(t, int64(1000), *second.DurationMs)
}

func TestCSECrossActivityIsolation(t *testing.T) {
	// The same extension runs in both passes. Correlating on activity ID keeps
	// them separate; pairing on the extension GUID alone would cross them.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.startUserPass()
	f.cseStart(gpUserActivity, 31*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpUserActivity, 32*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", 1000, "0x0")
	f.cseStop(gpTestActivity, 33*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", 20000, "0x0")

	p := f.payload()
	require.Len(t, p.Passes.Computer.CSEInvocations, 1)
	require.Len(t, p.Passes.User.CSEInvocations, 1)

	computer := p.Passes.Computer.CSEInvocations[0]
	user := p.Passes.User.CSEInvocations[0]

	// Each stop closed its own activity's start, not the other's.
	require.NotNil(t, computer.DurationMs)
	assert.Equal(t, int64(20000), *computer.DurationMs)
	require.NotNil(t, user.DurationMs)
	assert.Equal(t, int64(1000), *user.DurationMs)
}

func TestCSETraceEndsMidInvocation(t *testing.T) {
	// The capture window closes when the Agent service starts, so a trace
	// routinely ends with invocations still open.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStart(gpTestActivity, 14*time.Second, cseFolderRedGUID, "Folder Redirection", false, "")
	f.cseStop(gpTestActivity, 15*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", 2000, "0x0")

	p := f.payload()
	require.Len(t, p.Passes.Computer.CSEInvocations, 2)
	assert.Equal(t, 1, p.Passes.Computer.IncompleteInvocations)

	stranded := findInvocation(t, p.Passes.Computer.CSEInvocations, cseFolderRedGUID)
	assert.False(t, stranded.Complete)
	assert.Equal(t, cseResultUnknown, stranded.Result)
	assert.Empty(t, stranded.End)
}

func TestCSEUnattributedActivity(t *testing.T) {
	// No activity start was observed, so the scope is unknown. The invocation
	// is reported separately rather than guessed into a pass.
	f := newGPFixture(t)
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", 1000, "0x0")

	p := f.payload()
	assert.Empty(t, p.Passes.Computer.CSEInvocations)
	assert.Empty(t, p.Passes.User.CSEInvocations)
	require.Len(t, p.Unattributed, 1)
	assert.Equal(t, cseRegistryGUID, p.Unattributed[0].CSEGUID)
}

func TestCSEMissingExtensionIDIsCounted(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.send(gpTestActivity, evtCSEStart, 13*time.Second,
		property{Name: "CSEExtensionName", Value: "Registry"})

	p := f.payload()
	assert.Empty(t, p.Passes.Computer.CSEInvocations, "an invocation with no identity cannot be paired")
	assert.Equal(t, 1, p.ParseErrors)
}

// --- Pass structure ---

func TestPassesUserUnobserved(t *testing.T) {
	// The acceptance criterion: a computer-only trace must state that the user
	// pass was not observed, so absence is never read as "this host has none".
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", 1000, "0x0")

	p := f.payload()
	assert.True(t, p.Passes.Computer.Observed)
	assert.False(t, p.Passes.User.Observed)

	encoded, err := json.Marshal(p)
	require.NoError(t, err)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(encoded, &decoded))

	passes := decoded["passes"].(map[string]interface{})
	user := passes["user"].(map[string]interface{})

	// Both keys must be present. An omitted user pass would be indistinguishable
	// from a pass that ran and invoked nothing.
	assert.Contains(t, passes, "computer")
	assert.Contains(t, user, "observed")
	assert.Equal(t, false, user["observed"])
	assert.Equal(t, []interface{}{}, user["cse_invocations"], "must serialize as [] rather than null")
}

func TestPassTimingsAreNotDuplicated(t *testing.T) {
	// Pass start/end/duration live solely in boot_timeline and durations. A
	// second copy here could drift from them.
	f := newGPFixture(t)
	f.startComputerPass()
	f.send(gpTestActivity, evtMachineGPEnd, 20*time.Second)

	encoded, err := json.Marshal(f.payload())
	require.NoError(t, err)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	computer := decoded["passes"].(map[string]interface{})["computer"].(map[string]interface{})

	for _, key := range []string{"start", "end", "duration_ms"} {
		assert.NotContains(t, computer, key, "pass timings belong to boot_timeline, not group_policy")
	}
}

// --- Group Policy objects ---

func TestGPOMultiplePerCSE(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, gpoFragment(
		gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy", "DC=corp,DC=example"),
		gpoEntry(gpoDomainCtlGUID, "Default Domain Controllers Policy", "OU=Domain Controllers"),
	))

	p := f.payload()
	inv := p.Passes.Computer.CSEInvocations[0]
	assert.Equal(t, []string{gpoDefaultDomainGUID, gpoDomainCtlGUID}, inv.ApplicableGPOIDs)

	require.Len(t, p.GPOs, 2)
	assert.Equal(t, "Default Domain Policy", p.GPOs[0].Name)
	assert.Equal(t, "DC=corp,DC=example", p.GPOs[0].SOM)
	assert.Equal(t, "65539", p.GPOs[0].Version)
}

func TestGPOSharedAcrossCSEs(t *testing.T) {
	// The many-to-many case. One GPO feeding three invocations across both
	// passes must produce exactly one metadata entry, referenced three times.
	list := gpoFragment(gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy", "DC=corp"))

	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, list)
	f.cseStart(gpTestActivity, 14*time.Second, cseFolderRedGUID, "Folder Redirection", false, list)
	f.startUserPass()
	f.cseStart(gpUserActivity, 31*time.Second, cseRegistryGUID, "Registry", false, list)

	p := f.payload()
	require.Len(t, p.GPOs, 1, "metadata is stored once, not repeated per invocation")
	assert.Equal(t, gpoDefaultDomainGUID, p.GPOs[0].ID)

	references := 0
	for _, pass := range []GPPass{p.Passes.Computer, p.Passes.User} {
		for _, inv := range pass.CSEInvocations {
			assert.Equal(t, []string{gpoDefaultDomainGUID}, inv.ApplicableGPOIDs)
			references++
		}
	}
	assert.Equal(t, 3, references)
}

func TestGPODuplicateDisplayNamesStayDistinct(t *testing.T) {
	// The GUID is the identity. Two objects sharing a display name are two
	// objects.
	f := newGPFixture(t)
	f.startComputerPass()
	f.gpoInventory(gpTestActivity, 13*time.Second, gpoFragment(
		gpoEntry(gpoDefaultDomainGUID, "Shared Name", "DC=corp"),
		gpoEntry(gpoThirdGUID, "Shared Name", "OU=Servers"),
	))

	p := f.payload()
	require.Len(t, p.GPOs, 2)
	assert.NotEqual(t, p.GPOs[0].ID, p.GPOs[1].ID)
	assert.Equal(t, "Shared Name", p.GPOs[0].Name)
	assert.Equal(t, "Shared Name", p.GPOs[1].Name)
}

func TestGPOFromApplicableListOnlyGetsIDEntry(t *testing.T) {
	// A GPO reached only through an invocation's applicable list still gets an
	// entry, so every reference resolves even with no inventory event.
	f := newGPFixture(t)
	f.startComputerPass()
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false,
		gpoDefaultDomainGUID+" "+gpoDomainCtlGUID)

	p := f.payload()
	require.Len(t, p.GPOs, 2)
	for _, g := range p.GPOs {
		assert.NotEmpty(t, g.ID)
		assert.Empty(t, g.Name, "no inventory event supplied metadata")
	}
}

func TestGPOInventoryWithNoCSEObservations(t *testing.T) {
	// Inventory and invocations are separate events, so a truncated trace can
	// easily hold one without the other. That is not an error path.
	f := newGPFixture(t)
	f.startComputerPass()
	f.gpoInventory(gpTestActivity, 13*time.Second,
		gpoFragment(gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy", "DC=corp")))

	p := f.payload()
	require.Len(t, p.GPOs, 1)
	assert.Empty(t, p.Passes.Computer.CSEInvocations)
	assert.Empty(t, p.Passes.User.CSEInvocations)
	assert.Zero(t, p.ParseErrors)
}

func TestGPOMissingGUIDIsDropped(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.gpoInventory(gpTestActivity, 13*time.Second, gpoFragment(
		`<GPO><Name>No Identity</Name></GPO>`,
		`<GPO ID="not-a-guid"><Name>Bad Identity</Name></GPO>`,
		gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy", "DC=corp"),
	))

	p := f.payload()
	require.Len(t, p.GPOs, 1, "entries without a resolvable GUID are dropped")
	assert.Equal(t, gpoDefaultDomainGUID, p.GPOs[0].ID)
}

func TestGPOMalformedAndEmptyLists(t *testing.T) {
	t.Run("empty list yields nothing and is not an error", func(t *testing.T) {
		gpos, err := parseGPOList("")
		assert.NoError(t, err)
		assert.Empty(t, gpos)
	})

	t.Run("truncated fragment keeps what parsed", func(t *testing.T) {
		raw := gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy", "DC=corp") +
			`<GPO ID="` + gpoDomainCtlGUID + `"><Name>Truncated`
		gpos, err := parseGPOList(raw)
		assert.Error(t, err, "the truncated tail is reported")
		require.Len(t, gpos, 1, "the complete entry still contributes")
		assert.Equal(t, gpoDefaultDomainGUID, gpos[0].ID)
	})

	t.Run("garbage yields nothing and never panics", func(t *testing.T) {
		gpos, _ := parseGPOList("this is not xml at all <<<>>>")
		assert.Empty(t, gpos)
	})

	t.Run("rooted variant also parses", func(t *testing.T) {
		// The runtime shape is unsampled, so a wrapper element must not break
		// the parser.
		raw := "<GPOList>" + gpoEntry(gpoDefaultDomainGUID, "Default Domain Policy", "DC=corp") + "</GPOList>"
		gpos, err := parseGPOList(raw)
		assert.NoError(t, err)
		require.Len(t, gpos, 1)
		assert.Equal(t, gpoDefaultDomainGUID, gpos[0].ID)
	})

	t.Run("malformed inventory increments parse errors without failing the payload", func(t *testing.T) {
		f := newGPFixture(t)
		f.startComputerPass()
		f.gpoInventory(gpTestActivity, 13*time.Second, `<GPO ID="`+gpoDefaultDomainGUID+`"><Name>Unclosed`)

		p := f.payload()
		assert.Equal(t, 1, p.ParseErrors)
		assert.True(t, p.Passes.Computer.Observed, "the rest of the payload survives")
	})
}

func TestGPOOversizeListRejectedBeforeParsing(t *testing.T) {
	entry := gpoEntry(gpoDefaultDomainGUID, strings.Repeat("A", 512), "DC=corp")
	raw := strings.Repeat(entry, (maxGPOListBytes/len(entry))+2)
	require.Greater(t, len(raw), maxGPOListBytes)

	gpos, err := parseGPOList(raw)
	assert.ErrorIs(t, err, errGPOListTooLarge)
	assert.Empty(t, gpos)
}

func TestGPOListItemCap(t *testing.T) {
	var b strings.Builder
	for i := 0; i < maxGPOsPerList+25; i++ {
		b.WriteString(gpoEntry(fmt.Sprintf("{%08X-016D-11D2-945F-00C04FB984F9}", i), "GPO", "DC=corp"))
	}
	gpos, err := parseGPOList(b.String())
	assert.NoError(t, err)
	assert.Len(t, gpos, maxGPOsPerList)
}

func TestApplicableGPOListGUIDScanFallback(t *testing.T) {
	// ApplicableGPOList's runtime format has never been observed. When it is
	// not the XML fragment shape, a braced-GUID scan still recovers the
	// references regardless of the delimiter used.
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"semicolon separated", gpoDefaultDomainGUID + ";" + gpoDomainCtlGUID},
		{"newline separated", gpoDefaultDomainGUID + "\n" + gpoDomainCtlGUID},
		{"prose", "Applied " + gpoDefaultDomainGUID + " and " + gpoDomainCtlGUID + "."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ids, gpos, degraded := parseApplicableGPOList(tc.raw)
			assert.Equal(t, []string{gpoDefaultDomainGUID, gpoDomainCtlGUID}, ids)
			assert.Empty(t, gpos, "the fallback recovers identities only, not metadata")
			assert.False(t, degraded)
		})
	}

	t.Run("no recoverable identity is reported as degraded", func(t *testing.T) {
		ids, _, degraded := parseApplicableGPOList("nothing useful here")
		assert.Empty(t, ids)
		assert.True(t, degraded)
	})

	t.Run("duplicates collapse", func(t *testing.T) {
		ids, _, _ := parseApplicableGPOList(gpoDefaultDomainGUID + " " + gpoDefaultDomainGUID)
		assert.Equal(t, []string{gpoDefaultDomainGUID}, ids)
	})
}

// --- Helpers ---

func TestNormalizeGUID(t *testing.T) {
	want := gpoDefaultDomainGUID

	for _, in := range []string{
		gpoDefaultDomainGUID,
		strings.ToLower(gpoDefaultDomainGUID),
		strings.Trim(gpoDefaultDomainGUID, "{}"),
		"  " + gpoDefaultDomainGUID + "  ",
	} {
		_, got, ok := normalizeGUID(in)
		assert.True(t, ok, "%q should parse", in)
		assert.Equal(t, want, got, "%q should normalize to canonical braced uppercase", in)
	}

	for _, in := range []string{"", "   ", "not-a-guid", "{}", "{31B2F340}"} {
		_, _, ok := normalizeGUID(in)
		assert.False(t, ok, "%q should not parse", in)
	}
}

func TestFormatErrorCode(t *testing.T) {
	// win:HexInt32 means TDH renders these with a 0x prefix, so parsing must use
	// base 0. Base 10 would reject every non-zero code.
	for _, tc := range []struct{ in, want string }{
		{"0x8000000A", "0x8000000A"},
		{"0x0", "0x00000000"},
		{"0", "0x00000000"},
		{"5", "0x00000005"},
		{"0xFFFFFFFF", "0xFFFFFFFF"},
		{" 0x534 ", "0x00000534"},
	} {
		got, ok := formatErrorCode(tc.in)
		assert.True(t, ok, "%q should parse", tc.in)
		assert.Equal(t, tc.want, got)
	}

	for _, in := range []string{"", "not a number", "0x1FFFFFFFF"} {
		_, ok := formatErrorCode(in)
		assert.False(t, ok, "%q should not parse", in)
	}
}

func TestParseETWBool(t *testing.T) {
	// The exact token TDH emits for win:Boolean is not confirmed on every
	// Windows build, so the numeric spellings are accepted too.
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"true", true}, {"True", true}, {"TRUE", true}, {"1", true}, {"-1", true},
		{"false", false}, {"False", false}, {"0", false},
	} {
		got, ok := parseETWBool(tc.in)
		assert.True(t, ok, "%q should be recognized", tc.in)
		assert.Equal(t, tc.want, got, "%q", tc.in)
	}

	for _, in := range []string{"", "yes", "maybe"} {
		_, ok := parseETWBool(in)
		assert.False(t, ok, "%q should not be recognized", in)
	}
}

func TestTruncateProviderText(t *testing.T) {
	assert.Equal(t, "short", truncateProviderText("  short  ", 32))
	assert.LessOrEqual(t, len(truncateProviderText(strings.Repeat("x", 500), 32)), 32)
	// A multi-byte rune straddling the cut must not leave invalid UTF-8.
	out := truncateProviderText(strings.Repeat("é", 100), 33)
	assert.LessOrEqual(t, len(out), 33)
	assert.True(t, utf8ValidString(out))
}

func utf8ValidString(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

func TestEventPropertyReaderFallsBackOnDecodeFailure(t *testing.T) {
	// A bulk decode that fails part-way still returns the properties preceding
	// the failure, and those must be used rather than discarded.
	e := makeEvent(guidGroupPolicy, evtCSEStart, gpTestBoot,
		property{Name: "CSEExtensionId", Value: cseRegistryGUID})
	e.propsErr = fmt.Errorf("failed to parse property [3] %q: schema mismatch", "ApplicableGPOList")

	read := eventPropertyReader(e)
	assert.Equal(t, cseRegistryGUID, read("CSEExtensionId"), "properties before the failure are still usable")
	assert.Empty(t, read("ApplicableGPOList"))

	assert.Empty(t, eventPropertyReader(nil)("anything"), "a nil event must not panic")
}

func TestActivityIDOfNilEventIsZero(t *testing.T) {
	assert.Equal(t, windows.GUID{}, activityIDOf(nil))
}

// --- Plumbing ---

func TestAcceptedIDsCoversGroupPolicySwitch(t *testing.T) {
	// acceptedIDs is a hard pre-parse gate handed to the ETW filter, and
	// processEvent does not re-check it. An ID handled by the parser switch but
	// missing here is silently dropped in production with nothing to catch it,
	// so this is the only guard against that.
	handled := []uint16{
		evtMachineGPStart, evtMachineGPEnd,
		evtUserGPStart, evtUserGPEnd,
		4002, 4003, 4004, 4005, 4006, 4007,
		evtCSEStart,
		evtCSEStopSuccess, evtCSEStopWarning, evtCSEStopError,
		evtGPOListApplicable,
	}

	accepted := buildProviders(&BootTimeline{}, newGPAccumulator())[guidGroupPolicy].acceptedIDs
	for _, id := range handled {
		assert.Contains(t, accepted, id, "event %d is handled by the parser but would never reach it", id)
	}
	assert.Len(t, accepted, len(handled), "acceptedIDs should not admit events the parser ignores")

	assert.NotContains(t, accepted, uint16(5313), "the filtered-out GPO list is deliberately not collected")
}

func TestGroupPolicyPayloadOmittedWhenNoEventsSeen(t *testing.T) {
	assert.Nil(t, newGPAccumulator().finalize())
	assert.Nil(t, newCollector().groupPolicy.finalize())
}

func TestCSEInvocationCapPerPass(t *testing.T) {
	f := newGPFixture(t)
	f.startComputerPass()
	f.startUserPass()

	const overflow = 5
	for i := 0; i < maxCSEInvocationsPerPass+overflow; i++ {
		guid := fmt.Sprintf("{%08X-683F-11D2-A89A-00C04FBBCFA2}", i)
		offset := time.Duration(13000+i) * time.Millisecond
		f.cseStart(gpTestActivity, offset, guid, "Extension", false, "")
		f.cseStop(gpTestActivity, offset+time.Millisecond, evtCSEStopSuccess, guid, "Extension", 1, "0x0")
	}
	// One invocation in the other pass, which the bound must not touch.
	f.cseStart(gpUserActivity, 31*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpUserActivity, 32*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", 1000, "0x0")

	p := f.payload()
	assert.Len(t, p.Passes.Computer.CSEInvocations, maxCSEInvocationsPerPass)
	assert.Equal(t, overflow, p.Passes.Computer.CSEInvocationsTruncated)
	assert.Len(t, p.Passes.User.CSEInvocations, 1, "the bound is per pass")
	assert.Zero(t, p.Passes.User.CSEInvocationsTruncated)
}

func TestInvocationsAreOrderedChronologically(t *testing.T) {
	// Emission order is also retention order, so it has to be deterministic.
	f := newGPFixture(t)
	f.startComputerPass()
	// Delivered out of order, and one with no start at all.
	f.cseStart(gpTestActivity, 16*time.Second, cseFolderRedGUID, "Folder Redirection", false, "")
	f.cseStop(gpTestActivity, 17*time.Second, evtCSEStopSuccess, cseFolderRedGUID, "Folder Redirection", 1000, "0x0")
	f.cseStart(gpTestActivity, 13*time.Second, cseRegistryGUID, "Registry", false, "")
	f.cseStop(gpTestActivity, 14*time.Second, evtCSEStopSuccess, cseRegistryGUID, "Registry", 1000, "0x0")
	f.cseStop(gpTestActivity, 18*time.Second, evtCSEStopSuccess, cseAuditGUID, "Audit", 50, "0x0")

	invs := f.payload().Passes.Computer.CSEInvocations
	require.Len(t, invs, 3)
	assert.Equal(t, cseRegistryGUID, invs[0].CSEGUID)
	assert.Equal(t, cseFolderRedGUID, invs[1].CSEGUID)
	assert.Equal(t, cseAuditGUID, invs[2].CSEGUID, "an untimed record sorts last")
}
