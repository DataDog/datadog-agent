// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && test

package logondurationimpl

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/winutil/etw"
)

// property holds a name-value pair for constructing mock events in tests.
type property struct {
	Name  string
	Value interface{}
}

// mockEvent implements eventWithProperties for testing.
// It deliberately omits directPropertyLookup, so parsers that probe for it exercise their fallback path.
type mockEvent struct {
	providerID windows.GUID
	eventID    uint16
	activityID windows.GUID
	timestamp  time.Time
	props      map[string]interface{}
	// propsErr simulates a partial TDH decode: the properties gathered so far, plus an error.
	propsErr error
}

func (m *mockEvent) GetPropertyString(name string) string {
	// A real Event returns "" on any error; serving values despite propsErr would leave the bulk-read path untested.
	if m.propsErr != nil {
		return ""
	}
	if v, ok := m.props[name]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func (m *mockEvent) GetProviderID() windows.GUID { return m.providerID }
func (m *mockEvent) GetEventID() uint16          { return m.eventID }
func (m *mockEvent) GetTimestamp() time.Time     { return m.timestamp }
func (m *mockEvent) GetActivityID() windows.GUID { return m.activityID }

func (m *mockEvent) EventProperties() (map[string]interface{}, error) {
	return m.props, m.propsErr
}

// makeEvent creates a synthetic event for testing.
func makeEvent(providerGUID windows.GUID, eventID uint16, ts time.Time, eventData ...property) *mockEvent {
	props := make(map[string]interface{})
	for _, p := range eventData {
		props[p.Name] = p.Value
	}
	return &mockEvent{
		providerID: providerGUID,
		eventID:    eventID,
		timestamp:  ts,
		props:      props,
	}
}

func newCollector() *collector {
	c := &collector{}
	c.groupPolicy = newGPAccumulator()
	c.providers = buildProviders(&c.timeline, c.groupPolicy)
	return c
}

func TestGetEventPropString(t *testing.T) {
	t.Run("finds property in EventData", func(t *testing.T) {
		e := makeEvent(guidKernelProcess, 1, time.Now(), property{Name: "ImageName", Value: "smss.exe"})
		assert.Equal(t, "smss.exe", getEventPropString(e, "ImageName"))
	})

	t.Run("finds property in UserData", func(t *testing.T) {
		e := &mockEvent{props: map[string]interface{}{"SubscriberName": "GPClient"}}
		assert.Equal(t, "GPClient", getEventPropString(e, "SubscriberName"))
	})

	t.Run("prefers EventData over UserData", func(t *testing.T) {
		e := &mockEvent{props: map[string]interface{}{"Name": "from_event_data"}}
		assert.Equal(t, "from_event_data", getEventPropString(e, "Name"))
	})

	t.Run("returns empty string when not found", func(t *testing.T) {
		e := &mockEvent{props: map[string]interface{}{}}
		assert.Equal(t, "", getEventPropString(e, "NonExistent"))
	})

	t.Run("converts non-string values", func(t *testing.T) {
		e := &mockEvent{props: map[string]interface{}{"PID": int64(1234)}}
		assert.Equal(t, "1234", getEventPropString(e, "PID"))
	})
}

func TestParseKernelGeneral(t *testing.T) {
	ts := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	t.Run("sets BootStart on first event", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &kernelGeneralParser{timeline: tl}
		p.Parse(nil, evtBootStart, ts)
		assert.Equal(t, ts, tl.BootStart)
	})

	t.Run("does not overwrite BootStart on subsequent events", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &kernelGeneralParser{timeline: tl}
		ts2 := ts.Add(5 * time.Second)

		p.Parse(nil, evtBootStart, ts)
		p.Parse(nil, evtBootStart, ts2)

		assert.Equal(t, ts, tl.BootStart)
	})
}

