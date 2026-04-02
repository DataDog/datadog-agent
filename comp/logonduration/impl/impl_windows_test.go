// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && test

package logondurationimpl

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	logscompressionmock "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func fullBootTimeline(boot time.Time) BootTimeline {
	return BootTimeline{
		BootStart:                    boot,
		SmssStart:                    boot.Add(1 * time.Second),
		UserSmssStart:                boot.Add(5 * time.Second),
		WinlogonStart:                boot.Add(3 * time.Second),
		WinlogonInit:                 boot.Add(4 * time.Second),
		WinlogonInitDone:             boot.Add(6 * time.Second),
		LoginUIStart:                 boot.Add(8 * time.Second),
		LoginUIDone:                  boot.Add(10 * time.Second),
		MachineGPStart:               boot.Add(12 * time.Second),
		MachineGPEnd:                 boot.Add(20 * time.Second),
		UserGPStart:                  boot.Add(32 * time.Second),
		UserGPEnd:                    boot.Add(38 * time.Second),
		UserWinlogonStart:            boot.Add(25 * time.Second),
		SessionLogon:                 boot.Add(29 * time.Second),
		ProfileLoadStart:             boot.Add(31 * time.Second),
		ProfileLoadEnd:               boot.Add(34 * time.Second),
		ProfileCreationStart:         boot.Add(33 * time.Second),
		ProfileCreationEnd:           boot.Add(36 * time.Second),
		ExecuteShellCommandListStart: boot.Add(40 * time.Second),
		ExecuteShellCommandListEnd:   boot.Add(45 * time.Second),
		UserinitStart:                boot.Add(42 * time.Second),
		ExplorerStart:                boot.Add(50 * time.Second),
		ExplorerInitStart:            boot.Add(51 * time.Second),
		ExplorerInitEnd:              boot.Add(54 * time.Second),
		DesktopCreateStart:           boot.Add(53 * time.Second),
		DesktopCreateEnd:             boot.Add(56 * time.Second),
		DesktopVisibleStart:          boot.Add(55 * time.Second),
		DesktopVisibleEnd:            boot.Add(57 * time.Second),
		DesktopStartupAppsStart:      boot.Add(58 * time.Second),
		DesktopStartupAppsEnd:        boot.Add(62 * time.Second),
		DesktopReadyStart:            boot.Add(59 * time.Second),
		DesktopReadyEnd:              boot.Add(65 * time.Second),
	}
}

func TestBuildTimelineMilestones(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	t.Run("includes only non-zero timestamps", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:         boot,
			SmssStart:         boot.Add(1 * time.Second),
			DesktopReadyStart: boot.Add(90 * time.Second),
		}

		milestones := buildTimelineMilestones(tl)

		assert.Len(t, milestones, 3)
		assert.Equal(t, "Boot Start", milestones[0].Name)
		assert.Equal(t, "SMSS Start", milestones[1].Name)
		assert.Equal(t, "Desktop Ready", milestones[2].Name)
	})

	t.Run("computes correct offsets from boot start", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:         boot,
			SmssStart:         boot.Add(2 * time.Second),
			DesktopReadyStart: boot.Add(90 * time.Second),
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
			SmssStart:         boot.Add(1 * time.Second),
			DesktopReadyStart: boot.Add(90 * time.Second),
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

		require.Len(t, milestones, 20)

		expected := []struct {
			name     string
			offsetMs float64
		}{
			{"Boot Start", 0},
			{"SMSS Start", 1000},
			{"User Session SMSS Start", 5000},
			{"Winlogon Start", 3000},
			{"Winlogon Init", 4000},
			{"Login UI Start", 8000},
			{"Computer Group Policy", 12000},
			{"User Group Policy", 32000},
			{"User Session Winlogon Start", 25000},
			{"User Logon", 29000},
			{"Profile Loaded", 31000},
			{"Profile Created", 33000},
			{"Execute Shell Commands", 40000},
			{"Userinit.exe", 42000},
			{"Explorer.exe Start", 50000},
			{"Explorer Initializing", 51000},
			{"Desktop Created", 53000},
			{"Desktop Visible", 55000},
			{"Desktop Startup Apps", 58000},
			{"Desktop Ready", 59000},
		}
		for i, exp := range expected {
			assert.Equal(t, exp.name, milestones[i].Name, "milestone %d name", i)
			assert.InDelta(t, exp.offsetMs, milestones[i].OffsetMs, 0.001, "milestone %d offset", i)
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

		custom := buildCustomPayload(tl)

		durations, ok := custom["durations"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, int64(10000), durations["boot_duration_ms"])
		assert.Equal(t, int64(60000), durations["logon_duration_ms"])
		assert.Equal(t, int64(70000), durations["total_boot_duration_ms"])
	})

	t.Run("omits total boot duration when only boot duration available", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:    boot,
			LoginUIStart: boot.Add(10 * time.Second),
		}

		custom := buildCustomPayload(tl)

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

		custom := buildCustomPayload(tl)

		durations := custom["durations"].(map[string]interface{})
		assert.Equal(t, int64(60000), durations["logon_duration_ms"])
	})

	t.Run("includes boot duration", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:    boot,
			LoginUIStart: boot.Add(8 * time.Second),
		}

		custom := buildCustomPayload(tl)

		durations, ok := custom["durations"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, int64(8000), durations["boot_duration_ms"])
	})

	t.Run("omits durations when end timestamp is zero", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:    boot,
			SessionLogon: boot.Add(30 * time.Second),
		}

		custom := buildCustomPayload(tl)

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

		custom := buildCustomPayload(tl)

		_, hasDurations := custom["durations"]
		assert.False(t, hasDurations)
	})

	t.Run("always includes boot_timeline key", func(t *testing.T) {
		tl := BootTimeline{}

		custom := buildCustomPayload(tl)

		_, hasTimeline := custom["boot_timeline"]
		assert.True(t, hasTimeline)
	})

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
	assert.Equal(t, "Logon duration", attrs["title"])
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

