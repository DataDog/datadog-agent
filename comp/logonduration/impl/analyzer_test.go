// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package logondurationimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/tekert/goetw/etw"
)

// makeEvent creates a synthetic etw.Event for testing.
func makeEvent(providerGUID etw.GUID, eventID uint16, ts time.Time, eventData ...etw.EventProperty) *etw.Event {
	e := &etw.Event{
		EventData: eventData,
	}
	e.System.Provider.Guid = providerGUID
	e.System.EventID = eventID
	e.System.TimeCreated.SystemTime = ts
	return e
}

func newCollector() *collector {
	c := &collector{}
	c.initParseFunctions()
	return c
}

func TestGetEventPropString(t *testing.T) {
	t.Run("finds property in EventData", func(t *testing.T) {
		e := &etw.Event{
			EventData: []etw.EventProperty{
				{Name: "ImageFileName", Value: "smss.exe"},
			},
		}
		assert.Equal(t, "smss.exe", getEventPropString(e, "ImageFileName"))
	})

	t.Run("finds property in UserData", func(t *testing.T) {
		e := &etw.Event{
			UserData: []etw.EventProperty{
				{Name: "SubscriberName", Value: "GPClient"},
			},
		}
		assert.Equal(t, "GPClient", getEventPropString(e, "SubscriberName"))
	})

	t.Run("prefers EventData over UserData", func(t *testing.T) {
		e := &etw.Event{
			EventData: []etw.EventProperty{
				{Name: "Name", Value: "from_event_data"},
			},
			UserData: []etw.EventProperty{
				{Name: "Name", Value: "from_user_data"},
			},
		}
		assert.Equal(t, "from_event_data", getEventPropString(e, "Name"))
	})

	t.Run("returns empty string when not found", func(t *testing.T) {
		e := &etw.Event{}
		assert.Equal(t, "", getEventPropString(e, "NonExistent"))
	})

	t.Run("converts non-string values", func(t *testing.T) {
		e := &etw.Event{
			EventData: []etw.EventProperty{
				{Name: "PID", Value: int64(1234)},
			},
		}
		assert.Equal(t, "1234", getEventPropString(e, "PID"))
	})
}

func TestParseKernelGeneral(t *testing.T) {
	ts := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	t.Run("sets BootStart on first event", func(t *testing.T) {
		coll := newCollector()
		coll.parseKernelGeneral(nil, 12, ts)
		assert.Equal(t, ts, coll.timeline.BootStart)
	})

	t.Run("does not overwrite BootStart on subsequent events", func(t *testing.T) {
		coll := newCollector()
		ts2 := ts.Add(5 * time.Second)

		coll.parseKernelGeneral(nil, 12, ts)
		coll.parseKernelGeneral(nil, 12, ts2)

		assert.Equal(t, ts, coll.timeline.BootStart)
	})
}

