// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin && test

package logondurationimpl

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	sysprobeconfigmock "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/impl"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	logscompressionmock "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/logonduration"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

func TestBuildTimelineMilestones(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	t.Run("returns three milestones", func(t *testing.T) {
		ts := logonduration.LoginTimestamps{
			LoginWindowTime:  boot.Add(10 * time.Second),
			LoginTime:        boot.Add(30 * time.Second),
			DesktopReadyTime: boot.Add(90 * time.Second),
		}

		milestones := buildTimelineMilestones(boot, ts)

		require.Len(t, milestones, 3)
		assert.Equal(t, "Boot Duration", milestones[0].Name)
		assert.Equal(t, "Login Window Ready", milestones[1].Name)
		assert.Equal(t, "Logon Duration", milestones[2].Name)
	})

	t.Run("computes correct offsets from boot start", func(t *testing.T) {
		ts := logonduration.LoginTimestamps{
			LoginWindowTime:  boot.Add(10 * time.Second),
			LoginTime:        boot.Add(30 * time.Second),
			DesktopReadyTime: boot.Add(90 * time.Second),
		}

		milestones := buildTimelineMilestones(boot, ts)

		// The 20s idle gap (LoginWindowTime -> LoginTime) is collapsed out of the
		// post-login offsets, so logon_duration starts where the
		// login window appeared.
		assert.InDelta(t, 0.0, milestones[0].OffsetMs, 0.001)
		assert.InDelta(t, 10000.0, milestones[1].OffsetMs, 0.001)
		assert.InDelta(t, 10000.0, milestones[2].OffsetMs, 0.001)
	})

	t.Run("computes correct durations between milestones", func(t *testing.T) {
		ts := logonduration.LoginTimestamps{
			LoginWindowTime:  boot.Add(10 * time.Second),
			LoginTime:        boot.Add(30 * time.Second),
			DesktopReadyTime: boot.Add(90 * time.Second),
		}

		milestones := buildTimelineMilestones(boot, ts)

		assert.InDelta(t, 10000.0, milestones[0].DurationMs, 0.001)
		assert.InDelta(t, 0.0, milestones[1].DurationMs, 0.001)
		assert.InDelta(t, 60000.0, milestones[2].DurationMs, 0.001)
	})

	t.Run("formats timestamps correctly", func(t *testing.T) {
		ts := logonduration.LoginTimestamps{
			LoginWindowTime:  boot.Add(10 * time.Second),
			LoginTime:        boot.Add(30 * time.Second),
			DesktopReadyTime: boot.Add(90 * time.Second),
		}

		milestones := buildTimelineMilestones(boot, ts)

		assert.Equal(t, "2026-01-15T08:00:00.000Z", milestones[0].Timestamp)
		assert.Equal(t, "2026-01-15T08:00:10.000Z", milestones[1].Timestamp)
		assert.Equal(t, "2026-01-15T08:00:30.000Z", milestones[2].Timestamp)
	})

	t.Run("handles millisecond precision", func(t *testing.T) {
		ts := logonduration.LoginTimestamps{
			LoginWindowTime:  boot.Add(10*time.Second + 500*time.Millisecond),
			LoginTime:        boot.Add(30*time.Second + 250*time.Millisecond),
			DesktopReadyTime: boot.Add(90*time.Second + 750*time.Millisecond),
		}

		milestones := buildTimelineMilestones(boot, ts)

		// gap = 30.25s - 10.5s = 19.75s; collapsed offsets all land at 10500ms.
		assert.InDelta(t, 10500.0, milestones[1].OffsetMs, 0.001)
		assert.InDelta(t, 10500.0, milestones[2].OffsetMs, 0.001)
	})

	t.Run("zero LoginWindowTime omits the login_window_ready milestone", func(t *testing.T) {
		ts := logonduration.LoginTimestamps{
			LoginTime:        boot.Add(30 * time.Second),
			DesktopReadyTime: boot.Add(90 * time.Second),
		}

		milestones := buildTimelineMilestones(boot, ts)

		// Milestones with a zero timestamp are skipped: login_window_ready drops out.
		require.Len(t, milestones, 2)
		assert.Equal(t, "boot_duration", milestones[0].ID)
		assert.Equal(t, "logon_duration", milestones[1].ID)

		// Boot Duration depends on LoginWindowTime, so its duration is 0.
		assert.InDelta(t, 0.0, milestones[0].DurationMs, 0.001)
		// No gap can be computed without LoginWindowTime, so logon_duration keeps its
		// wall-clock offset.
		assert.InDelta(t, 30000.0, milestones[1].OffsetMs, 0.001)
		assert.InDelta(t, 60000.0, milestones[1].DurationMs, 0.001)
	})

	t.Run("zero LoginTime omits the logon_duration milestone", func(t *testing.T) {
		ts := logonduration.LoginTimestamps{
			LoginWindowTime:  boot.Add(10 * time.Second),
			DesktopReadyTime: boot.Add(90 * time.Second),
		}

		milestones := buildTimelineMilestones(boot, ts)

		require.Len(t, milestones, 2)
		assert.Equal(t, "boot_duration", milestones[0].ID)
		assert.Equal(t, "login_window_ready", milestones[1].ID)
		assert.InDelta(t, 10000.0, milestones[0].DurationMs, 0.001)
		assert.InDelta(t, 10000.0, milestones[1].OffsetMs, 0.001)
		assert.InDelta(t, 0.0, milestones[1].DurationMs, 0.001)
	})

	t.Run("zero DesktopReadyTime yields 0 logon durations", func(t *testing.T) {
		ts := logonduration.LoginTimestamps{
			LoginWindowTime: boot.Add(10 * time.Second),
			LoginTime:       boot.Add(30 * time.Second),
		}

		milestones := buildTimelineMilestones(boot, ts)

		require.Len(t, milestones, 3)
		// logon_duration's duration depends on DesktopReadyTime.
		assert.Equal(t, "logon_duration", milestones[2].ID)
		assert.InDelta(t, 0.0, milestones[2].DurationMs, 0.001)
		// The idle gap (LoginWindowTime -> LoginTime) is still collapsed.
		assert.InDelta(t, 10000.0, milestones[2].OffsetMs, 0.001)
	})
}

