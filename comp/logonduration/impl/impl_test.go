// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

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

func TestBuildTimelineMilestones(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	t.Run("includes only non-zero timestamps", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:    boot,
			SmssStart:    boot.Add(1 * time.Second),
			DesktopReady: boot.Add(90 * time.Second),
		}

		milestones := buildTimelineMilestones(tl)

		assert.Len(t, milestones, 3)
		assert.Equal(t, "Boot Start", milestones[0].Name)
		assert.Equal(t, "SMSS Start", milestones[1].Name)
		assert.Equal(t, "Desktop Ready", milestones[2].Name)
	})

	t.Run("computes correct offsets from boot start", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:    boot,
			SmssStart:    boot.Add(2 * time.Second),
			DesktopReady: boot.Add(90 * time.Second),
		}

		milestones := buildTimelineMilestones(tl)

		assert.InDelta(t, 0.0, milestones[0].OffsetS, 0.001)
		assert.InDelta(t, 2.0, milestones[1].OffsetS, 0.001)
		assert.InDelta(t, 90.0, milestones[2].OffsetS, 0.001)
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
			SmssStart:    boot.Add(1 * time.Second),
			DesktopReady: boot.Add(90 * time.Second),
		}

		milestones := buildTimelineMilestones(tl)

		require.Len(t, milestones, 2)
		assert.InDelta(t, 0.0, milestones[0].OffsetS, 0.001)
		assert.InDelta(t, 0.0, milestones[1].OffsetS, 0.001)
		assert.NotEmpty(t, milestones[0].Timestamp)
		assert.NotEmpty(t, milestones[1].Timestamp)
	})

	t.Run("full timeline includes all milestones in order", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:                    boot,
			SmssStart:                    boot.Add(1 * time.Second),
			UserSmssStart:                boot.Add(5 * time.Second),
			WinlogonStart:                boot.Add(3 * time.Second),
			WinlogonInit:                 boot.Add(4 * time.Second),
			LSMStart:                     boot.Add(8 * time.Second),
			LSMReady:                     boot.Add(10 * time.Second),
			MachineGPStart:               boot.Add(12 * time.Second),
			MachineGPEnd:                 boot.Add(20 * time.Second),
			UserGPStart:                  boot.Add(32 * time.Second),
			UserGPEnd:                    boot.Add(38 * time.Second),
			UserWinlogonStart:            boot.Add(25 * time.Second),
			LogonStart:                   boot.Add(30 * time.Second),
			ProfileLoadStart:             boot.Add(31 * time.Second),
			ProfileCreationStart:         boot.Add(33 * time.Second),
			ExecuteShellCommandListStart: boot.Add(40 * time.Second),
			UserinitStart:                boot.Add(42 * time.Second),
			ExplorerStart:                boot.Add(50 * time.Second),
			DesktopReady:                 boot.Add(60 * time.Second),
		}

		milestones := buildTimelineMilestones(tl)

		assert.Len(t, milestones, 16)
		assert.Equal(t, "Boot Start", milestones[0].Name)
		assert.Equal(t, "Desktop Ready", milestones[15].Name)
	})
}

func TestBuildCustomPayload(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	t.Run("includes total boot duration", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:    boot,
			DesktopReady: boot.Add(90 * time.Second),
		}

		custom := buildCustomPayload(tl)

		durations, ok := custom["durations"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, int64(90000), durations["Total Boot Duration (ms)"])
	})

	t.Run("includes total logon duration", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:    boot,
			LogonStart:   boot.Add(30 * time.Second),
			LogonStop:    boot.Add(90 * time.Second),
			DesktopReady: boot.Add(90 * time.Second),
		}

		custom := buildCustomPayload(tl)

		durations := custom["durations"].(map[string]interface{})
		assert.Equal(t, int64(60000), durations["Total Logon Duration (ms)"])
	})

	t.Run("includes profile load duration", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:        boot,
			ProfileLoadStart: boot.Add(30 * time.Second),
			ProfileLoadEnd:   boot.Add(35 * time.Second),
			DesktopReady:     boot.Add(90 * time.Second),
		}

		custom := buildCustomPayload(tl)

		durations := custom["durations"].(map[string]interface{})
		assert.Equal(t, int64(5000), durations["Profile Load Duration (ms)"])
	})

	t.Run("includes group policy durations", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:      boot,
			MachineGPStart: boot.Add(10 * time.Second),
			MachineGPEnd:   boot.Add(18 * time.Second),
			UserGPStart:    boot.Add(30 * time.Second),
			UserGPEnd:      boot.Add(33 * time.Second),
			DesktopReady:   boot.Add(90 * time.Second),
		}

		custom := buildCustomPayload(tl)

		durations := custom["durations"].(map[string]interface{})
		assert.Equal(t, int64(8000), durations["Machine GP Duration (ms)"])
		assert.Equal(t, int64(3000), durations["User GP Duration (ms)"])
	})

	t.Run("omits durations when end timestamp is zero", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:        boot,
			ProfileLoadStart: boot.Add(30 * time.Second),
			// ProfileLoadEnd is zero
		}

		custom := buildCustomPayload(tl)

		if durations, ok := custom["durations"].(map[string]interface{}); ok {
			_, hasProfile := durations["Profile Load Duration (ms)"]
			assert.False(t, hasProfile)
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

	t.Run("includes theme loading duration", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:        boot,
			ThemesLogonStart: boot.Add(50 * time.Second),
			ThemesLogonEnd:   boot.Add(55 * time.Second),
			DesktopReady:     boot.Add(90 * time.Second),
		}

		custom := buildCustomPayload(tl)

		durations := custom["durations"].(map[string]interface{})
		assert.Equal(t, int64(5000), durations["Theme Loading Duration (ms)"])
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
			BootStart:    boot,
			LogonStart:   boot.Add(30 * time.Second),
			DesktopReady: boot.Add(90 * time.Second),
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
			BootStart:    boot,
			DesktopReady: boot.Add(90 * time.Second),
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
	assert.Equal(t, "Windows logon took 90000 ms", attrs["message"])
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
	assert.Equal(t, "Windows logon duration analysis after reboot", attrs["message"])
}
