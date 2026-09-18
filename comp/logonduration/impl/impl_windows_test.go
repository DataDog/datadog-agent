// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && test

package logondurationimpl

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func fullBootTimeline(boot time.Time) BootTimeline {
	return BootTimeline{
		BootStart:                    boot,
		LoginUIStart:                 boot.Add(8 * time.Second),
		LoginUIDone:                  boot.Add(10 * time.Second),
		MachineGPStart:               boot.Add(12 * time.Second),
		MachineGPEnd:                 boot.Add(20 * time.Second),
		UserGPStart:                  boot.Add(32 * time.Second),
		UserGPEnd:                    boot.Add(38 * time.Second),
		SessionLogon:                 boot.Add(29 * time.Second),
		ProfileLoadStart:             boot.Add(31 * time.Second),
		ProfileLoadEnd:               boot.Add(34 * time.Second),
		ProfileCreationStart:         boot.Add(33 * time.Second),
		ProfileCreationEnd:           boot.Add(36 * time.Second),
		ExecuteShellCommandListStart: boot.Add(40 * time.Second),
		ExecuteShellCommandListEnd:   boot.Add(45 * time.Second),
		ExplorerStart:                boot.Add(50 * time.Second),
		ExplorerInitStart:            boot.Add(51 * time.Second),
		ExplorerInitEnd:              boot.Add(54 * time.Second),
		DesktopCreateStart:           boot.Add(53 * time.Second),
		DesktopCreateEnd:             boot.Add(56 * time.Second),
		DesktopVisibleStart:          boot.Add(55 * time.Second),
		DesktopVisibleEnd:            boot.Add(57 * time.Second),
		DesktopStartupAppsStart:      boot.Add(58 * time.Second),
		DesktopStartupAppsEnd:        boot.Add(62 * time.Second),
	}
}

// straddlingGPTimeline is a real capture, in ms off boot: the computer pass starts at the
// login screen and is still running when the session logs on.
func straddlingGPTimeline(boot time.Time) BootTimeline {
	ms := func(n int) time.Time { return boot.Add(time.Duration(n) * time.Millisecond) }
	return BootTimeline{
		BootStart:           boot,
		LoginUIStart:        ms(8698),
		LoginUIDone:         ms(8746),
		MachineGPStart:      ms(10808),
		MachineGPEnd:        ms(39337),
		SessionLogon:        ms(36486),
		DesktopVisibleStart: ms(47159),
	}
}

func twoBusyIntervalTimeline(boot time.Time) BootTimeline {
	return BootTimeline{
		BootStart:           boot,
		LoginUIStart:        boot.Add(8 * time.Second),
		LoginUIDone:         boot.Add(10 * time.Second),
		MachineGPStart:      boot.Add(12 * time.Second),
		MachineGPEnd:        boot.Add(20 * time.Second),
		ExplorerInitStart:   boot.Add(30 * time.Second),
		ExplorerInitEnd:     boot.Add(40 * time.Second),
		SessionLogon:        boot.Add(50 * time.Second),
		DesktopVisibleStart: boot.Add(70 * time.Second),
	}
}

func nestedBusyIntervalTimeline(boot time.Time) BootTimeline {
	return BootTimeline{
		BootStart:           boot,
		LoginUIStart:        boot.Add(8 * time.Second),
		LoginUIDone:         boot.Add(10 * time.Second),
		MachineGPStart:      boot.Add(12 * time.Second),
		MachineGPEnd:        boot.Add(40 * time.Second),
		ProfileLoadStart:    boot.Add(20 * time.Second),
		ProfileLoadEnd:      boot.Add(30 * time.Second),
		SessionLogon:        boot.Add(50 * time.Second),
		DesktopVisibleStart: boot.Add(70 * time.Second),
	}
}

func milestonesByID(milestones []Milestone) map[string]Milestone {
	byID := make(map[string]Milestone, len(milestones))
	for _, m := range milestones {
		byID[m.ID] = m
	}
	return byID
}