func TestParseKernelProcess(t *testing.T) {
	ts := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	makeProcessEvent := func(imageName string, timestamp time.Time) *etw.Event {
		return makeEvent(*guidKernelProcess, 1, timestamp,
			etw.EventProperty{Name: "ImageFileName", Value: imageName},
		)
	}

	t.Run("first smss.exe sets SmssStart", func(t *testing.T) {
		coll := newCollector()
		e := makeProcessEvent("smss.exe", ts)

		coll.parseKernelProcess(e, 1, ts)

		assert.Equal(t, ts, coll.timeline.SmssStart)
		assert.Equal(t, 1, coll.smssCount)
	})

	t.Run("third smss.exe sets UserSmssStart", func(t *testing.T) {
		coll := newCollector()
		ts2 := ts.Add(2 * time.Second)
		ts3 := ts.Add(5 * time.Second)

		coll.parseKernelProcess(makeProcessEvent("smss.exe", ts), 1, ts)
		coll.parseKernelProcess(makeProcessEvent("smss.exe", ts2), 1, ts2)
		coll.parseKernelProcess(makeProcessEvent("smss.exe", ts3), 1, ts3)

		assert.Equal(t, ts, coll.timeline.SmssStart)
		assert.Equal(t, ts3, coll.timeline.UserSmssStart)
		assert.Equal(t, 3, coll.smssCount)
	})

	t.Run("first winlogon.exe sets WinlogonStart", func(t *testing.T) {
		coll := newCollector()
		e := makeProcessEvent("winlogon.exe", ts)

		coll.parseKernelProcess(e, 1, ts)

		assert.Equal(t, ts, coll.timeline.WinlogonStart)
		assert.Equal(t, 1, coll.winlogonCount)
	})

	t.Run("second winlogon.exe sets UserWinlogonStart", func(t *testing.T) {
		coll := newCollector()
		ts2 := ts.Add(10 * time.Second)

		coll.parseKernelProcess(makeProcessEvent("winlogon.exe", ts), 1, ts)
		coll.parseKernelProcess(makeProcessEvent("winlogon.exe", ts2), 1, ts2)

		assert.Equal(t, ts, coll.timeline.WinlogonStart)
		assert.Equal(t, ts2, coll.timeline.UserWinlogonStart)
	})

	t.Run("userinit.exe sets UserinitStart only once", func(t *testing.T) {
		coll := newCollector()
		ts2 := ts.Add(5 * time.Second)

		coll.parseKernelProcess(makeProcessEvent("userinit.exe", ts), 1, ts)
		coll.parseKernelProcess(makeProcessEvent("userinit.exe", ts2), 1, ts2)

		assert.Equal(t, ts, coll.timeline.UserinitStart)
	})

	t.Run("explorer.exe sets ExplorerStart only once", func(t *testing.T) {
		coll := newCollector()
		ts2 := ts.Add(5 * time.Second)

		coll.parseKernelProcess(makeProcessEvent("explorer.exe", ts), 1, ts)
		coll.parseKernelProcess(makeProcessEvent("explorer.exe", ts2), 1, ts2)

		assert.Equal(t, ts, coll.timeline.ExplorerStart)
	})

	t.Run("handles mixed case image names", func(t *testing.T) {
		coll := newCollector()
		e := makeProcessEvent("SMSS.EXE", ts)

		coll.parseKernelProcess(e, 1, ts)

		assert.Equal(t, ts, coll.timeline.SmssStart)
	})

	t.Run("handles full path image names", func(t *testing.T) {
		coll := newCollector()
		e := makeProcessEvent(`C:\Windows\System32\smss.exe`, ts)

		coll.parseKernelProcess(e, 1, ts)

		assert.Equal(t, ts, coll.timeline.SmssStart)
	})

	t.Run("tries alternative property names", func(t *testing.T) {
		coll := newCollector()
		e := makeEvent(*guidKernelProcess, 1, ts,
			etw.EventProperty{Name: "ImageName", Value: "explorer.exe"},
		)

		coll.parseKernelProcess(e, 1, ts)

		assert.Equal(t, ts, coll.timeline.ExplorerStart)
	})

	t.Run("ignores unknown processes", func(t *testing.T) {
		coll := newCollector()
		e := makeProcessEvent("svchost.exe", ts)

		coll.parseKernelProcess(e, 1, ts)

		assert.True(t, coll.timeline.SmssStart.IsZero())
		assert.True(t, coll.timeline.WinlogonStart.IsZero())
		assert.True(t, coll.timeline.UserinitStart.IsZero())
		assert.True(t, coll.timeline.ExplorerStart.IsZero())
	})
}