func TestBuildCustomPayload(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)

	t.Run("includes boot duration", func(t *testing.T) {
		ts := logonduration.LoginTimestamps{
			LoginWindowTime:  boot.Add(10 * time.Second),
			LoginTime:        boot.Add(30 * time.Second),
			DesktopReadyTime: boot.Add(90 * time.Second),
		}

		custom := buildCustomPayload(boot, ts)

		durations, ok := custom["durations"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, int64(10000), durations["boot_duration_ms"])
	})

	t.Run("includes logon duration", func(t *testing.T) {
		ts := logonduration.LoginTimestamps{
			LoginWindowTime:  boot.Add(10 * time.Second),
			LoginTime:        boot.Add(30 * time.Second),
			DesktopReadyTime: boot.Add(90 * time.Second),
		}

		custom := buildCustomPayload(boot, ts)

		durations, ok := custom["durations"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, int64(60000), durations["logon_duration_ms"])
	})

	t.Run("includes total boot duration as sum of boot and logon", func(t *testing.T) {
		ts := logonduration.LoginTimestamps{
			LoginWindowTime:  boot.Add(10 * time.Second),
			LoginTime:        boot.Add(30 * time.Second),
			DesktopReadyTime: boot.Add(90 * time.Second),
		}

		custom := buildCustomPayload(boot, ts)

		durations, ok := custom["durations"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, int64(70000), durations["total_boot_duration_ms"])
	})

	t.Run("includes filevault status when true", func(t *testing.T) {
		ts := logonduration.LoginTimestamps{
			LoginWindowTime:  boot.Add(10 * time.Second),
			LoginTime:        boot.Add(30 * time.Second),
			DesktopReadyTime: boot.Add(90 * time.Second),
			FileVaultEnabled: true,
		}

		custom := buildCustomPayload(boot, ts)

		assert.Equal(t, true, custom["filevault_enabled"])
	})

	t.Run("includes filevault status when false", func(t *testing.T) {
		ts := logonduration.LoginTimestamps{
			LoginWindowTime:  boot.Add(10 * time.Second),
			LoginTime:        boot.Add(30 * time.Second),
			DesktopReadyTime: boot.Add(90 * time.Second),
			FileVaultEnabled: false,
		}

		custom := buildCustomPayload(boot, ts)

		assert.Equal(t, false, custom["filevault_enabled"])
	})

	t.Run("includes boot_timeline key", func(t *testing.T) {
		ts := logonduration.LoginTimestamps{
			LoginWindowTime:  boot.Add(10 * time.Second),
			LoginTime:        boot.Add(30 * time.Second),
			DesktopReadyTime: boot.Add(90 * time.Second),
		}

		custom := buildCustomPayload(boot, ts)

		timeline, ok := custom["boot_timeline"].([]Milestone)
		require.True(t, ok)
		assert.Len(t, timeline, 3)
	})

	t.Run("zero LoginWindowTime yields 0 boot_duration_ms", func(t *testing.T) {
		ts := logonduration.LoginTimestamps{
			LoginTime:        boot.Add(30 * time.Second),
			DesktopReadyTime: boot.Add(90 * time.Second),
		}

		custom := buildCustomPayload(boot, ts)

		durations, ok := custom["durations"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, int64(0), durations["boot_duration_ms"])
	})

	t.Run("zero LoginTime or DesktopReadyTime yields 0 logon_duration_ms", func(t *testing.T) {
		ts := logonduration.LoginTimestamps{
			LoginWindowTime:  boot.Add(10 * time.Second),
			DesktopReadyTime: boot.Add(90 * time.Second),
		}

		custom := buildCustomPayload(boot, ts)

		durations, ok := custom["durations"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, int64(0), durations["logon_duration_ms"])
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
	ts := logonduration.LoginTimestamps{
		LoginWindowTime:  boot.Add(10 * time.Second),
		LoginTime:        boot.Add(30 * time.Second),
		DesktopReadyTime: boot.Add(90 * time.Second),
	}

	err := comp.submitEvent(boot, ts)
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
	assert.Equal(t, "Device booted up: Boot & login took 70000 ms", attrs["title"])
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

func TestSubmitEvent_MessageIncludesLogonDuration(t *testing.T) {
	hostname := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
	compression := fxutil.Test[logscompression.Component](t, logscompressionmock.MockModule())
	forwarder := eventplatformimpl.NewNoopEventPlatformForwarder(hostname, compression)

	comp := &logonDurationComponent{
		hostname:               hostname,
		eventPlatformForwarder: forwarder,
	}

	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	ts := logonduration.LoginTimestamps{
		LoginWindowTime:  boot.Add(10 * time.Second),
		LoginTime:        boot.Add(30 * time.Second),
		DesktopReadyTime: boot.Add(90 * time.Second),
	}

	err := comp.submitEvent(boot, ts)
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
		ts       logonduration.LoginTimestamps
		expected string
	}{
		{
			name: "complete",
			ts: logonduration.LoginTimestamps{
				LoginWindowTime:  boot.Add(10 * time.Second),
				LoginTime:        boot.Add(30 * time.Second),
				DesktopReadyTime: boot.Add(90 * time.Second),
			},
			expected: "Device booted up: Boot & login took 70000 ms",
		},
		{
			name:     "boot only",
			ts:       logonduration.LoginTimestamps{LoginWindowTime: boot.Add(10 * time.Second)},
			expected: "Device booted up: Boot timeline incomplete",
		},
		{
			name: "logon only",
			ts: logonduration.LoginTimestamps{
				LoginTime:        boot.Add(30 * time.Second),
				DesktopReadyTime: boot.Add(90 * time.Second),
			},
			expected: "Device booted up: Boot timeline incomplete",
		},
		{
			name:     "neither",
			ts:       logonduration.LoginTimestamps{},
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

			err := comp.submitEvent(boot, tc.ts)
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

func TestSubmitEvent_IncludesSystemNotableEventsMetadata(t *testing.T) {
	hostname := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
	compression := fxutil.Test[logscompression.Component](t, logscompressionmock.MockModule())
	forwarder := eventplatformimpl.NewNoopEventPlatformForwarder(hostname, compression)

	comp := &logonDurationComponent{
		hostname:               hostname,
		eventPlatformForwarder: forwarder,
	}

	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	ts := logonduration.LoginTimestamps{
		LoginWindowTime:  boot.Add(10 * time.Second),
		LoginTime:        boot.Add(30 * time.Second),
		DesktopReadyTime: boot.Add(90 * time.Second),
	}

	err := comp.submitEvent(boot, ts)
	require.NoError(t, err)

	sent := forwarder.Purge()
	msgs := sent[eventplatform.EventTypeEventManagement]
	require.Len(t, msgs, 1)

	var payload map[string]interface{}
	err = json.Unmarshal(msgs[0].GetContent(), &payload)
	require.NoError(t, err)

	data := payload["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})

	sne, ok := attrs["system-notable-events"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "logon_duration", sne["event_type"])
}

func TestSubmitEvent_TimestampFormat(t *testing.T) {
	hostname := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
	compression := fxutil.Test[logscompression.Component](t, logscompressionmock.MockModule())
	forwarder := eventplatformimpl.NewNoopEventPlatformForwarder(hostname, compression)

	comp := &logonDurationComponent{
		hostname:               hostname,
		eventPlatformForwarder: forwarder,
	}

	boot := time.Date(2026, 1, 15, 8, 30, 45, 123456789, time.UTC)
	ts := logonduration.LoginTimestamps{
		LoginWindowTime:  boot.Add(10 * time.Second),
		LoginTime:        boot.Add(30 * time.Second),
		DesktopReadyTime: boot.Add(90 * time.Second),
	}

	err := comp.submitEvent(boot, ts)
	require.NoError(t, err)

	sent := forwarder.Purge()
	msgs := sent[eventplatform.EventTypeEventManagement]
	require.Len(t, msgs, 1)

	var payload map[string]interface{}
	err = json.Unmarshal(msgs[0].GetContent(), &payload)
	require.NoError(t, err)

	data := payload["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	assert.Equal(t, "2026-01-15T08:30:45.123456Z", attrs["timestamp"])
}

func TestSubmitEvent_CustomPayloadIncludesFileVault(t *testing.T) {
	hostname := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
	compression := fxutil.Test[logscompression.Component](t, logscompressionmock.MockModule())
	forwarder := eventplatformimpl.NewNoopEventPlatformForwarder(hostname, compression)

	comp := &logonDurationComponent{
		hostname:               hostname,
		eventPlatformForwarder: forwarder,
	}

	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	ts := logonduration.LoginTimestamps{
		LoginWindowTime:  boot.Add(10 * time.Second),
		LoginTime:        boot.Add(30 * time.Second),
		DesktopReadyTime: boot.Add(90 * time.Second),
		FileVaultEnabled: true,
	}

	err := comp.submitEvent(boot, ts)
	require.NoError(t, err)

	sent := forwarder.Purge()
	msgs := sent[eventplatform.EventTypeEventManagement]
	require.Len(t, msgs, 1)

	var payload map[string]interface{}
	err = json.Unmarshal(msgs[0].GetContent(), &payload)
	require.NoError(t, err)

	data := payload["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	nestedAttrs := attrs["attributes"].(map[string]interface{})
	custom := nestedAttrs["custom"].(map[string]interface{})

	assert.Equal(t, true, custom["filevault_enabled"])
}

func TestSubmitEvent_DurationsInPayload(t *testing.T) {
	hostname := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
	compression := fxutil.Test[logscompression.Component](t, logscompressionmock.MockModule())
	forwarder := eventplatformimpl.NewNoopEventPlatformForwarder(hostname, compression)

	comp := &logonDurationComponent{
		hostname:               hostname,
		eventPlatformForwarder: forwarder,
	}

	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	ts := logonduration.LoginTimestamps{
		LoginWindowTime:  boot.Add(15 * time.Second),
		LoginTime:        boot.Add(45 * time.Second),
		DesktopReadyTime: boot.Add(120 * time.Second),
	}

	err := comp.submitEvent(boot, ts)
	require.NoError(t, err)

	sent := forwarder.Purge()
	msgs := sent[eventplatform.EventTypeEventManagement]
	require.Len(t, msgs, 1)

	var payload map[string]interface{}
	err = json.Unmarshal(msgs[0].GetContent(), &payload)
	require.NoError(t, err)

	data := payload["data"].(map[string]interface{})
	attrs := data["attributes"].(map[string]interface{})
	nestedAttrs := attrs["attributes"].(map[string]interface{})
	custom := nestedAttrs["custom"].(map[string]interface{})
	durations := custom["durations"].(map[string]interface{})

	assert.Equal(t, float64(15000), durations["boot_duration_ms"])
	assert.Equal(t, float64(75000), durations["logon_duration_ms"])
	assert.Equal(t, float64(90000), durations["total_boot_duration_ms"])
}

// testFixture holds all test dependencies for component integration tests
type testFixture struct {
	t              *testing.T
	sysProbeClient *mockSysProbeClient
	forwarder      eventplatform.Forwarder
	reqs           Requires
}

// newFixture creates a new test fixture with mock dependencies
func newFixture(t *testing.T, enabled bool) *testFixture {
	logComp := logmock.New(t)

	configComp := config.NewMock(t)
	configComp.SetInTest("logon_duration.enabled", enabled)

	sysprobeConfigComp := sysprobeconfigmock.NewMock(t)

	hostnameComp := fxutil.Test[hostnameinterface.Component](t, hostnameimpl.MockModule())
	compressionComp := fxutil.Test[logscompression.Component](t, logscompressionmock.MockModule())
	forwarder := eventplatformimpl.NewNoopEventPlatformForwarder(hostnameComp, compressionComp)
	eventPlatformComp := option.NewPtr(forwarder)

	sp := &mockSysProbeClient{}

	return &testFixture{
		t:              t,
		sysProbeClient: sp,
		forwarder:      forwarder,
		reqs: Requires{
			Log:            logComp,
			Config:         configComp,
			SysprobeConfig: sysprobeConfigComp,
			Hostname:       hostnameComp,
			Lc:             compdef.NewTestLifecycle(t),
			EventPlatform:  eventPlatformComp,
		},
	}
}

// componentTestHelper wraps the component with test-specific wait helpers
type componentTestHelper struct {
	*logonDurationComponent
	fixture *testFixture
}

// WaitForSysProbeCall waits for the GetLoginTimestamps method to be called
func (h *componentTestHelper) WaitForSysProbeCall() *componentTestHelper {
	require.Eventually(h.fixture.t, func() bool {
		return h.fixture.sysProbeClient.GetCallCount() > 0
	}, time.Second, 10*time.Millisecond, "Expected GetLoginTimestamps to be called")
	return h
}

// sut returns the system under test with the mock client
func (tf *testFixture) sut() *componentTestHelper {
	provides := newWithClient(tf.reqs, tf.sysProbeClient)
	comp := provides.Comp.(*logonDurationComponent)

	return &componentTestHelper{
		logonDurationComponent: comp,
		fixture:                tf,
	}
}

func TestNewComponent_DisabledByConfig(t *testing.T) {
	f := newFixture(t, false)

	provides := newWithClient(f.reqs, f.sysProbeClient)

	assert.NotNil(t, provides.Comp)
	comp := provides.Comp.(*logonDurationComponent)
	assert.Nil(t, comp.eventPlatformForwarder)
	assert.Nil(t, comp.sysProbeClient)
}

func TestNewComponent_EnabledByConfig(t *testing.T) {
	f := newFixture(t, true)

	provides := newWithClient(f.reqs, f.sysProbeClient)

	assert.NotNil(t, provides.Comp)
	comp := provides.Comp.(*logonDurationComponent)
	assert.NotNil(t, comp.eventPlatformForwarder)
	assert.NotNil(t, comp.sysProbeClient)
}

func TestSysProbeClient_ReturnsTimestamps(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	expectedTs := logonduration.LoginTimestamps{
		LoginWindowTime:  boot.Add(10 * time.Second),
		LoginTime:        boot.Add(30 * time.Second),
		DesktopReadyTime: boot.Add(90 * time.Second),
		FileVaultEnabled: true,
	}

	mockClient := &mockSysProbeClient{}
	mockClient.On("GetLoginTimestamps", mock.Anything).Return(expectedTs, nil)

	ts, err := mockClient.GetLoginTimestamps(context.Background())

	require.NoError(t, err)
	assert.Equal(t, expectedTs.LoginWindowTime, ts.LoginWindowTime)
	assert.Equal(t, expectedTs.LoginTime, ts.LoginTime)
	assert.Equal(t, expectedTs.DesktopReadyTime, ts.DesktopReadyTime)
	assert.Equal(t, expectedTs.FileVaultEnabled, ts.FileVaultEnabled)
	mockClient.AssertExpectations(t)
}

func TestSysProbeClient_ReturnsError(t *testing.T) {
	expectedErr := errors.New("system-probe connection failed")

	mockClient := &mockSysProbeClient{}
	mockClient.On("GetLoginTimestamps", mock.Anything).Return(logonduration.LoginTimestamps{}, expectedErr)

	ts, err := mockClient.GetLoginTimestamps(context.Background())

	require.Error(t, err)
	assert.Equal(t, expectedErr, err)
	assert.Equal(t, logonduration.LoginTimestamps{}, ts)
	mockClient.AssertExpectations(t)
}

func TestSysProbeClient_ThreadSafety(t *testing.T) {
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	expectedTs := logonduration.LoginTimestamps{
		LoginWindowTime:  boot.Add(10 * time.Second),
		LoginTime:        boot.Add(30 * time.Second),
		DesktopReadyTime: boot.Add(90 * time.Second),
	}

	mockClient := &mockSysProbeClient{}
	mockClient.On("GetLoginTimestamps", mock.Anything).Return(expectedTs, nil)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = mockClient.GetLoginTimestamps(context.Background())
		}()
	}
	wg.Wait()

	assert.Equal(t, 10, mockClient.GetCallCount())
}

func TestComponentLifecycle_StartStop(t *testing.T) {
	f := newFixture(t, true)
	boot := time.Date(2026, 1, 15, 8, 0, 0, 0, time.UTC)
	ts := logonduration.LoginTimestamps{
		LoginWindowTime:  boot.Add(10 * time.Second),
		LoginTime:        boot.Add(30 * time.Second),
		DesktopReadyTime: boot.Add(90 * time.Second),
	}
	f.sysProbeClient.On("GetLoginTimestamps", mock.Anything).Return(ts, nil)

	helper := f.sut()

	err := helper.start()
	require.NoError(t, err)
	assert.NotNil(t, helper.ctxCancel)

	err = helper.stop()
	require.NoError(t, err)
}

func TestComponentLifecycle_StopWithoutStart(t *testing.T) {
	f := newFixture(t, true)
	f.sysProbeClient.On("GetLoginTimestamps", mock.Anything).Return(logonduration.LoginTimestamps{}, nil)

	helper := f.sut()

	err := helper.stop()
	require.NoError(t, err)
}