func assertMonotoneTimeline(t *testing.T, milestones []Milestone) {
	t.Helper()
	ordered := make([]Milestone, len(milestones))
	copy(ordered, milestones)
	// Timestamp is fixed-width UTC, so lexical order is chronological order.
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Timestamp < ordered[j].Timestamp })
	for i := 1; i < len(ordered); i++ {
		prev, cur := ordered[i-1], ordered[i]
		assert.GreaterOrEqualf(t, cur.OffsetMs, prev.OffsetMs,
			"%s at %s renders at %.0f, behind %s at %s which renders at %.0f",
			cur.ID, cur.Timestamp, cur.OffsetMs, prev.ID, prev.Timestamp, prev.OffsetMs)
	}
}

func TestBootTimelineOffsetsAreMonotone(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	noMachinePass := fullBootTimeline(boot)
	noMachinePass.MachineGPStart = time.Time{}
	noMachinePass.MachineGPEnd = time.Time{}

	unterminatedPass := fullBootTimeline(boot)
	unterminatedPass.MachineGPEnd = time.Time{}

	passFromBeforeTheWait := fullBootTimeline(boot)
	passFromBeforeTheWait.MachineGPStart = boot.Add(9 * time.Second)
	passFromBeforeTheWait.MachineGPEnd = boot.Add(20 * time.Second)

	for name, tl := range map[string]BootTimeline{
		"full timeline":                  fullBootTimeline(boot),
		"machine pass straddles logon":   straddlingGPTimeline(boot),
		"no machine pass":                noMachinePass,
		"machine pass never ended":       unterminatedPass,
		"machine pass predates the wait": passFromBeforeTheWait,
		"two busy intervals":             twoBusyIntervalTimeline(boot),
		"nested busy intervals":          nestedBusyIntervalTimeline(boot),
	} {
		t.Run(name, func(t *testing.T) {
			assertMonotoneTimeline(t, buildTimelineMilestones(tl))
		})
	}
}