func TestParseWinlogon(t *testing.T) {
	ts := time.Date(2026, 1, 15, 8, 0, 30, 0, time.UTC)

	t.Run("event 101 sets WinlogonInit", func(t *testing.T) {
		coll := newCollector()
		coll.parseWinlogon(nil, 101, ts)
		assert.Equal(t, ts, coll.timeline.WinlogonInit)
	})

	t.Run("event 101 first-write-wins", func(t *testing.T) {
		coll := newCollector()
		ts2 := ts.Add(5 * time.Second)
		coll.parseWinlogon(nil, 101, ts)
		coll.parseWinlogon(nil, 101, ts2)
		assert.Equal(t, ts, coll.timeline.WinlogonInit)
	})

	t.Run("event 102 sets WinlogonInitDone", func(t *testing.T) {
		coll := newCollector()
		coll.parseWinlogon(nil, 102, ts)
		assert.Equal(t, ts, coll.timeline.WinlogonInitDone)
	})

	t.Run("event 103 sets LoginUIStart", func(t *testing.T) {
		coll := newCollector()
		coll.parseWinlogon(nil, 103, ts)
		assert.Equal(t, ts, coll.timeline.LoginUIStart)
	})

	t.Run("event 104 sets LoginUIDone", func(t *testing.T) {
		coll := newCollector()
		coll.parseWinlogon(nil, 104, ts)
		assert.Equal(t, ts, coll.timeline.LoginUIDone)
	})

	t.Run("event 9 sets ExecuteShellCommandListStart", func(t *testing.T) {
		coll := newCollector()
		coll.parseWinlogon(nil, 9, ts)
		assert.Equal(t, ts, coll.timeline.ExecuteShellCommandListStart)
	})

	t.Run("event 10 sets ExecuteShellCommandListEnd", func(t *testing.T) {
		coll := newCollector()
		coll.parseWinlogon(nil, 10, ts)
		assert.Equal(t, ts, coll.timeline.ExecuteShellCommandListEnd)
	})

	t.Run("event 5001 sets LogonStart", func(t *testing.T) {
		coll := newCollector()
		coll.parseWinlogon(nil, 5001, ts)
		assert.Equal(t, ts, coll.timeline.LogonStart)
	})

}

func TestParseUserProfile(t *testing.T) {
	ts := time.Date(2026, 1, 15, 8, 0, 45, 0, time.UTC)

	t.Run("event 1001 sets ProfileCreationStart", func(t *testing.T) {
		coll := newCollector()
		coll.parseUserProfile(nil, 1001, ts)
		assert.Equal(t, ts, coll.timeline.ProfileCreationStart)
	})

	t.Run("event 1001 first-write-wins", func(t *testing.T) {
		coll := newCollector()
		ts2 := ts.Add(5 * time.Second)
		coll.parseUserProfile(nil, 1001, ts)
		coll.parseUserProfile(nil, 1001, ts2)
		assert.Equal(t, ts, coll.timeline.ProfileCreationStart)
	})

	t.Run("event 1002 sets ProfileCreationEnd (first-write-wins)", func(t *testing.T) {
		coll := newCollector()
		ts2 := ts.Add(5 * time.Second)
		coll.parseUserProfile(nil, 1002, ts)
		coll.parseUserProfile(nil, 1002, ts2)
		assert.Equal(t, ts, coll.timeline.ProfileCreationEnd)
	})
}

func TestParseGroupPolicy(t *testing.T) {
	ts := time.Date(2026, 1, 15, 8, 0, 12, 0, time.UTC)

	t.Run("event 4000 sets MachineGPStart", func(t *testing.T) {
		coll := newCollector()
		coll.parseGroupPolicy(makeEvent(*guidGroupPolicy, 4000, ts), 4000, ts)
		assert.Equal(t, ts, coll.timeline.MachineGPStart)
	})

	t.Run("event 8000 sets MachineGPEnd", func(t *testing.T) {
		coll := newCollector()
		coll.parseGroupPolicy(makeEvent(*guidGroupPolicy, 8000, ts), 8000, ts)
		assert.Equal(t, ts, coll.timeline.MachineGPEnd)
	})

	t.Run("event 4001 sets UserGPStart", func(t *testing.T) {
		coll := newCollector()
		coll.parseGroupPolicy(makeEvent(*guidGroupPolicy, 4001, ts), 4001, ts)
		assert.Equal(t, ts, coll.timeline.UserGPStart)
	})

	t.Run("event 8001 sets UserGPEnd (first-write-wins)", func(t *testing.T) {
		coll := newCollector()
		ts2 := ts.Add(5 * time.Second)
		coll.parseGroupPolicy(makeEvent(*guidGroupPolicy, 8001, ts), 8001, ts)
		coll.parseGroupPolicy(makeEvent(*guidGroupPolicy, 8001, ts2), 8001, ts2)
		assert.Equal(t, ts, coll.timeline.UserGPEnd)
	})

	t.Run("event 4000 first-write-wins for MachineGPStart", func(t *testing.T) {
		coll := newCollector()
		ts2 := ts.Add(5 * time.Second)
		coll.parseGroupPolicy(makeEvent(*guidGroupPolicy, 4000, ts), 4000, ts)
		coll.parseGroupPolicy(makeEvent(*guidGroupPolicy, 4000, ts2), 4000, ts2)
		assert.Equal(t, ts, coll.timeline.MachineGPStart)
	})
}

