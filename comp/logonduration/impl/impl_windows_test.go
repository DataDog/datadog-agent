// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows && test

package logondurationimpl

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	logscompressionmock "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
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
		LogonStart:                   boot.Add(30 * time.Second),
		LogonStop:                    boot.Add(35 * time.Second),
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
			SmssStart:         boot.Add(1 * time.Second),
			DesktopReadyStart: boot.Add(90 * time.Second),
		}

		milestones := buildTimelineMilestones(tl)

		require.Len(t, milestones, 2)
		assert.InDelta(t, 0.0, milestones[0].OffsetS, 0.001)
		assert.InDelta(t, 0.0, milestones[1].OffsetS, 0.001)
		assert.NotEmpty(t, milestones[0].Timestamp)
		assert.NotEmpty(t, milestones[1].Timestamp)
	})

	t.Run("full timeline includes all milestones in order", func(t *testing.T) {
		tl := fullBootTimeline(boot)

		milestones := buildTimelineMilestones(tl)

		require.Len(t, milestones, 20)

		expected := []struct {
			name    string
			offsetS float64
		}{
			{"Boot Start", 0},
			{"SMSS Start", 1},
			{"User Session SMSS Start", 5},
			{"Winlogon Start", 3},
			{"Winlogon Init", 4},
			{"Login UI Start", 8},
			{"Computer Group Policy", 12},
			{"User Group Policy", 32},
			{"User Session Winlogon Start", 25},
			{"User Logon", 30},
			{"Profile Loaded", 31},
			{"Profile Created", 33},
			{"Execute Shell Commands", 40},
			{"Userinit.exe", 42},
			{"Explorer.exe Start", 50},
			{"Explorer Initializing", 51},
			{"Desktop Created", 53},
			{"Desktop Visible", 55},
			{"Desktop Startup Apps", 58},
			{"Desktop Ready", 59},
		}
		for i, exp := range expected {
			assert.Equal(t, exp.name, milestones[i].Name, "milestone %d name", i)
			assert.InDelta(t, exp.offsetS, milestones[i].OffsetS, 0.001, "milestone %d offset", i)
		}
	})
}