func TestBuildTimelineMilestones(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	t.Run("includes only non-zero timestamps", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:               boot,
			LoginUIStart:            boot.Add(1 * time.Second),
			DesktopStartupAppsStart: boot.Add(90 * time.Second),
		}

		milestones := buildTimelineMilestones(tl)

		assert.Len(t, milestones, 3)
		assert.Equal(t, "Boot Duration", milestones[0].Name)
		assert.Equal(t, "Login UI Start", milestones[1].Name)
		assert.Equal(t, "Desktop Startup Apps", milestones[2].Name)
	})

	t.Run("computes correct offsets from boot start", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:               boot,
			LoginUIStart:            boot.Add(2 * time.Second),
			DesktopStartupAppsStart: boot.Add(90 * time.Second),
		}

		milestones := buildTimelineMilestones(tl)

		assert.InDelta(t, 0.0, milestones[0].OffsetMs, 0.001)
		assert.InDelta(t, 2000.0, milestones[1].OffsetMs, 0.001)
		assert.InDelta(t, 90000.0, milestones[2].OffsetMs, 0.001)
	})

	t.Run("formats timestamps correctly", func(t *testing.T) {
		tl := BootTimeline{
			BootStart: boot,
		}

		milestones := buildTimelineMilestones(tl)

		require.Len(t, milestones, 1)
		assert.Equal(t, "2026-01-15T08:00:00.000Z", milestones[0].Timestamp)
	})

	t.Run("all zero timestamps returns empty slice", func(t *testing.T) {
		tl := BootTimeline{}

		milestones := buildTimelineMilestones(tl)

		assert.Empty(t, milestones)
	})

	t.Run("zero BootStart produces zero offsets", func(t *testing.T) {
		tl := BootTimeline{
			LoginUIStart:            boot.Add(1 * time.Second),
			DesktopStartupAppsStart: boot.Add(90 * time.Second),
		}

		milestones := buildTimelineMilestones(tl)

		require.Len(t, milestones, 2)
		assert.InDelta(t, 0.0, milestones[0].OffsetMs, 0.001)
		assert.InDelta(t, 0.0, milestones[1].OffsetMs, 0.001)
		assert.NotEmpty(t, milestones[0].Timestamp)
		assert.NotEmpty(t, milestones[1].Timestamp)
	})

	t.Run("full timeline includes all milestones in order", func(t *testing.T) {
		tl := fullBootTimeline(boot)

		milestones := buildTimelineMilestones(tl)

		require.Len(t, milestones, 11)

		// Region is LoginUIDone(10s) -> SessionLogon(29s) and the computer pass holds
		// [12s, 20s] of it, so 2000+9000ms is elided, not the full 19000ms.
		expected := []struct {
			name     string
			offsetMs float64
		}{
			{"Boot Duration", 0},
			{"Login UI Start", 8000},
			{"Computer Group Policy", 10000},
			{"User Group Policy", 21000},
			{"Logon Duration", 18000},
			{"Profile Loaded", 20000},
			{"Profile Created", 22000},
			{"Execute Shell Commands", 29000},
			{"Explorer Initializing", 40000},
			{"Desktop Visible", 42000},
			{"Desktop Startup Apps", 47000},
		}
		for i, exp := range expected {
			assert.Equal(t, exp.name, milestones[i].Name, "milestone %d name", i)
			assert.InDelta(t, exp.offsetMs, milestones[i].OffsetMs, 0.001, "milestone %d offset", i)
		}

		var bootDur, logonDur *Milestone
		for i := range milestones {
			switch milestones[i].ID {
			case "boot_duration":
				bootDur = &milestones[i]
			case "logon_duration":
				logonDur = &milestones[i]
			}
		}
		require.NotNil(t, bootDur, "boot_duration milestone missing")
		require.NotNil(t, logonDur, "logon_duration milestone missing")
		assert.InDelta(t, 8000.0, bootDur.DurationMs, 0.001)
		assert.InDelta(t, 26000.0, logonDur.DurationMs, 0.001)
	})

	t.Run("collapses idle gap while preserving wall-clock timestamps", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:           boot,
			LoginUIStart:        boot.Add(8 * time.Second),
			LoginUIDone:         boot.Add(10 * time.Second),
			SessionLogon:        boot.Add(29 * time.Second),
			DesktopVisibleStart: boot.Add(55 * time.Second),
		}

		milestones := buildTimelineMilestones(tl)

		var logon *Milestone
		for i := range milestones {
			if milestones[i].ID == "logon_duration" {
				logon = &milestones[i]
				break
			}
		}
		require.NotNil(t, logon, "logon_duration milestone missing")
		// raw offset 29000 - gap 19000 = 10000
		assert.InDelta(t, 10000.0, logon.OffsetMs, 0.001)
		// timestamp stays wall-clock (SessionLogon = boot + 29s)
		assert.Equal(t, "2026-01-15T08:00:29.000Z", logon.Timestamp)
	})

	t.Run("machine pass straddling logon keeps its full span", func(t *testing.T) {
		m := milestonesByID(buildTimelineMilestones(straddlingGPTimeline(boot)))

		// Only the 2062ms before the pass is unobserved; from there the machine is busy
		// through logon.
		gp, logon := m["computer_group_policy"], m["logon_duration"]
		assert.InDelta(t, 8746.0, gp.OffsetMs, 0.001)
		assert.InDelta(t, 28529.0, gp.DurationMs, 0.001)
		assert.InDelta(t, 34424.0, logon.OffsetMs, 0.001)
		assert.InDelta(t, 10673.0, logon.DurationMs, 0.001)
		assert.InDelta(t, 2851.0, (gp.OffsetMs+gp.DurationMs)-logon.OffsetMs, 0.001,
			"the pass outlives the start of logon by exactly as long as it really did")
	})

	t.Run("nothing observed in the region elides all of it", func(t *testing.T) {
		tl := fullBootTimeline(boot)
		tl.MachineGPStart = time.Time{}
		tl.MachineGPEnd = time.Time{}

		m := milestonesByID(buildTimelineMilestones(tl))

		// Nothing to preserve, so the whole 19000ms wait goes - the pre-fix numbers.
		for id, offset := range map[string]float64{
			"logon_duration":         10000,
			"profile_loaded":         12000,
			"user_group_policy":      13000,
			"profile_created":        14000,
			"execute_shell_commands": 21000,
			"explorer_initializing":  32000,
			"desktop_visible":        34000,
			"desktop_startup_apps":   39000,
		} {
			require.Contains(t, m, id)
			assert.InDelta(t, offset, m[id].OffsetMs, 0.001, id)
		}
	})

	t.Run("a pass with no end clamps to the start of the region", func(t *testing.T) {
		tl := fullBootTimeline(boot)
		tl.MachineGPEnd = time.Time{}

		m := milestonesByID(buildTimelineMilestones(tl))

		// No span to preserve, so the region is elided whole and the point lands on its start.
		assert.InDelta(t, 10000.0, m["computer_group_policy"].OffsetMs, 0.001)
		assert.InDelta(t, 0.0, m["computer_group_policy"].DurationMs, 0.001)
		assert.InDelta(t, 10000.0, m["logon_duration"].OffsetMs, 0.001)
		assert.InDelta(t, 39000.0, m["desktop_startup_apps"].OffsetMs, 0.001)
	})

	t.Run("two busy intervals in the region elide the three stretches around them", func(t *testing.T) {
		m := milestonesByID(buildTimelineMilestones(twoBusyIntervalTimeline(boot)))

		// Elided: [10s,12s), [20s,30s), [40s,50s) = 22000ms. Both spans keep their width.
		assert.InDelta(t, 10000.0, m["computer_group_policy"].OffsetMs, 0.001)
		assert.InDelta(t, 8000.0, m["computer_group_policy"].DurationMs, 0.001)
		assert.InDelta(t, 18000.0, m["explorer_initializing"].OffsetMs, 0.001)
		assert.InDelta(t, 10000.0, m["explorer_initializing"].DurationMs, 0.001)
		assert.InDelta(t, 28000.0, m["logon_duration"].OffsetMs, 0.001)
	})

	t.Run("an interval nested in another does not rewind the merge cursor", func(t *testing.T) {
		m := milestonesByID(buildTimelineMilestones(nestedBusyIntervalTimeline(boot)))

		// [20s,30s) lies inside [12s,40s), so only [10s,12s) and [40s,50s) go: 12000ms,
		// not the 22000ms a cursor that stepped back to 30s would elide.
		assert.InDelta(t, 10000.0, m["computer_group_policy"].OffsetMs, 0.001)
		assert.InDelta(t, 18000.0, m["profile_loaded"].OffsetMs, 0.001)
		assert.InDelta(t, 38000.0, m["logon_duration"].OffsetMs, 0.001)
	})

	t.Run("a pass reaching into the region from before it keeps its full width", func(t *testing.T) {
		tl := fullBootTimeline(boot)
		tl.MachineGPStart = boot.Add(9 * time.Second)
		tl.MachineGPEnd = boot.Add(20 * time.Second)

		m := milestonesByID(buildTimelineMilestones(tl))

		// Only [20s, 29s) is unobserved; the part before LoginUIDone is outside the region.
		gp := m["computer_group_policy"]
		assert.InDelta(t, 9000.0, gp.OffsetMs, 0.001)
		assert.InDelta(t, 11000.0, gp.DurationMs, 0.001)
		assert.InDelta(t, 20000.0, m["logon_duration"].OffsetMs, 0.001)
	})

	t.Run("desktop_visible merged spans DesktopCreateStart to DesktopVisibleEnd", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:          boot,
			DesktopCreateStart: boot.Add(53 * time.Second),
			DesktopVisibleEnd:  boot.Add(57 * time.Second),
		}

		milestones := buildTimelineMilestones(tl)

		var dv *Milestone
		for i := range milestones {
			if milestones[i].ID == "desktop_visible" {
				dv = &milestones[i]
				break
			}
		}
		require.NotNil(t, dv, "desktop_visible milestone missing")
		assert.Equal(t, "Desktop Visible", dv.Name)
		assert.InDelta(t, 53000.0, dv.OffsetMs, 0.001)
		assert.InDelta(t, 4000.0, dv.DurationMs, 0.001)

		for _, m := range milestones {
			assert.NotEqual(t, "desktop_created", m.ID)
			assert.NotEqual(t, "desktop_ready", m.ID)
		}
	})
}