func TestParseShellCore(t *testing.T) {
	ts := time.Date(2026, 1, 15, 8, 1, 30, 0, time.UTC)

	makeShellCoreEvent := func(id uint16, timestamp time.Time, props ...etw.EventProperty) *etw.Event {
		return makeEvent(*guidShellCore, id, timestamp, props...)
	}

	t.Run("event 9601 sets ExplorerInitStart", func(t *testing.T) {
		coll := newCollector()
		coll.parseShellCore(makeShellCoreEvent(9601, ts), 9601, ts)
		assert.Equal(t, ts, coll.timeline.ExplorerInitStart)
	})

	t.Run("event 9601 first-write-wins", func(t *testing.T) {
		coll := newCollector()
		ts2 := ts.Add(5 * time.Second)
		coll.parseShellCore(makeShellCoreEvent(9601, ts), 9601, ts)
		coll.parseShellCore(makeShellCoreEvent(9601, ts2), 9601, ts2)
		assert.Equal(t, ts, coll.timeline.ExplorerInitStart)
	})

	t.Run("event 9602 sets ExplorerInitEnd", func(t *testing.T) {
		coll := newCollector()
		coll.parseShellCore(makeShellCoreEvent(9602, ts), 9602, ts)
		assert.Equal(t, ts, coll.timeline.ExplorerInitEnd)
	})

	t.Run("event 9611 sets DesktopCreateStart", func(t *testing.T) {
		coll := newCollector()
		coll.parseShellCore(makeShellCoreEvent(9611, ts), 9611, ts)
		assert.Equal(t, ts, coll.timeline.DesktopCreateStart)
	})

	t.Run("event 9612 sets DesktopCreateEnd", func(t *testing.T) {
		coll := newCollector()
		coll.parseShellCore(makeShellCoreEvent(9612, ts), 9612, ts)
		assert.Equal(t, ts, coll.timeline.DesktopCreateEnd)
	})

	t.Run("event 9648 WaitForDesktopVisuals sets DesktopVisibleStart", func(t *testing.T) {
		coll := newCollector()
		e := makeShellCoreEvent(9648, ts, etw.EventProperty{Name: "psz", Value: "WaitForDesktopVisuals"})
		coll.parseShellCore(e, 9648, ts)
		assert.Equal(t, ts, coll.timeline.DesktopVisibleStart)
	})

	t.Run("event 9649 WaitForDesktopVisuals sets DesktopVisibleEnd", func(t *testing.T) {
		coll := newCollector()
		e := makeShellCoreEvent(9649, ts, etw.EventProperty{Name: "psz", Value: "WaitForDesktopVisuals"})
		coll.parseShellCore(e, 9649, ts)
		assert.Equal(t, ts, coll.timeline.DesktopVisibleEnd)
	})

	t.Run("event 9648 Finalize sets DesktopReadyStart", func(t *testing.T) {
		coll := newCollector()
		e := makeShellCoreEvent(9648, ts, etw.EventProperty{Name: "psz", Value: "Finalize"})
		coll.parseShellCore(e, 9648, ts)
		assert.Equal(t, ts, coll.timeline.DesktopReadyStart)
	})

	t.Run("event 9649 Finalize sets DesktopReadyEnd", func(t *testing.T) {
		coll := newCollector()
		e := makeShellCoreEvent(9649, ts, etw.EventProperty{Name: "psz", Value: "Finalize"})
		coll.parseShellCore(e, 9649, ts)
		assert.Equal(t, ts, coll.timeline.DesktopReadyEnd)
	})

	t.Run("event 9648 DesktopStartupApps sets DesktopStartupAppsStart", func(t *testing.T) {
		coll := newCollector()
		e := makeShellCoreEvent(9648, ts, etw.EventProperty{Name: "psz", Value: "DesktopStartupApps"})
		coll.parseShellCore(e, 9648, ts)
		assert.Equal(t, ts, coll.timeline.DesktopStartupAppsStart)
	})

	t.Run("event 9649 DesktopStartupApps sets DesktopStartupAppsEnd", func(t *testing.T) {
		coll := newCollector()
		e := makeShellCoreEvent(9649, ts, etw.EventProperty{Name: "psz", Value: "DesktopStartupApps"})
		coll.parseShellCore(e, 9649, ts)
		assert.Equal(t, ts, coll.timeline.DesktopStartupAppsEnd)
	})
}

