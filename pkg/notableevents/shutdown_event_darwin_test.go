// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableevents

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	notableeventtypes "github.com/DataDog/datadog-agent/pkg/notableevents/types"
)

const testBootUUID = "4D0D452B-C974-480E-AF64-0F35ACD2A43E"

func TestShutdownCauseEventContract(t *testing.T) {
	bootTime := time.Date(2026, time.August, 11, 9, 55, 21, 0, time.UTC)
	result, emit := classifyShutdownTokens(pmuBootFaultInfo{Tokens: []string{
		"ot,tdie_overtemp", "ot,tsns_overtemp", "sochot,reset_in_3",
	}})
	require.True(t, emit)

	identity := shutdownCauseIdentity(testBootUUID, result.FaultTokens)
	event := result.event(identity, testBootUUID, bootTime, bootTime)

	require.NoError(t, notableeventtypes.ValidateEvent(event))
	assert.True(t, strings.HasPrefix(event.ID, "macos-shutdown-v1:"))
	assert.Equal(t, "System shutdown fault", event.EventType)
	assert.Equal(t, "macOS overheated shutdown", event.Title)
	assert.Equal(t, "The previous shutdown was caused by a thermal fault", event.Message)
	assert.Equal(t, bootTime, event.Timestamp)

	payload, ok := event.Custom["macos_shutdown_cause"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "IOPMUBootFaultInfo", payload["source"])
	assert.Equal(t, "thermal", payload["classification"])
	assert.Equal(t, "ot", payload["primary_family"])
	assert.Equal(t, []interface{}{"ot", "sochot"}, payload["families"])
	assert.Equal(t, []interface{}{
		"ot,tdie_overtemp", "ot,tsns_overtemp", "sochot,reset_in_3",
	}, payload["tokens"])
	assert.Equal(t, payload["tokens"], payload["fault_tokens"])
	assert.Equal(t, testBootUUID, payload["boot_uuid"])
	assert.Equal(t, "2026-08-11T09:55:21Z", payload["boot_time"])
}

// TestShutdownCauseEventOmitsUnknownBootTime covers the kern.boottime failure
// path: the caller supplies the timestamp and boot_time is left out rather than
// reported as a zero value.
func TestShutdownCauseEventOmitsUnknownBootTime(t *testing.T) {
	fallback := time.Date(2026, time.August, 13, 8, 0, 0, 0, time.UTC)
	result, emit := classifyShutdownTokens(pmuBootFaultInfo{Tokens: []string{"ntc_shdn"}})
	require.True(t, emit)

	event := result.event(shutdownCauseIdentity(testBootUUID, result.FaultTokens), testBootUUID, time.Time{}, fallback)

	require.NoError(t, notableeventtypes.ValidateEvent(event))
	assert.Equal(t, fallback, event.Timestamp)
	payload := event.Custom["macos_shutdown_cause"].(map[string]interface{})
	assert.NotContains(t, payload, "boot_time")
	assert.Equal(t, testBootUUID, payload["boot_uuid"])
}

// TestShutdownCauseEventEveryClassification keeps every classification inside
// the wire contract, titles and messages included.
func TestShutdownCauseEventEveryClassification(t *testing.T) {
	tokens := map[shutdownClass]string{
		shutdownClassThermal:  "ot,overtemp",
		shutdownClassPower:    "uv,vddmain_uvlo",
		shutdownClassCrash:    "crash,crash_in",
		shutdownClassWatchdog: "timeout,watchdog_timeout",
		shutdownClassHardware: "spmi,spmi_fault",
	}
	require.Len(t, tokens, len(shutdownClassPrecedence))

	for _, class := range shutdownClassPrecedence {
		t.Run(string(class), func(t *testing.T) {
			result, emit := classifyShutdownTokens(pmuBootFaultInfo{Tokens: []string{tokens[class]}})
			require.True(t, emit)
			require.Equal(t, class, result.Class)

			event := result.event(
				shutdownCauseIdentity(testBootUUID, result.FaultTokens),
				testBootUUID,
				time.Now().UTC(),
				time.Now().UTC())

			require.NoError(t, notableeventtypes.ValidateEvent(event))
			assert.NotEmpty(t, event.Title)
			assert.NotEmpty(t, event.Message)
			assert.LessOrEqual(t, len(event.Title), notableeventtypes.MaxEventStringBytes)
			assert.LessOrEqual(t, len(event.Message), notableeventtypes.MaxEventStringBytes)
			assert.NotContains(t, strings.ToLower(event.Message), "panic")
		})
	}
}

// TestShutdownCauseEventFitsWireLimit uses the full 80-token dictionary, which
// is the largest payload the measured hardware can produce.
func TestShutdownCauseEventFitsWireLimit(t *testing.T) {
	result, emit := classifyShutdownTokens(pmuBootFaultInfo{Tokens: allPMUFaultTokens()})
	require.True(t, emit)
	require.Equal(t, shutdownClassThermal, result.Class)
	require.Len(t, result.Tokens, 80)
	require.Len(t, result.FaultTokens, 59)

	event := result.event(
		shutdownCauseIdentity(testBootUUID, result.FaultTokens),
		testBootUUID,
		time.Now().UTC(),
		time.Now().UTC())

	require.NoError(t, notableeventtypes.ValidateEvent(event))
	assert.True(t, eventFitsWireLimit(event))

	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	assert.Less(t, len(encoded), notableeventtypes.MaxEventWireSize)
}

func TestShutdownCauseIdentity(t *testing.T) {
	thermal := []string{"ot,overtemp"}
	power := []string{"uv,vddmain_uvlo"}
	otherBoot := "D27F2229-FD15-4D46-94A1-318553D26EF9"

	// Same boot, same cause: one identity, therefore one event.
	assert.Equal(t,
		shutdownCauseIdentity(testBootUUID, thermal),
		shutdownCauseIdentity(testBootUUID, thermal))

	// A recurring cause on a later boot must report again.
	assert.NotEqual(t,
		shutdownEventID(shutdownCauseIdentity(testBootUUID, thermal)),
		shutdownEventID(shutdownCauseIdentity(otherBoot, thermal)))

	// A different cause on the same boot is a different event.
	assert.NotEqual(t,
		shutdownEventID(shutdownCauseIdentity(testBootUUID, thermal)),
		shutdownEventID(shutdownCauseIdentity(testBootUUID, power)))

	// The identifier is a valid, distinct namespace.
	id := shutdownEventID(shutdownCauseIdentity(testBootUUID, thermal))
	assert.True(t, notableeventtypes.IsEventID(id))
	assert.NotEqual(t, eventID(shutdownCauseIdentity(testBootUUID, thermal)), id)
}

// TestSanitizedPayloadStringsBoundsInput guards the payload helper against a
// driver-published value that is hostile or simply too large.
func TestSanitizedPayloadStringsBoundsInput(t *testing.T) {
	assert.Empty(t, sanitizedPayloadStrings(nil))

	oversized := make([]string, notableeventtypes.MaxCustomItems+10)
	for index := range oversized {
		oversized[index] = "ot,overtemp"
	}
	assert.Len(t, sanitizedPayloadStrings(oversized), notableeventtypes.MaxCustomItems)

	assert.Equal(t, []interface{}{"ot,overtemp"}, sanitizedPayloadStrings([]string{"", "  ", "ot,overtemp"}))

	long := strings.Repeat("a", notableeventtypes.MaxEventStringBytes+10)
	bounded := sanitizedPayloadStrings([]string{long})
	require.Len(t, bounded, 1)
	assert.Len(t, bounded[0], notableeventtypes.MaxEventStringBytes)
}