func TestBuildCustomPayload(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	t.Run("includes total boot duration as sum of boot and logon", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:           boot,
			LoginUIStart:        boot.Add(10 * time.Second),
			SessionLogon:        boot.Add(30 * time.Second),
			DesktopVisibleStart: boot.Add(90 * time.Second),
		}

		custom := buildCustomPayload(tl, nil)

		durations, ok := custom["durations"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, int64(10000), durations["boot_duration_ms"])
		assert.Equal(t, int64(60000), durations["logon_duration_ms"])
		assert.Equal(t, int64(70000), durations["total_boot_duration_ms"])

		// boot_duration / logon_duration milestones must not leak bare keys
		// that duplicate the authoritative *_ms keys.
		_, hasBootDup := durations["boot_duration"]
		_, hasLogonDup := durations["logon_duration"]
		assert.False(t, hasBootDup)
		assert.False(t, hasLogonDup)
	})

	t.Run("omits total boot duration when only boot duration available", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:    boot,
			LoginUIStart: boot.Add(10 * time.Second),
		}

		custom := buildCustomPayload(tl, nil)

		durations, ok := custom["durations"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, int64(10000), durations["boot_duration_ms"])
		_, hasTotal := durations["total_boot_duration_ms"]
		assert.False(t, hasTotal)
	})

	t.Run("includes logon duration", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:           boot,
			SessionLogon:        boot.Add(30 * time.Second),
			DesktopVisibleStart: boot.Add(90 * time.Second),
		}

		custom := buildCustomPayload(tl, nil)

		durations := custom["durations"].(map[string]interface{})
		assert.Equal(t, int64(60000), durations["logon_duration_ms"])
	})

	t.Run("includes boot duration", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:    boot,
			LoginUIStart: boot.Add(8 * time.Second),
		}

		custom := buildCustomPayload(tl, nil)

		durations, ok := custom["durations"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, int64(8000), durations["boot_duration_ms"])
	})

	t.Run("omits durations when end timestamp is zero", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:    boot,
			SessionLogon: boot.Add(30 * time.Second),
		}

		custom := buildCustomPayload(tl, nil)

		if durations, ok := custom["durations"].(map[string]interface{}); ok {
			_, hasLogon := durations["logon_duration_ms"]
			assert.False(t, hasLogon)
		}
	})

	t.Run("no durations key when nothing computable", func(t *testing.T) {
		tl := BootTimeline{
			BootStart: boot,
			// no end timestamps set
		}

		custom := buildCustomPayload(tl, nil)

		_, hasDurations := custom["durations"]
		assert.False(t, hasDurations)
	})

	t.Run("always includes boot_timeline key", func(t *testing.T) {
		tl := BootTimeline{}

		custom := buildCustomPayload(tl, nil)

		_, hasTimeline := custom["boot_timeline"]
		assert.True(t, hasTimeline)
	})

}

