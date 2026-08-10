// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && test

package logondurationimpl

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/impl"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	logscompressionmock "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
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

		// gap = SessionLogon(29s) - LoginUIDone(10s) = 19000ms; milestones
		// at/after SessionLogon have their offset collapsed by the idle gap.
		expected := []struct {
			name     string
			offsetMs float64
		}{
			{"Boot Duration", 0},
			{"Login UI Start", 8000},
			{"Computer Group Policy", 12000},
			{"User Group Policy", 13000},
			{"Logon Duration", 10000},
			{"Profile Loaded", 12000},
			{"Profile Created", 14000},
			{"Execute Shell Commands", 21000},
			{"Explorer Initializing", 32000},
			{"Desktop Visible", 34000},
			{"Desktop Startup Apps", 39000},
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

// newPayloadTestComponent builds a component wired to a no-op forwarder, whose
// Purge returns the exact bytes sendEvent marshalled.
func newPayloadTestComponent(t *testing.T) (*logonDurationComponent, eventplatform.Forwarder) {
	t.Helper()
	hostname := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
	compression := fxutil.Test[logscompression.Component](t, logscompressionmock.MockModule())
	forwarder := eventplatformimpl.NewNoopEventPlatformForwarder(hostname, compression)

	return &logonDurationComponent{
		hostname:               hostname,
		eventPlatformForwarder: forwarder,
	}, forwarder
}

// submitAndDecodeCustom submits a result and returns the custom attribute bag
// as it appears on the wire.
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
	// A trace with no Group Policy events at all omits the block entirely, so
	// existing consumers see exactly what they saw before.
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

	// Adding the block must not perturb either existing key.
	for _, key := range []string{"boot_timeline", "durations"} {
		before, err := json.Marshal(withoutGP[key])
		require.NoError(t, err)
		after, err := json.Marshal(withGP[key])
		require.NoError(t, err)
		assert.JSONEq(t, string(before), string(after), "%s must be unaffected", key)
	}

	// submitEvent formats this value with %d, so its Go type is load-bearing.
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
	assert.NotContains(t, inv, "error_code", "a successful invocation carries no code")

	// GPOs are inlined, so a consumer rendering pass -> CSE -> GPO needs no join.
	gpos := inv["gpos"].([]interface{})
	require.Len(t, gpos, 1)
	assert.Equal(t, "{31B2F340-016D-11D2-945F-00C04FB984F9}", gpos[0].(map[string]interface{})["id"])
	assert.Equal(t, "Default Domain Policy", gpos[0].(map[string]interface{})["name"])

	// A pass with nothing measured is simply absent; boot_timeline is what says
	// whether the pass ran.
	assert.NotContains(t, block, "user")

	// The existing keys still ride alongside.
	assert.Contains(t, custom, "boot_timeline")
}

func TestSubmitEvent_PayloadSizeStaysUnderWarnThreshold(t *testing.T) {
	// A pessimistic but realistic worst case: Windows registers roughly 30
	// extensions per scope, so 60 per pass is double that, and 20 applicable
	// GPOs per extension is far past what a normal domain applies. This is the
	// empirical backing for inlining GPOs instead of pooling them in a shared
	// table, and it fails here rather than as an invisible intake rejection if
	// a future field blows the budget.
	const (
		invocationsPerPass = 60
		gposPerInvocation  = 20
	)
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	gpos := make([]GPORef, gposPerInvocation)
	for i := range gpos {
		gpos[i] = GPORef{
			ID:   fmt.Sprintf("{%08X-016D-11D2-945F-00C04FB984F9}", i),
			Name: strings.Repeat("G", maxGPONameBytes),
		}
	}

	pass := func() []CSEInvocation {
		invs := make([]CSEInvocation, 0, invocationsPerPass)
		for i := 0; i < invocationsPerPass; i++ {
			invs = append(invs, CSEInvocation{
				CSEID:      fmt.Sprintf("{%08X-683F-11D2-A89A-00C04FBBCFA2}", i),
				CSEName:    strings.Repeat("N", maxCSENameBytes),
				OffsetMs:   int64(i) * 1000,
				DurationMs: int64(i),
				Result:     cseResultError,
				ErrorCode:  "0x8000000A",
				Async:      true,
				GPOs:       gpos,
			})
		}
		return invs
	}

	_, size := submitAndDecodeCustom(t, &AnalysisResult{
		Timeline:    fullBootTimeline(boot),
		GroupPolicy: &GroupPolicyDetails{Computer: pass(), User: pass()},
	})

	assert.Less(t, size, payloadSizeWarnBytes,
		"a worst-case payload must stay under the size warning threshold (was %d bytes)", size)
}

func TestSubmitEvent_PayloadFormat(t *testing.T) {
	hostname := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
	compression := fxutil.Test[logscompression.Component](t, logscompressionmock.MockModule())
	forwarder := eventplatformimpl.NewNoopEventPlatformForwarder(hostname, compression)

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
	compression := fxutil.Test[logscompression.Component](t, logscompressionmock.MockModule())
	forwarder := eventplatformimpl.NewNoopEventPlatformForwarder(hostname, compression)

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
			compression := fxutil.Test[logscompression.Component](t, logscompressionmock.MockModule())
			forwarder := eventplatformimpl.NewNoopEventPlatformForwarder(hostname, compression)

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
	compression := fxutil.Test[logscompression.Component](t, logscompressionmock.MockModule())
	forwarder := eventplatformimpl.NewNoopEventPlatformForwarder(hostname, compression)

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