func TestParseKernelProcess(t *testing.T) {
	ts := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	makeProcessEvent := func(imageName string, timestamp time.Time) *mockEvent {
		return makeEvent(guidKernelProcess, evtProcessStart, timestamp,
			property{Name: "ImageName", Value: imageName},
		)
	}

	t.Run("explorer.exe sets ExplorerStart only once", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &kernelProcessParser{timeline: tl}
		ts2 := ts.Add(5 * time.Second)

		p.Parse(makeProcessEvent("explorer.exe", ts), evtProcessStart, ts)
		p.Parse(makeProcessEvent("explorer.exe", ts2), evtProcessStart, ts2)

		assert.Equal(t, ts, tl.ExplorerStart)
	})

	t.Run("handles mixed case image names", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &kernelProcessParser{timeline: tl}
		e := makeProcessEvent("EXPLORER.EXE", ts)

		p.Parse(e, evtProcessStart, ts)

		assert.Equal(t, ts, tl.ExplorerStart)
	})

	t.Run("handles full path image names", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &kernelProcessParser{timeline: tl}
		e := makeProcessEvent(`C:\Windows\explorer.exe`, ts)

		p.Parse(e, evtProcessStart, ts)

		assert.Equal(t, ts, tl.ExplorerStart)
	})

	t.Run("tries alternative property names", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &kernelProcessParser{timeline: tl}
		e := makeEvent(guidKernelProcess, evtProcessStart, ts,
			property{Name: "ImageName", Value: "explorer.exe"},
		)

		p.Parse(e, evtProcessStart, ts)

		assert.Equal(t, ts, tl.ExplorerStart)
	})

	t.Run("ignores unknown processes", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &kernelProcessParser{timeline: tl}
		e := makeProcessEvent("svchost.exe", ts)

		p.Parse(e, evtProcessStart, ts)

		assert.True(t, tl.ExplorerStart.IsZero())
	})
}

func TestParseWinlogon(t *testing.T) {
	ts := time.Date(2026, 1, 15, 8, 0, 30, 0, time.UTC)

	t.Run("event 103 sets LoginUIStart", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &winlogonParser{timeline: tl}
		p.Parse(nil, evtLoginUIStart, ts)
		assert.Equal(t, ts, tl.LoginUIStart)
	})

	t.Run("event 104 sets LoginUIDone", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &winlogonParser{timeline: tl}
		p.Parse(nil, evtLoginUIDone, ts)
		assert.Equal(t, ts, tl.LoginUIDone)
	})

	t.Run("event 9 sets ExecuteShellCommandListStart", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &winlogonParser{timeline: tl}
		p.Parse(nil, evtWinlogonShellCmdStart, ts)
		assert.Equal(t, ts, tl.ExecuteShellCommandListStart)
	})

	t.Run("event 10 sets ExecuteShellCommandListEnd", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &winlogonParser{timeline: tl}
		p.Parse(nil, evtWinlogonShellCmdEnd, ts)
		assert.Equal(t, ts, tl.ExecuteShellCommandListEnd)
	})

	t.Run("event 7001 sets SessionLogon", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &winlogonParser{timeline: tl}
		p.Parse(nil, evtSessionLogon, ts)
		assert.Equal(t, ts, tl.SessionLogon)
	})

	t.Run("event 7001 first wins", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &winlogonParser{timeline: tl}
		p.Parse(nil, evtSessionLogon, ts)
		ts2 := ts.Add(5 * time.Second)
		p.Parse(nil, evtSessionLogon, ts2)
		assert.Equal(t, ts, tl.SessionLogon)
	})
}

func TestParseUserProfile(t *testing.T) {
	ts := time.Date(2026, 1, 15, 8, 0, 45, 0, time.UTC)

	t.Run("event 1001 sets ProfileCreationStart", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &userProfileParser{timeline: tl}
		p.Parse(nil, evtProfileCreationStart, ts)
		assert.Equal(t, ts, tl.ProfileCreationStart)
	})

	t.Run("event 1001 first-write-wins", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &userProfileParser{timeline: tl}
		ts2 := ts.Add(5 * time.Second)
		p.Parse(nil, evtProfileCreationStart, ts)
		p.Parse(nil, evtProfileCreationStart, ts2)
		assert.Equal(t, ts, tl.ProfileCreationStart)
	})

	t.Run("event 1002 sets ProfileCreationEnd (first-write-wins)", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &userProfileParser{timeline: tl}
		ts2 := ts.Add(5 * time.Second)
		p.Parse(nil, evtProfileCreationEnd, ts)
		p.Parse(nil, evtProfileCreationEnd, ts2)
		assert.Equal(t, ts, tl.ProfileCreationEnd)
	})
}