// newPayloadTestComponent wires a component to a no-op forwarder whose Purge returns the bytes sendEvent marshalled.
func newPayloadTestComponent(t *testing.T) (*logonDurationComponent, eventplatform.Forwarder) {
	t.Helper()
	hostname := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
	forwarder := eventplatformimpl.NewNoopEventPlatformForwarder(hostname)

	return &logonDurationComponent{
		hostname:               hostname,
		eventPlatformForwarder: forwarder,
	}, forwarder
}

// submitAndDecodeCustom submits a result and returns the custom bag as it appears on the wire.
func submitAndDecodeCustom(t *testing.T, result *AnalysisResult) (map[string]interface{}, int) {
	t.Helper()
	comp, forwarder := newPayloadTestComponent(t)
	require.NoError(t, comp.submitEvent(result))

	msgs := forwarder.Purge()[eventplatform.EventTypeEventManagement]
	require.Len(t, msgs, 1)
	content := msgs[0].GetContent()

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(content, &payload))

	attrs := payload["data"].(map[string]interface{})["attributes"].(map[string]interface{})
	custom := attrs["attributes"].(map[string]interface{})["custom"].(map[string]interface{})
	return custom, len(content)
}

func TestBuildCustomPayload_GroupPolicyAbsentWhenNil(t *testing.T) {
	custom := buildCustomPayload(fullBootTimeline(time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)), nil)
	assert.NotContains(t, custom, "group_policy_details")
	assert.Contains(t, custom, "boot_timeline")
	assert.Contains(t, custom, "durations")
}