func TestProcessEvent(t *testing.T) {
	ts := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	t.Run("routes Kernel-General event 12 to parseKernelGeneral", func(t *testing.T) {
		coll := newCollector()
		e := makeEvent(*guidKernelGeneral, 12, ts)

		processEvent(coll, e)

		assert.Equal(t, ts, coll.timeline.BootStart)
	})

	t.Run("routes Kernel-Process event 1 to parseKernelProcess", func(t *testing.T) {
		coll := newCollector()
		e := makeEvent(*guidKernelProcess, 1, ts,
			etw.EventProperty{Name: "ImageFileName", Value: "explorer.exe"},
		)

		processEvent(coll, e)

		assert.Equal(t, ts, coll.timeline.ExplorerStart)
	})

	t.Run("routes Winlogon event to parseWinlogon", func(t *testing.T) {
		coll := newCollector()
		e := makeEvent(*guidWinlogon, 5001, ts)

		processEvent(coll, e)

		assert.Equal(t, ts, coll.timeline.LogonStart)
	})

	t.Run("routes UserProfile event to parseUserProfile", func(t *testing.T) {
		coll := newCollector()
		e := makeEvent(*guidUserProfile, 1001, ts)

		processEvent(coll, e)

		assert.Equal(t, ts, coll.timeline.ProfileCreationStart)
	})

	t.Run("routes GroupPolicy event to parseGroupPolicy", func(t *testing.T) {
		coll := newCollector()
		e := makeEvent(*guidGroupPolicy, 4000, ts)

		processEvent(coll, e)

		assert.Equal(t, ts, coll.timeline.MachineGPStart)
	})

	t.Run("routes Shell-Core event to parseShellCore", func(t *testing.T) {
		coll := newCollector()
		e := makeEvent(*guidShellCore, 9601, ts)

		processEvent(coll, e)

		assert.Equal(t, ts, coll.timeline.ExplorerInitStart)
	})

	t.Run("ignores event with unknown provider GUID", func(t *testing.T) {
		coll := newCollector()
		unknownGUID := *etw.MustParseGUID("{00000000-0000-0000-0000-000000000001}")
		e := makeEvent(unknownGUID, 1, ts)

		processEvent(coll, e)

		assert.True(t, coll.timeline.BootStart.IsZero())
	})
}