func TestParseGroupPolicy(t *testing.T) {
	ts := time.Date(2026, 1, 15, 8, 0, 12, 0, time.UTC)

	t.Run("event 4000 sets MachineGPStart", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &groupPolicyParser{timeline: tl, gp: newGPAccumulator()}
		p.Parse(nil, evtMachineGPStart, ts)
		assert.Equal(t, ts, tl.MachineGPStart)
	})

	t.Run("event 8000 sets MachineGPEnd", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &groupPolicyParser{timeline: tl, gp: newGPAccumulator()}
		p.Parse(nil, evtMachineGPEnd, ts)
		assert.Equal(t, ts, tl.MachineGPEnd)
	})

	t.Run("event 4001 sets UserGPStart", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &groupPolicyParser{timeline: tl, gp: newGPAccumulator()}
		p.Parse(nil, evtUserGPStart, ts)
		assert.Equal(t, ts, tl.UserGPStart)
	})

	t.Run("event 8001 sets UserGPEnd (first-write-wins)", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &groupPolicyParser{timeline: tl, gp: newGPAccumulator()}
		ts2 := ts.Add(5 * time.Second)
		p.Parse(nil, evtUserGPEnd, ts)
		p.Parse(nil, evtUserGPEnd, ts2)
		assert.Equal(t, ts, tl.UserGPEnd)
	})

	t.Run("event 4000 first-write-wins for MachineGPStart", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &groupPolicyParser{timeline: tl, gp: newGPAccumulator()}
		ts2 := ts.Add(5 * time.Second)
		p.Parse(nil, evtMachineGPStart, ts)
		p.Parse(nil, evtMachineGPStart, ts2)
		assert.Equal(t, ts, tl.MachineGPStart)
	})
}

func TestParseShellCore(t *testing.T) {
	ts := time.Date(2026, 1, 15, 8, 1, 30, 0, time.UTC)

	makeShellCoreEvent := func(id uint16, timestamp time.Time, props ...property) *mockEvent {
		return makeEvent(guidShellCore, id, timestamp, props...)
	}

	t.Run("event 9601 sets ExplorerInitStart", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &shellCoreParser{timeline: tl}
		p.Parse(makeShellCoreEvent(evtExplorerInitStart, ts), evtExplorerInitStart, ts)
		assert.Equal(t, ts, tl.ExplorerInitStart)
	})

	t.Run("event 9601 first-write-wins", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &shellCoreParser{timeline: tl}
		ts2 := ts.Add(5 * time.Second)
		p.Parse(makeShellCoreEvent(evtExplorerInitStart, ts), evtExplorerInitStart, ts)
		p.Parse(makeShellCoreEvent(evtExplorerInitStart, ts2), evtExplorerInitStart, ts2)
		assert.Equal(t, ts, tl.ExplorerInitStart)
	})

	t.Run("event 9602 sets ExplorerInitEnd", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &shellCoreParser{timeline: tl}
		p.Parse(makeShellCoreEvent(evtExplorerInitEnd, ts), evtExplorerInitEnd, ts)
		assert.Equal(t, ts, tl.ExplorerInitEnd)
	})

	t.Run("event 9611 sets DesktopCreateStart", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &shellCoreParser{timeline: tl}
		p.Parse(makeShellCoreEvent(evtDesktopCreateStart, ts), evtDesktopCreateStart, ts)
		assert.Equal(t, ts, tl.DesktopCreateStart)
	})

	t.Run("event 9612 sets DesktopCreateEnd", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &shellCoreParser{timeline: tl}
		p.Parse(makeShellCoreEvent(evtDesktopCreateEnd, ts), evtDesktopCreateEnd, ts)
		assert.Equal(t, ts, tl.DesktopCreateEnd)
	})

	t.Run("event 9648 WaitForDesktopVisuals sets DesktopVisibleStart", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &shellCoreParser{timeline: tl}
		e := makeShellCoreEvent(evtExplorerStepStart, ts, property{Name: "psz", Value: "WaitForDesktopVisuals"})
		p.Parse(e, evtExplorerStepStart, ts)
		assert.Equal(t, ts, tl.DesktopVisibleStart)
	})

	t.Run("event 9649 WaitForDesktopVisuals sets DesktopVisibleEnd", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &shellCoreParser{timeline: tl}
		e := makeShellCoreEvent(evtExplorerStepEnd, ts, property{Name: "psz", Value: "WaitForDesktopVisuals"})
		p.Parse(e, evtExplorerStepEnd, ts)
		assert.Equal(t, ts, tl.DesktopVisibleEnd)
	})

	t.Run("event 9648 DesktopStartupApps sets DesktopStartupAppsStart", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &shellCoreParser{timeline: tl}
		e := makeShellCoreEvent(evtExplorerStepStart, ts, property{Name: "psz", Value: "DesktopStartupApps"})
		p.Parse(e, evtExplorerStepStart, ts)
		assert.Equal(t, ts, tl.DesktopStartupAppsStart)
	})

	t.Run("event 9649 DesktopStartupApps sets DesktopStartupAppsEnd", func(t *testing.T) {
		tl := &BootTimeline{}
		p := &shellCoreParser{timeline: tl}
		e := makeShellCoreEvent(evtExplorerStepEnd, ts, property{Name: "psz", Value: "DesktopStartupApps"})
		p.Parse(e, evtExplorerStepEnd, ts)
		assert.Equal(t, ts, tl.DesktopStartupAppsEnd)
	})
}