func TestBuildCustomPayload_ExistingKeysUnchanged(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	tl := fullBootTimeline(boot)

	withoutGP := buildCustomPayload(tl, nil)
	withGP := buildCustomPayload(tl, &GroupPolicyDetails{
		Computer: []CSEInvocation{{CSEID: "{35378EAC-683F-11D2-A89A-00C04FBBCFA2}"}},
	})

	for _, key := range []string{"boot_timeline", "durations"} {
		before, err := json.Marshal(withoutGP[key])
		require.NoError(t, err)
		after, err := json.Marshal(withGP[key])
		require.NoError(t, err)
		assert.JSONEq(t, string(before), string(after), "%s must be unaffected", key)
	}

	// An int64 on the wire, not a float: the payload is a shipped contract.
	durations := withGP["durations"].(map[string]interface{})
	_, isInt64 := durations["total_boot_duration_ms"].(int64)
	assert.True(t, isInt64, "total_boot_duration_ms must stay an int64")
}

func TestSubmitEvent_GroupPolicyReachesTheWire(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	gp := &GroupPolicyDetails{
		Computer: []CSEInvocation{{
			CSEID:      "{35378EAC-683F-11D2-A89A-00C04FBBCFA2}",
			CSEName:    "Registry",
			OffsetMs:   12500,
			DurationMs: 1250,
			Result:     cseResultSuccess,
			GPOs: []GPORef{{
				ID:   "{31B2F340-016D-11D2-945F-00C04FB984F9}",
				Name: "Default Domain Policy",
			}},
		}},
	}

	custom, _ := submitAndDecodeCustom(t, &AnalysisResult{
		Timeline:    BootTimeline{BootStart: boot},
		GroupPolicy: gp,
	})

	block, ok := custom["group_policy_details"].(map[string]interface{})
	require.True(t, ok, "group_policy_details should be present under custom")

	invocations := block["computer"].([]interface{})
	require.Len(t, invocations, 1)
	inv := invocations[0].(map[string]interface{})
	assert.Equal(t, "{35378EAC-683F-11D2-A89A-00C04FBBCFA2}", inv["cse_id"])
	assert.Equal(t, "Registry", inv["cse_name"])
	assert.Equal(t, float64(12500), inv["offset_ms"])
	assert.Equal(t, float64(1250), inv["duration_ms"])
	assert.Equal(t, "success", inv["result"])
	assert.NotContains(t, inv, "scope", "the enclosing array carries the scope")

	gpos := inv["gpos"].([]interface{})
	require.Len(t, gpos, 1)
	assert.Equal(t, "{31B2F340-016D-11D2-945F-00C04FB984F9}", gpos[0].(map[string]interface{})["id"])
	assert.Equal(t, "Default Domain Policy", gpos[0].(map[string]interface{})["name"])

	assert.NotContains(t, block, "user")
	assert.NotContains(t, block, "computer_cses_omitted", "a pass under the cap carries no truncation count")

	assert.Contains(t, custom, "boot_timeline")
}

