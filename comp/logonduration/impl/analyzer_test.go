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
	"github.com/stretchr/testify/require"
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
	c := &collector{
		currentSubs: make(map[string]time.Time),
	}
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

func TestFindSubscriberName(t *testing.T) {
	t.Run("finds named SubscriberName property", func(t *testing.T) {
		e := &etw.Event{
			EventData: []etw.EventProperty{
				{Name: "SubscriberName", Value: "GPClient"},
			},
		}
		assert.Equal(t, "GPClient", findSubscriberName(e))
	})

	t.Run("finds Name property as fallback", func(t *testing.T) {
		e := &etw.Event{
			EventData: []etw.EventProperty{
				{Name: "Name", Value: "Profiles"},
			},
		}
		assert.Equal(t, "Profiles", findSubscriberName(e))
	})

	t.Run("falls back to known subscriber name in values", func(t *testing.T) {
		e := &etw.Event{
			EventData: []etw.EventProperty{
				{Name: "SomeOtherProp", Value: "something with SessionEnv in it"},
			},
		}
		assert.Equal(t, "SessionEnv", findSubscriberName(e))
	})

	t.Run("returns empty when no subscriber info found", func(t *testing.T) {
		e := &etw.Event{
			EventData: []etw.EventProperty{
				{Name: "UnrelatedProp", Value: "unrelated_value"},
			},
		}
		assert.Equal(t, "", findSubscriberName(e))
	})

	t.Run("returns empty for event with no properties", func(t *testing.T) {
		e := &etw.Event{}
		assert.Equal(t, "", findSubscriberName(e))
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

	t.Run("event 107 sets ServicesWaitStart", func(t *testing.T) {
		coll := newCollector()
		coll.parseWinlogon(nil, 107, ts)
		assert.Equal(t, ts, coll.timeline.LSMStart)
	})

	t.Run("event 108 sets ServicesReady", func(t *testing.T) {
		coll := newCollector()
		coll.parseWinlogon(nil, 108, ts)
		assert.Equal(t, ts, coll.timeline.LSMReady)
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

	t.Run("event 11 sets ThemesLogonStart (last-write-wins)", func(t *testing.T) {
		coll := newCollector()
		ts2 := ts.Add(5 * time.Second)
		coll.parseWinlogon(nil, 11, ts)
		coll.parseWinlogon(nil, 11, ts2)
		assert.Equal(t, ts2, coll.timeline.ThemesLogonStart)
	})

	t.Run("event 13 sets ThemesLogonEnd (last-write-wins)", func(t *testing.T) {
		coll := newCollector()
		ts2 := ts.Add(5 * time.Second)
		coll.parseWinlogon(nil, 13, ts)
		coll.parseWinlogon(nil, 13, ts2)
		assert.Equal(t, ts2, coll.timeline.ThemesLogonEnd)
	})

	t.Run("event 5001 sets LogonStart", func(t *testing.T) {
		coll := newCollector()
		coll.parseWinlogon(nil, 5001, ts)
		assert.Equal(t, ts, coll.timeline.LogonStart)
	})

	t.Run("event 802 sets SubscribersDone", func(t *testing.T) {
		coll := newCollector()
		coll.parseWinlogon(nil, 802, ts)
		assert.Equal(t, ts, coll.timeline.SubscribersDone)
	})
}

func TestParseWinlogon_SubscriberTracking(t *testing.T) {
	ts := time.Date(2026, 1, 15, 8, 0, 30, 0, time.UTC)

	t.Run("805/806 pair produces SubscriberInfo", func(t *testing.T) {
		coll := newCollector()
		startEvent := makeEvent(*guidWinlogon, 805, ts,
			etw.EventProperty{Name: "SubscriberName", Value: "GPClient"},
		)
		endEvent := makeEvent(*guidWinlogon, 806, ts.Add(2*time.Second),
			etw.EventProperty{Name: "SubscriberName", Value: "GPClient"},
		)

		coll.parseWinlogon(startEvent, 805, ts)
		coll.parseWinlogon(endEvent, 806, ts.Add(2*time.Second))

		require.Len(t, coll.subscribers, 1)
		assert.Equal(t, "GPClient", coll.subscribers[0].Name)
		assert.Equal(t, ts, coll.subscribers[0].Start)
		assert.Equal(t, ts.Add(2*time.Second), coll.subscribers[0].End)
		assert.Equal(t, 2*time.Second, coll.subscribers[0].Duration)
	})

	t.Run("806 without prior 805 is ignored", func(t *testing.T) {
		coll := newCollector()
		endEvent := makeEvent(*guidWinlogon, 806, ts,
			etw.EventProperty{Name: "SubscriberName", Value: "GPClient"},
		)

		coll.parseWinlogon(endEvent, 806, ts)

		assert.Empty(t, coll.subscribers)
	})

	t.Run("multiple subscriber pairs tracked independently", func(t *testing.T) {
		coll := newCollector()

		coll.parseWinlogon(makeEvent(*guidWinlogon, 805, ts,
			etw.EventProperty{Name: "SubscriberName", Value: "GPClient"},
		), 805, ts)
		coll.parseWinlogon(makeEvent(*guidWinlogon, 805, ts.Add(1*time.Second),
			etw.EventProperty{Name: "SubscriberName", Value: "Profiles"},
		), 805, ts.Add(1*time.Second))

		coll.parseWinlogon(makeEvent(*guidWinlogon, 806, ts.Add(3*time.Second),
			etw.EventProperty{Name: "SubscriberName", Value: "GPClient"},
		), 806, ts.Add(3*time.Second))
		coll.parseWinlogon(makeEvent(*guidWinlogon, 806, ts.Add(4*time.Second),
			etw.EventProperty{Name: "SubscriberName", Value: "Profiles"},
		), 806, ts.Add(4*time.Second))

		require.Len(t, coll.subscribers, 2)
		assert.Equal(t, "GPClient", coll.subscribers[0].Name)
		assert.Equal(t, 3*time.Second, coll.subscribers[0].Duration)
		assert.Equal(t, "Profiles", coll.subscribers[1].Name)
		assert.Equal(t, 3*time.Second, coll.subscribers[1].Duration)
	})

	t.Run("subscriber with empty name is skipped", func(t *testing.T) {
		coll := newCollector()
		e := makeEvent(*guidWinlogon, 805, ts,
			etw.EventProperty{Name: "SomeUnknownProp", Value: "unknown_value"},
		)

		coll.parseWinlogon(e, 805, ts)

		assert.Empty(t, coll.currentSubs)
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

	t.Run("sets DesktopReady", func(t *testing.T) {
		coll := newCollector()
		coll.parseShellCore(nil, 62171, ts)
		assert.Equal(t, ts, coll.timeline.DesktopReady)
	})

	t.Run("first-write-wins", func(t *testing.T) {
		coll := newCollector()
		ts2 := ts.Add(5 * time.Second)
		coll.parseShellCore(nil, 62171, ts)
		coll.parseShellCore(nil, 62171, ts2)
		assert.Equal(t, ts, coll.timeline.DesktopReady)
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
		e := makeEvent(*guidShellCore, 62171, ts)

		processEvent(coll, e)

		assert.Equal(t, ts, coll.timeline.DesktopReady)
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
		makeEvent(*guidWinlogon, 108, boot.Add(10*time.Second)),
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
		makeEvent(*guidShellCore, 62171, boot.Add(60*time.Second)),
		makeEvent(*guidWinlogon, 5002, boot.Add(60*time.Second)),
	}

	for _, e := range events {
		processEvent(coll, e)
	}

	tl := coll.timeline
	assert.Equal(t, boot, tl.BootStart)
	assert.Equal(t, boot.Add(1*time.Second), tl.SmssStart)
	assert.Equal(t, boot.Add(4*time.Second), tl.WinlogonInit)
	assert.Equal(t, boot.Add(10*time.Second), tl.LSMReady)
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
	assert.Equal(t, boot.Add(60*time.Second), tl.DesktopReady)
	assert.Equal(t, boot.Add(60*time.Second), tl.LogonStop)

	custom := buildCustomPayload(tl)
	durations := custom["durations"].(map[string]interface{})
	assert.Equal(t, int64(60000), durations["Total Boot Duration (ms)"])
	assert.Equal(t, int64(30000), durations["Total Logon Duration (ms)"])
	assert.Equal(t, int64(4000), durations["Profile Creation Duration (ms)"])
	assert.Equal(t, int64(8000), durations["Machine GP Duration (ms)"])
}