func TestProcessEvent(t *testing.T) {
	ts := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	t.Run("routes Kernel-General event 12 to kernelGeneralParser", func(t *testing.T) {
		coll := newCollector()
		e := makeEvent(guidKernelGeneral, 12, ts)

		processEvent(coll, e)

		assert.Equal(t, ts, coll.timeline.BootStart)
	})

	t.Run("routes Kernel-Process event 1 to kernelProcessParser", func(t *testing.T) {
		coll := newCollector()
		e := makeEvent(guidKernelProcess, 1, ts,
			property{Name: "ImageName", Value: "explorer.exe"},
		)

		processEvent(coll, e)

		assert.Equal(t, ts, coll.timeline.ExplorerStart)
	})

	t.Run("routes Winlogon event to winlogonParser", func(t *testing.T) {
		coll := newCollector()
		e := makeEvent(guidWinlogon, 7001, ts)

		processEvent(coll, e)

		assert.Equal(t, ts, coll.timeline.SessionLogon)
	})

	t.Run("routes UserProfile event to userProfileParser", func(t *testing.T) {
		coll := newCollector()
		e := makeEvent(guidUserProfile, 1001, ts)

		processEvent(coll, e)

		assert.Equal(t, ts, coll.timeline.ProfileCreationStart)
	})

	t.Run("routes GroupPolicy event to groupPolicyParser", func(t *testing.T) {
		coll := newCollector()
		e := makeEvent(guidGroupPolicy, 4000, ts)

		processEvent(coll, e)

		assert.Equal(t, ts, coll.timeline.MachineGPStart)
	})

	t.Run("routes Shell-Core event to shellCoreParser", func(t *testing.T) {
		coll := newCollector()
		e := makeEvent(guidShellCore, 9601, ts)

		processEvent(coll, e)

		assert.Equal(t, ts, coll.timeline.ExplorerInitStart)
	})

	t.Run("ignores event with unknown provider GUID", func(t *testing.T) {
		coll := newCollector()
		unknownGUID := etw.MustParseGUID("{00000000-0000-0000-0000-000000000001}")
		e := makeEvent(unknownGUID, 1, ts)

		processEvent(coll, e)

		assert.True(t, coll.timeline.BootStart.IsZero())
	})
}

// withActivity stamps an ETW activity ID, without which the collector attributes no invocation.
func withActivity(e *mockEvent, activity windows.GUID) *mockEvent {
	e.activityID = activity
	return e
}