func TestSubmitEvent_WorstCasePayloadSize(t *testing.T) {
	// The four caps are coupled through this one byte budget, so raising any of them fails here.
	// Uncompressed bytes, and deliberately well below the intake's own limit: the margin is the point.
	const maxWorstCaseBytes = 3_000_000

	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	gpos := make([]GPORef, maxGPOsPerCSE)
	for i := range gpos {
		gpos[i] = GPORef{
			ID:   fmt.Sprintf("{%08X-016D-11D2-945F-00C04FB984F9}", i),
			Name: strings.Repeat("G", maxGPONameBytes),
		}
	}

	pass := func() []CSEInvocation {
		invs := make([]CSEInvocation, 0, maxCSEInvocationsPerScope)
		for i := 0; i < maxCSEInvocationsPerScope; i++ {
			invs = append(invs, CSEInvocation{
				CSEID:       fmt.Sprintf("{%08X-683F-11D2-A89A-00C04FBBCFA2}", i),
				CSEName:     strings.Repeat("N", maxCSENameBytes),
				OffsetMs:    int64(i) * 1000,
				DurationMs:  int64(i),
				Result:      cseResultError,
				Async:       true,
				GPOs:        gpos,
				GPOsOmitted: 4096,
			})
		}
		return invs
	}

	_, size := submitAndDecodeCustom(t, &AnalysisResult{
		Timeline: fullBootTimeline(boot),
		GroupPolicy: &GroupPolicyDetails{
			Computer:            pass(),
			ComputerCSEsOmitted: 4096,
			User:                pass(),
			UserCSEsOmitted:     4096,
		},
	})

	t.Logf("worst case %d bytes of %d (%d invocations x %d GPO refs at %d-byte names), %d bytes of margin",
		size, maxWorstCaseBytes, 2*maxCSEInvocationsPerScope, maxGPOsPerCSE, maxGPONameBytes,
		maxWorstCaseBytes-size)

	assert.Less(t, size, maxWorstCaseBytes,
		"the largest payload the caps permit must stay under the ceiling (was %d bytes for %d invocations x %d GPO refs)",
		size, 2*maxCSEInvocationsPerScope, maxGPOsPerCSE)
}

func TestSubmitEvent_PayloadFormat(t *testing.T) {
	hostname := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
	forwarder := eventplatformimpl.NewNoopEventPlatformForwarder(hostname)

	comp := &logonDurationComponent{
		hostname:               hostname,
		eventPlatformForwarder: forwarder,
	}

	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	result := &AnalysisResult{
		Timeline: BootTimeline{
			BootStart:           boot,
			SessionLogon:        boot.Add(30 * time.Second),
			DesktopVisibleStart: boot.Add(90 * time.Second),
		},
	}

	err := comp.submitEvent(result)
	require.NoError(t, err)

	sent := forwarder.Purge()
	msgs := sent[eventplatform.EventTypeEventManagement]
	require.Len(t, msgs, 1)

	var payload map[string]interface{}
	err = json.Unmarshal(msgs[0].GetContent(), &payload)
	require.NoError(t, err)

	data, ok := payload["data"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "event", data["type"])

	attrs, ok := data["attributes"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "Device booted up: Boot timeline incomplete", attrs["title"])
	assert.Equal(t, "alert", attrs["category"])
	assert.Equal(t, "system-notable-events", attrs["integration_id"])

	_, hasHost := attrs["host"]
	assert.True(t, hasHost)
	_, hasTimestamp := attrs["timestamp"]
	assert.True(t, hasTimestamp)

	nestedAttrs, ok := attrs["attributes"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "ok", nestedAttrs["status"])
	assert.Equal(t, "3", nestedAttrs["priority"])

	custom, ok := nestedAttrs["custom"].(map[string]interface{})
	require.True(t, ok)
	_, hasTimeline := custom["boot_timeline"]
	assert.True(t, hasTimeline)
	_, hasDurations := custom["durations"]
	assert.True(t, hasDurations)
}