func TestCollector_FullBootSequence(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	coll := newCollector()

	events := []*etw.Event{
		makeEvent(*guidKernelGeneral, 12, boot),
		makeEvent(*guidKernelProcess, 1, boot.Add(1*time.Second),
			etw.EventProperty{Name: "ImageFileName", Value: "smss.exe"}),
		makeEvent(*guidWinlogon, 101, boot.Add(4*time.Second)),
		makeEvent(*guidWinlogon, 103, boot.Add(8*time.Second)),
		makeEvent(*guidWinlogon, 104, boot.Add(10*time.Second)),
		makeEvent(*guidGroupPolicy, 4000, boot.Add(12*time.Second)),
		makeEvent(*guidGroupPolicy, 8000, boot.Add(20*time.Second)),
		makeEvent(*guidKernelProcess, 1, boot.Add(25*time.Second),
			etw.EventProperty{Name: "ImageFileName", Value: "winlogon.exe"}),
		makeEvent(*guidWinlogon, 5001, boot.Add(30*time.Second)),
		makeEvent(*guidUserProfile, 1001, boot.Add(31*time.Second)),
		makeEvent(*guidUserProfile, 1002, boot.Add(35*time.Second)),
		makeEvent(*guidWinlogon, 9, boot.Add(40*time.Second)),
		makeEvent(*guidKernelProcess, 1, boot.Add(42*time.Second),
			etw.EventProperty{Name: "ImageFileName", Value: "userinit.exe"}),
		makeEvent(*guidWinlogon, 10, boot.Add(45*time.Second)),
		makeEvent(*guidKernelProcess, 1, boot.Add(50*time.Second),
			etw.EventProperty{Name: "ImageFileName", Value: "explorer.exe"}),
		makeEvent(*guidShellCore, 9601, boot.Add(51*time.Second)),
		makeEvent(*guidShellCore, 9602, boot.Add(53*time.Second)),
		makeEvent(*guidShellCore, 9648, boot.Add(55*time.Second),
			etw.EventProperty{Name: "psz", Value: "WaitForDesktopVisuals"}),
		makeEvent(*guidShellCore, 9649, boot.Add(60*time.Second),
			etw.EventProperty{Name: "psz", Value: "WaitForDesktopVisuals"}),
		makeEvent(*guidWinlogon, 5002, boot.Add(60*time.Second)),
		makeEvent(*guidShellCore, 9648, boot.Add(61*time.Second),
			etw.EventProperty{Name: "psz", Value: "Finalize"}),
		makeEvent(*guidShellCore, 9649, boot.Add(65*time.Second),
			etw.EventProperty{Name: "psz", Value: "Finalize"}),
	}

	for _, e := range events {
		processEvent(coll, e)
	}

	tl := coll.timeline
	assert.Equal(t, boot, tl.BootStart)
	assert.Equal(t, boot.Add(1*time.Second), tl.SmssStart)
	assert.Equal(t, boot.Add(4*time.Second), tl.WinlogonInit)
	assert.Equal(t, boot.Add(8*time.Second), tl.LoginUIStart)
	assert.Equal(t, boot.Add(10*time.Second), tl.LoginUIDone)
	assert.Equal(t, boot.Add(12*time.Second), tl.MachineGPStart)
	assert.Equal(t, boot.Add(20*time.Second), tl.MachineGPEnd)
	assert.Equal(t, boot.Add(25*time.Second), tl.WinlogonStart)
	assert.Equal(t, boot.Add(30*time.Second), tl.LogonStart)
	assert.Equal(t, boot.Add(31*time.Second), tl.ProfileCreationStart)
	assert.Equal(t, boot.Add(35*time.Second), tl.ProfileCreationEnd)
	assert.Equal(t, boot.Add(40*time.Second), tl.ExecuteShellCommandListStart)
	assert.Equal(t, boot.Add(42*time.Second), tl.UserinitStart)
	assert.Equal(t, boot.Add(45*time.Second), tl.ExecuteShellCommandListEnd)
	assert.Equal(t, boot.Add(50*time.Second), tl.ExplorerStart)
	assert.Equal(t, boot.Add(51*time.Second), tl.ExplorerInitStart)
	assert.Equal(t, boot.Add(53*time.Second), tl.ExplorerInitEnd)
	assert.Equal(t, boot.Add(55*time.Second), tl.DesktopVisibleStart)
	assert.Equal(t, boot.Add(60*time.Second), tl.DesktopVisibleEnd)
	assert.Equal(t, boot.Add(60*time.Second), tl.LogonStop)
	assert.Equal(t, boot.Add(61*time.Second), tl.DesktopReadyStart)
	assert.Equal(t, boot.Add(65*time.Second), tl.DesktopReadyEnd)

	custom := buildCustomPayload(tl)
	durations := custom["durations"].(map[string]interface{})
	assert.Equal(t, int64(65000), durations["Total Boot Duration (ms)"])
	assert.Equal(t, int64(8000), durations["Boot Duration (ms)"])
	assert.Equal(t, int64(25000), durations["Logon Duration (ms)"])
}