func TestBuildCustomPayload(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	t.Run("includes total boot duration as sum of boot and logon", func(t *testing.T) {
		tl := BootTimeline{
			BootStart:           boot,
			LoginUIStart:        boot.Add(10 * time.Second),
			LogonStart:          boot.Add(30 * time.Second),
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
			LogonStart:          boot.Add(30 * time.Second),
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
			BootStart:  boot,
			LogonStart: boot.Add(30 * time.Second),
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
			LogonStart:          boot.Add(30 * time.Second),
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
			LogonStart:          boot.Add(30 * time.Second),
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
	assert.Equal(t, "Windows logon took 60000 ms", attrs["message"])
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

func TestSubmitMetrics_AllPhases(t *testing.T) {
	hostnameComp := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
	mockSender := mocksender.NewMockSender("test")

	comp := &logonDurationComponent{
		hostname: hostnameComp,
		sender:   mockSender,
	}

	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	result := &AnalysisResult{
		Timeline: fullBootTimeline(boot),
	}

	hostname := hostnameComp.GetSafe(context.TODO())

	// Total: boot (8000) + logon (25000) = 33000
	mockSender.On("Distribution", "eudm.boot_duration", float64(33000), hostname, []string{"phase:total"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(8000), hostname, []string{"phase:boot"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(25000), hostname, []string{"phase:logon"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(2000), hostname, []string{"phase:winlogon_init"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(2000), hostname, []string{"phase:login_ui"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(8000), hostname, []string{"phase:computer_group_policy"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(6000), hostname, []string{"phase:user_group_policy"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(5000), hostname, []string{"phase:user_logon"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(3000), hostname, []string{"phase:profile_load"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(3000), hostname, []string{"phase:profile_create"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(5000), hostname, []string{"phase:execute_shell_commands"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(8000), hostname, []string{"phase:userinit"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(3000), hostname, []string{"phase:explorer_initializing"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(3000), hostname, []string{"phase:desktop_created"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(2000), hostname, []string{"phase:desktop_visible"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(4000), hostname, []string{"phase:desktop_startup_apps"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(6000), hostname, []string{"phase:desktop_ready"}).Return()
	mockSender.On("Commit").Return()

	comp.submitMetrics(result)

	mockSender.AssertExpectations(t)
}

func TestSubmitMetrics_TotalRequiresAllFourTimestamps(t *testing.T) {
	hostnameComp := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())

	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	t.Run("missing LoginUIStart omits total", func(t *testing.T) {
		mockSender := mocksender.NewMockSender("test")
		comp := &logonDurationComponent{
			hostname: hostnameComp,
			sender:   mockSender,
		}

		result := &AnalysisResult{
			Timeline: BootTimeline{
				BootStart:           boot,
				LogonStart:          boot.Add(30 * time.Second),
				DesktopVisibleStart: boot.Add(55 * time.Second),
				DesktopVisibleEnd:   boot.Add(57 * time.Second),
			},
		}

		hostname := hostnameComp.GetSafe(context.TODO())

		mockSender.On("Distribution", "eudm.boot_duration", float64(25000), hostname, []string{"phase:logon"}).Return()
		mockSender.On("Distribution", "eudm.boot_duration", float64(2000), hostname, []string{"phase:desktop_visible"}).Return()
		mockSender.On("Commit").Return()

		comp.submitMetrics(result)

		mockSender.AssertExpectations(t)
		mockSender.AssertNotCalled(t, "Distribution", "eudm.boot_duration", mock.Anything, hostname, []string{"phase:total"})
	})

	t.Run("missing DesktopVisibleStart omits total", func(t *testing.T) {
		mockSender := mocksender.NewMockSender("test")
		comp := &logonDurationComponent{
			hostname: hostnameComp,
			sender:   mockSender,
		}

		result := &AnalysisResult{
			Timeline: BootTimeline{
				BootStart:    boot,
				LoginUIStart: boot.Add(8 * time.Second),
				LoginUIDone:  boot.Add(10 * time.Second),
				LogonStart:   boot.Add(30 * time.Second),
				LogonStop:    boot.Add(35 * time.Second),
			},
		}

		hostname := hostnameComp.GetSafe(context.TODO())

		mockSender.On("Distribution", "eudm.boot_duration", float64(8000), hostname, []string{"phase:boot"}).Return()
		mockSender.On("Distribution", "eudm.boot_duration", float64(2000), hostname, []string{"phase:login_ui"}).Return()
		mockSender.On("Distribution", "eudm.boot_duration", float64(5000), hostname, []string{"phase:user_logon"}).Return()
		mockSender.On("Commit").Return()

		comp.submitMetrics(result)

		mockSender.AssertExpectations(t)
		mockSender.AssertNotCalled(t, "Distribution", "eudm.boot_duration", mock.Anything, hostname, []string{"phase:total"})
	})

	t.Run("missing LogonStart omits total", func(t *testing.T) {
		mockSender := mocksender.NewMockSender("test")
		comp := &logonDurationComponent{
			hostname: hostnameComp,
			sender:   mockSender,
		}

		result := &AnalysisResult{
			Timeline: BootTimeline{
				BootStart:           boot,
				LoginUIStart:        boot.Add(8 * time.Second),
				LoginUIDone:         boot.Add(10 * time.Second),
				DesktopVisibleStart: boot.Add(55 * time.Second),
				DesktopVisibleEnd:   boot.Add(57 * time.Second),
			},
		}

		hostname := hostnameComp.GetSafe(context.TODO())

		mockSender.On("Distribution", "eudm.boot_duration", float64(8000), hostname, []string{"phase:boot"}).Return()
		mockSender.On("Distribution", "eudm.boot_duration", float64(2000), hostname, []string{"phase:login_ui"}).Return()
		mockSender.On("Distribution", "eudm.boot_duration", float64(2000), hostname, []string{"phase:desktop_visible"}).Return()
		mockSender.On("Commit").Return()

		comp.submitMetrics(result)

		mockSender.AssertExpectations(t)
		mockSender.AssertNotCalled(t, "Distribution", "eudm.boot_duration", mock.Anything, hostname, []string{"phase:total"})
	})

	t.Run("missing BootStart omits total", func(t *testing.T) {
		mockSender := mocksender.NewMockSender("test")
		comp := &logonDurationComponent{
			hostname: hostnameComp,
			sender:   mockSender,
		}

		result := &AnalysisResult{
			Timeline: BootTimeline{
				LoginUIStart:        boot.Add(8 * time.Second),
				LoginUIDone:         boot.Add(10 * time.Second),
				LogonStart:          boot.Add(30 * time.Second),
				DesktopVisibleStart: boot.Add(55 * time.Second),
				DesktopVisibleEnd:   boot.Add(57 * time.Second),
			},
		}

		hostname := hostnameComp.GetSafe(context.TODO())

		mockSender.On("Distribution", "eudm.boot_duration", float64(25000), hostname, []string{"phase:logon"}).Return()
		mockSender.On("Distribution", "eudm.boot_duration", float64(2000), hostname, []string{"phase:login_ui"}).Return()
		mockSender.On("Distribution", "eudm.boot_duration", float64(2000), hostname, []string{"phase:desktop_visible"}).Return()
		mockSender.On("Commit").Return()

		comp.submitMetrics(result)

		mockSender.AssertExpectations(t)
		mockSender.AssertNotCalled(t, "Distribution", "eudm.boot_duration", mock.Anything, hostname, []string{"phase:total"})
	})
}

func TestSubmitMetrics_SkipsPhaseWithZeroStartOrEnd(t *testing.T) {
	hostnameComp := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
	mockSender := mocksender.NewMockSender("test")

	comp := &logonDurationComponent{
		hostname: hostnameComp,
		sender:   mockSender,
	}

	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	result := &AnalysisResult{
		Timeline: BootTimeline{
			BootStart:    boot,
			LoginUIStart: boot.Add(8 * time.Second),
			WinlogonInit: boot.Add(4 * time.Second),
			// WinlogonInitDone is zero → winlogon_init phase skipped
			LogonStart:          boot.Add(30 * time.Second),
			DesktopVisibleStart: boot.Add(55 * time.Second),
			// MachineGPStart set but MachineGPEnd zero → computer_group_policy skipped
			MachineGPStart: boot.Add(12 * time.Second),
		},
	}

	hostname := hostnameComp.GetSafe(context.TODO())

	mockSender.On("Distribution", "eudm.boot_duration", float64(33000), hostname, []string{"phase:total"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(8000), hostname, []string{"phase:boot"}).Return()
	mockSender.On("Distribution", "eudm.boot_duration", float64(25000), hostname, []string{"phase:logon"}).Return()
	mockSender.On("Commit").Return()

	comp.submitMetrics(result)

	mockSender.AssertExpectations(t)
	mockSender.AssertNotCalled(t, "Distribution", "eudm.boot_duration", mock.Anything, hostname, []string{"phase:winlogon_init"})
	mockSender.AssertNotCalled(t, "Distribution", "eudm.boot_duration", mock.Anything, hostname, []string{"phase:computer_group_policy"})
}

func TestSubmitMetrics_AllZeroDurations(t *testing.T) {
	hostnameComp := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
	mockSender := mocksender.NewMockSender("test")

	comp := &logonDurationComponent{
		hostname: hostnameComp,
		sender:   mockSender,
	}

	result := &AnalysisResult{
		Timeline: BootTimeline{},
	}

	mockSender.On("Commit").Return()

	comp.submitMetrics(result)

	mockSender.AssertExpectations(t)
	mockSender.AssertNotCalled(t, "Distribution", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestSubmitMetrics_NilSender(t *testing.T) {
	hostnameComp := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())

	comp := &logonDurationComponent{
		hostname: hostnameComp,
		sender:   nil,
	}

	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	result := &AnalysisResult{
		Timeline: BootTimeline{
			BootStart:           boot,
			LoginUIStart:        boot.Add(8 * time.Second),
			LogonStart:          boot.Add(30 * time.Second),
			DesktopVisibleStart: boot.Add(55 * time.Second),
		},
	}

	// Should not panic with nil sender
	comp.submitMetrics(result)
}