func TestSubmitEvent_MessageIncludesTotalDuration(t *testing.T) {
	hostname := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
	forwarder := eventplatformimpl.NewNoopEventPlatformForwarder(hostname)

	comp := &logonDurationComponent{
		hostname:               hostname,
		eventPlatformForwarder: forwarder,
	}

	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	result := &AnalysisResult{
		Timeline: BootTimeline{
			BootStart:           boot,
			LoginUIStart:        boot.Add(10 * time.Second),
			SessionLogon:        boot.Add(30 * time.Second),
			DesktopVisibleStart: boot.Add(90 * time.Second),
		},
	}

	err := comp.submitEvent(result)
	require.NoError(t, err)

	sent := forwarder.Purge()
	msgs := sent[eventplatform.EventTypeEventManagement]
	require.Len(t, msgs, 1)

	var payload map[string]interface{}
	err = json.Unmarshal(msgs[0].GetContent(), &payload)
	require.NoError(t, err)

	data := payload["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	assert.Equal(t, "Total boot duration took 70000 ms.", attrs["message"])
}

func TestSubmitEvent_TitleReflectsCompleteness(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		timeline BootTimeline
		expected string
	}{
		{
			name: "complete",
			timeline: BootTimeline{
				BootStart:           boot,
				LoginUIStart:        boot.Add(10 * time.Second),
				SessionLogon:        boot.Add(30 * time.Second),
				DesktopVisibleStart: boot.Add(90 * time.Second),
			},
			expected: "Device booted up: Boot & login took 70000 ms",
		},
		{
			name: "boot only",
			timeline: BootTimeline{
				BootStart:    boot,
				LoginUIStart: boot.Add(10 * time.Second),
			},
			expected: "Device booted up: Boot timeline incomplete",
		},
		{
			name: "logon only",
			timeline: BootTimeline{
				BootStart:           boot,
				SessionLogon:        boot.Add(30 * time.Second),
				DesktopVisibleStart: boot.Add(90 * time.Second),
			},
			expected: "Device booted up: Boot timeline incomplete",
		},
		{
			name:     "neither",
			timeline: BootTimeline{BootStart: boot},
			expected: "Device booted up: Boot timeline incomplete",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hostname := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
			forwarder := eventplatformimpl.NewNoopEventPlatformForwarder(hostname)

			comp := &logonDurationComponent{
				hostname:               hostname,
				eventPlatformForwarder: forwarder,
			}

			err := comp.submitEvent(&AnalysisResult{Timeline: tc.timeline})
			require.NoError(t, err)

			sent := forwarder.Purge()
			msgs := sent[eventplatform.EventTypeEventManagement]
			require.Len(t, msgs, 1)

			var payload map[string]interface{}
			require.NoError(t, json.Unmarshal(msgs[0].GetContent(), &payload))
			attrs := payload["data"].(map[string]interface{})["attributes"].(map[string]interface{})
			assert.Equal(t, tc.expected, attrs["title"])
		})
	}
}

func TestSubmitEvent_FallbackMessageWhenNoDuration(t *testing.T) {
	hostname := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
	forwarder := eventplatformimpl.NewNoopEventPlatformForwarder(hostname)

	comp := &logonDurationComponent{
		hostname:               hostname,
		eventPlatformForwarder: forwarder,
	}

	result := &AnalysisResult{
		Timeline: BootTimeline{},
	}

	err := comp.submitEvent(result)
	require.NoError(t, err)

	sent := forwarder.Purge()
	msgs := sent[eventplatform.EventTypeEventManagement]
	require.Len(t, msgs, 1)

	var payload map[string]interface{}
	err = json.Unmarshal(msgs[0].GetContent(), &payload)
	require.NoError(t, err)

	data := payload["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	assert.Equal(t, "Total boot duration analysis after reboot", attrs["message"])
}