func TestCollector_FullBootSequence(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	coll := newCollector()
	machinePass := windows.GUID{Data1: 0x5BB89DD7}

	events := []*mockEvent{
		makeEvent(guidKernelGeneral, 12, boot),
		makeEvent(guidWinlogon, 103, boot.Add(8*time.Second)),
		makeEvent(guidWinlogon, 104, boot.Add(10*time.Second)),
		withActivity(makeEvent(guidGroupPolicy, 4000, boot.Add(12*time.Second)), machinePass),
		withActivity(makeEvent(guidGroupPolicy, 4016, boot.Add(14*time.Second),
			property{Name: "CSEExtensionId", Value: "{35378EAC-683F-11D2-A89A-00C04FBBCFA2}"},
			property{Name: "CSEExtensionName", Value: "Registry"},
			property{Name: "IsExtensionAsyncProcessing", Value: "false"},
			property{Name: "ApplicableGPOList", Value: `<GPO ID="{31B2F340-016D-11D2-945F-00C04FB984F9}"><Name>Default Domain Policy</Name></GPO>`}), machinePass),
		withActivity(makeEvent(guidGroupPolicy, 5016, boot.Add(17*time.Second),
			property{Name: "CSEExtensionId", Value: "{35378EAC-683F-11D2-A89A-00C04FBBCFA2}"},
			property{Name: "CSEExtensionName", Value: "Registry"},
			property{Name: "CSEElaspedTimeInMilliSeconds", Value: "2980"},
			property{Name: "ErrorCode", Value: "0x0"}), machinePass),
		withActivity(makeEvent(guidGroupPolicy, 8000, boot.Add(20*time.Second)), machinePass),
		makeEvent(guidWinlogon, 7001, boot.Add(29*time.Second)),
		makeEvent(guidUserProfile, 1001, boot.Add(31*time.Second)),
		makeEvent(guidUserProfile, 1002, boot.Add(35*time.Second)),
		makeEvent(guidWinlogon, 9, boot.Add(40*time.Second)),
		makeEvent(guidWinlogon, 10, boot.Add(45*time.Second)),
		makeEvent(guidKernelProcess, 1, boot.Add(50*time.Second),
			property{Name: "ImageName", Value: "explorer.exe"}),
		makeEvent(guidShellCore, 9601, boot.Add(51*time.Second)),
		makeEvent(guidShellCore, 9602, boot.Add(53*time.Second)),
		makeEvent(guidShellCore, 9648, boot.Add(55*time.Second),
			property{Name: "psz", Value: "WaitForDesktopVisuals"}),
		makeEvent(guidShellCore, 9649, boot.Add(60*time.Second),
			property{Name: "psz", Value: "WaitForDesktopVisuals"}),
		makeEvent(guidShellCore, 9648, boot.Add(61*time.Second),
			property{Name: "psz", Value: "DesktopStartupApps"}),
		makeEvent(guidShellCore, 9649, boot.Add(65*time.Second),
			property{Name: "psz", Value: "DesktopStartupApps"}),
	}

	for _, e := range events {
		processEvent(coll, e)
	}

	tl := coll.timeline
	assert.Equal(t, boot, tl.BootStart)
	assert.Equal(t, boot.Add(8*time.Second), tl.LoginUIStart)
	assert.Equal(t, boot.Add(10*time.Second), tl.LoginUIDone)
	assert.Equal(t, boot.Add(12*time.Second), tl.MachineGPStart)
	assert.Equal(t, boot.Add(20*time.Second), tl.MachineGPEnd)
	assert.Equal(t, boot.Add(29*time.Second), tl.SessionLogon)
	assert.Equal(t, boot.Add(31*time.Second), tl.ProfileCreationStart)
	assert.Equal(t, boot.Add(35*time.Second), tl.ProfileCreationEnd)
	assert.Equal(t, boot.Add(40*time.Second), tl.ExecuteShellCommandListStart)
	assert.Equal(t, boot.Add(45*time.Second), tl.ExecuteShellCommandListEnd)
	assert.Equal(t, boot.Add(50*time.Second), tl.ExplorerStart)
	assert.Equal(t, boot.Add(51*time.Second), tl.ExplorerInitStart)
	assert.Equal(t, boot.Add(53*time.Second), tl.ExplorerInitEnd)
	assert.Equal(t, boot.Add(55*time.Second), tl.DesktopVisibleStart)
	assert.Equal(t, boot.Add(60*time.Second), tl.DesktopVisibleEnd)
	assert.Equal(t, boot.Add(61*time.Second), tl.DesktopStartupAppsStart)
	assert.Equal(t, boot.Add(65*time.Second), tl.DesktopStartupAppsEnd)

	custom := buildCustomPayload(tl, coll.groupPolicy.finalize(tl))
	durations := custom["durations"].(map[string]interface{})
	assert.Equal(t, int64(34000), durations["total_boot_duration_ms"])
	assert.Equal(t, int64(8000), durations["boot_duration_ms"])
	assert.Equal(t, int64(26000), durations["logon_duration_ms"])

	// This event set produces 8 of the 11 candidates; the extension invocations must not add a 9th.
	milestones := custom["boot_timeline"].([]Milestone)
	require.Len(t, milestones, 8)
	for _, m := range milestones {
		assert.NotContains(t, m.ID, "cse", "extension detail must not leak into boot_timeline")
	}

	gp := custom["group_policy_details"].(*GroupPolicyDetails)
	assert.Empty(t, gp.User, "no user pass in this trace")

	require.Len(t, gp.Computer, 1)
	inv := gp.Computer[0]
	assert.Equal(t, "Registry", inv.CSEName)
	assert.Equal(t, cseResultSuccess, inv.Result)
	assert.Equal(t, int64(3000), inv.DurationMs,
		"the measured 4016 -> 5016 interval, not the provider's 2980")

	var parent Milestone
	for _, m := range milestones {
		if m.ID == "computer_group_policy" {
			parent = m
		}
	}
	require.Equal(t, "computer_group_policy", parent.ID)
	// Raw 14000, less the 2000ms of login screen elided between LoginUIDone and the pass.
	assert.Equal(t, int64(12000), inv.OffsetMs)
	assert.GreaterOrEqual(t, float64(inv.OffsetMs), parent.OffsetMs)
	assert.LessOrEqual(t, float64(inv.OffsetMs+inv.DurationMs), parent.OffsetMs+parent.DurationMs)

	require.Len(t, inv.GPOs, 1)
	assert.Equal(t, "{31B2F340-016D-11D2-945F-00C04FB984F9}", inv.GPOs[0].ID)
	assert.Equal(t, "Default Domain Policy", inv.GPOs[0].Name, "name comes from the 4016 ApplicableGPOList")
}
