// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package notableevents

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventUnmarshalJSONPreservesCustomNumbers(t *testing.T) {
	const exactInteger = "9007199254740993"
	id := eventID("socket-exact-number")
	data := []byte(`{
		"id":"` + id + `",
		"timestamp":"2026-07-22T12:00:00Z",
		"event_type":"Application crash",
		"title":"Application crash: Test",
		"message":"An application crashed unexpectedly",
		"custom":{"value":` + exactInteger + `}
	}`)

	var event Event
	require.NoError(t, json.Unmarshal(data, &event))
	number, ok := event.Custom["value"].(json.Number)
	require.True(t, ok)
	assert.Equal(t, json.Number(exactInteger), number)

	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"value":`+exactInteger)
}

func TestEventUnmarshalJSONRejectsSemanticCorruption(t *testing.T) {
	valid := validPersistedDarwinEvent("socket-validation")
	tooDeep := map[string]interface{}{"leaf": "value"}
	for range maxDarwinCustomDepth + 1 {
		tooDeep = map[string]interface{}{"child": tooDeep}
	}

	tests := []struct {
		name string
		data func() []byte
	}{
		{
			name: "invalid id",
			data: func() []byte {
				event := valid
				event.ID = "event"
				return mustMarshalJSON(t, event)
			},
		},
		{
			name: "empty required field",
			data: func() []byte {
				event := valid
				event.Title = ""
				return mustMarshalJSON(t, event)
			},
		},
		{
			name: "oversized field",
			data: func() []byte {
				event := valid
				event.Message = strings.Repeat("x", maxDarwinEventStringBytes+1)
				return mustMarshalJSON(t, event)
			},
		},
		{
			name: "excessive custom nesting",
			data: func() []byte {
				event := valid
				event.Custom = tooDeep
				return mustMarshalJSON(t, event)
			},
		},
		{
			name: "invalid number",
			data: func() []byte {
				data := mustMarshalJSON(t, valid)
				return []byte(strings.Replace(string(data), `"scope":"system"`, `"scope":1e400`, 1))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var event Event
			err := json.Unmarshal(test.data(), &event)
			require.Error(t, err)
		})
	}
}

func TestEventUnmarshalJSONRejectsInvalidOrTrailingInput(t *testing.T) {
	for _, data := range []string{
		`{"custom":{"value":1`,
		`{"custom":{"value":1}} {"custom":{"value":2}}`,
	} {
		var event Event
		require.Error(t, json.Unmarshal([]byte(data), &event))
	}
}

func mustMarshalJSON(t *testing.T, value interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return data
}

// TestParseMacOSCrashReport verifies metadata and body objects are parsed together.
func TestParseMacOSCrashReport(t *testing.T) {
	report, err := parseMacOSCrashReport([]byte(sampleIPSReport(
		"ExampleApp",
		"com.example.app",
		"/Users/alice/Applications/ExampleApp.app/Contents/MacOS/ExampleApp",
		"INCIDENT-1",
	)))
	require.NoError(t, err)

	assert.Equal(t, macosCrashBugType, getString(report.metadata, "bug_type"))
	assert.Equal(t, "ExampleApp", report.appName())
	assert.Equal(t, "INCIDENT-1", report.incidentID())
	assert.True(t, report.captureTime().Equal(time.Date(2026, 7, 16, 19, 34, 56, 120000000, time.UTC)))
}

// TestMacOSCrashReportEventIsSanitized verifies emitted payloads exclude sensitive report data.
func TestMacOSCrashReportEventIsSanitized(t *testing.T) {
	report, err := parseMacOSCrashReport([]byte(sampleIPSReport(
		"ExampleApp",
		"com.example.app",
		"/Users/alice/Library/Application Support/Private Folder/ExampleApp",
		"INCIDENT-PRIVATE",
	)))
	require.NoError(t, err)

	event := report.event("incident:INCIDENT-PRIVATE", "user")
	assert.Equal(t, eventID("incident:INCIDENT-PRIVATE"), event.ID)
	assert.NotContains(t, event.ID, "INCIDENT-PRIVATE")

	custom := event.Custom["macos_diagnostic_report"].(map[string]interface{})
	assert.NotContains(t, custom, "app_location_hint")
	assert.NotContains(t, custom, "executable_basename")

	reportFields := custom["report"].(map[string]interface{})
	assert.NotContains(t, reportFields, "procPath")
	assert.NotContains(t, reportFields, "userID")
	assert.NotContains(t, reportFields, "threads")
	assert.Equal(t, map[string]interface{}{
		"CFBundleIdentifier": "com.example.app",
		"CFBundleVersion":    "123",
	}, reportFields["bundleInfo"])
	assert.Equal(t, map[string]interface{}{
		"signal": "SIGSEGV",
		"type":   "EXC_BAD_ACCESS",
	}, reportFields["exception"])
	assert.Equal(t, map[string]interface{}{
		"code":      int64(11),
		"namespace": "SIGNAL",
	}, reportFields["termination"])

	wire, err := json.Marshal(event)
	require.NoError(t, err)
	wireText := string(wire)
	assert.NotContains(t, wireText, "/Users/alice")
	assert.NotContains(t, wireText, "Private Folder")
	assert.NotContains(t, wireText, "procPath")
	assert.NotContains(t, wireText, ".ips")
	assert.NotContains(t, wireText, "deviceIdentifierForVendor")
	assert.NotContains(t, wireText, "Segmentation fault")
}

// TestMacOSNestedFieldsRequireExplicitScalarAllowlist verifies nested payload data remains explicitly allowlisted.
func TestMacOSNestedFieldsRequireExplicitScalarAllowlist(t *testing.T) {
	report, err := parseMacOSCrashReport([]byte(
		`{"bug_type":"309","incident_id":"INCIDENT"}` + "\n" +
			`{"bug_type":"309","procName":"App",` +
			`"bundleInfo":{"CFBundleIdentifier":{"secret":"/Users/alice"},"unknown":"secret"},` +
			`"exception":{"type":["EXC_BAD_ACCESS"],"signal":"SIGSEGV","unknown":"secret"},` +
			`"termination":{"namespace":"SIGNAL","code":{"secret":true},"indicator":"private"}}`,
	))
	require.NoError(t, err)

	fields := report.event("incident:INCIDENT", "system").
		Custom["macos_diagnostic_report"].(map[string]interface{})["report"].(map[string]interface{})
	assert.Empty(t, fields["bundleInfo"])
	assert.Equal(t, map[string]interface{}{"signal": "SIGSEGV"}, fields["exception"])
	assert.Equal(t, map[string]interface{}{"namespace": "SIGNAL"}, fields["termination"])
}

// TestAppLocationHintUsesCoarseKnownCategories verifies executable paths yield only coarse location labels.
func TestAppLocationHintUsesCoarseKnownCategories(t *testing.T) {
	tests := map[string]string{
		"/System/Applications/Utilities/Terminal.app/Contents/MacOS/Terminal": "system_application",
		"/Applications/ExampleApp.app/Contents/MacOS/ExampleApp":              "application",
		"/System/Library/CoreServices/Finder.app/Contents/MacOS/Finder":       "",
		"/Users/alice/Applications/Example.app/Contents/MacOS/Example":        "user_application",
		"/Users/alice/Library/Application Support/Example/Example":            "",
		"/Users/alice/Documents/private/Example":                              "",
		"/Users/alice":                                                        "",
		"/Users/alice/Applications":                                           "",
		"/Users//Applications/Example.app/Contents/MacOS/Example":             "",
		"/Users/alice//Applications/Example.app/Contents/MacOS/Example":       "",
		"/Users/alice/Applications/../Documents/private/Example":              "",
		"/private/var/folders/private/Example":                                "temporary",
		"/private/tmp/Example":                                                "temporary",
		"/Volumes/External/Example.app/Contents/MacOS/Example":                "external",
		"relative/private/Example":                                            "",
	}
	for path, expected := range tests {
		assert.Equal(t, expected, appLocationHint(path), path)
		assert.False(t, strings.Contains(appLocationHint(path), "alice"), path)
	}
}

// TestAppNamePathFallbackDoesNotExposeUsername verifies path-derived names omit user directory details.
func TestAppNamePathFallbackDoesNotExposeUsername(t *testing.T) {
	report := &macOSCrashReport{
		metadata: map[string]interface{}{},
		report:   map[string]interface{}{"procPath": "/Users/alice"},
	}
	assert.Equal(t, unknownDarwinProcessName, report.appName())
	custom := report.customPayload("user")
	assert.NotContains(t, custom, "executable_basename")
	wire, err := json.Marshal(custom)
	require.NoError(t, err)
	assert.NotContains(t, string(wire), "alice")

	report.report["procPath"] = "/Users/alice/Applications/Example.app/Contents/MacOS/private-binary"
	assert.Equal(t, "Example", report.appName())
	custom = report.customPayload("user")
	assert.Equal(t, "Example", custom["executable_basename"])
	wire, err = json.Marshal(custom)
	require.NoError(t, err)
	assert.NotContains(t, string(wire), "alice")
	assert.NotContains(t, string(wire), "private-binary")
}

// TestMacOSCrashReportRejectsHostileProcessNames verifies path-like and
// control-bearing labels fall back to a fixed non-sensitive name.
func TestMacOSCrashReportRejectsHostileProcessNames(t *testing.T) {
	hostileValues := []string{
		"/Users/alice/Applications/Secret.app",
		`..\Users\alice\Secret.exe`,
		"App\nforged",
		"App\x00hidden",
	}
	for _, hostile := range hostileValues {
		t.Run(hostile, func(t *testing.T) {
			report := &macOSCrashReport{
				metadata: map[string]interface{}{
					"bug_type":    macosCrashBugType,
					"name":        hostile,
					"app_name":    hostile,
					"incident_id": hostile,
				},
				report: map[string]interface{}{
					"procName": hostile,
					"procPath": "/Users/alice",
				},
			}

			event := report.event("hostile", "user")
			assert.Equal(t, "Application crash: "+unknownDarwinProcessName, event.Title)
			wire, err := json.Marshal(event)
			require.NoError(t, err)
			assert.NotContains(t, string(wire), "alice")
			assert.NotContains(t, string(wire), `\n`)
			assert.NotContains(t, string(wire), `\u0000`)
		})
	}
}

// TestMacOSCrashReportBoundsAllStringsAndWireSize verifies maximally populated
// allowlisted reports remain within both per-string and serialized limits.
func TestMacOSCrashReportBoundsAllStringsAndWireSize(t *testing.T) {
	longValue := strings.Repeat(`"<>&`, maxDarwinEventWireSize)
	metadata := make(map[string]interface{}, len(macosMetadataKeys))
	for _, key := range macosMetadataKeys {
		metadata[key] = longValue
	}
	metadata["bug_type"] = macosCrashBugType
	reportFields := make(map[string]interface{}, len(macosReportScalarKeys))
	for _, key := range macosReportScalarKeys {
		reportFields[key] = longValue
	}
	reportFields["procName"] = longValue
	for key, allowed := range macosNestedReportKeys {
		nested := make(map[string]interface{}, len(allowed))
		for _, nestedKey := range allowed {
			nested[nestedKey] = longValue
		}
		reportFields[key] = nested
	}
	report := &macOSCrashReport{metadata: metadata, report: reportFields}

	event := report.event("bounded", "hostile-scope")
	wire, err := json.Marshal(event)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(wire), maxDarwinEventWireSize)
	assert.True(t, eventFitsWireLimit(event))
	assertEventStringsBounded(t, event)
}

func assertEventStringsBounded(t *testing.T, event Event) {
	t.Helper()
	assert.LessOrEqual(t, len(event.ID), maxDarwinEventStringBytes)
	assert.LessOrEqual(t, len(event.EventType), maxDarwinEventStringBytes)
	assert.LessOrEqual(t, len(event.Title), maxDarwinEventStringBytes)
	assert.LessOrEqual(t, len(event.Message), maxDarwinEventStringBytes)
	assertValueStringsBounded(t, event.Custom)
}

func assertValueStringsBounded(t *testing.T, value interface{}) {
	t.Helper()
	switch typed := value.(type) {
	case string:
		assert.LessOrEqual(t, len(typed), maxDarwinEventStringBytes)
	case map[string]interface{}:
		for _, child := range typed {
			assertValueStringsBounded(t, child)
		}
	case []interface{}:
		for _, child := range typed {
			assertValueStringsBounded(t, child)
		}
	}
}

// parseMacOSCrashReport composes the production metadata and body parsers for test fixtures.
func parseMacOSCrashReport(data []byte) (*macOSCrashReport, error) {
	metadataData, reportData, found := strings.Cut(string(data), "\n")
	if !found {
		return nil, errors.New("missing report body")
	}
	metadata, err := parseMacOSCrashMetadata([]byte(metadataData))
	if err != nil {
		return nil, err
	}
	report, err := parseMacOSCrashReportBody([]byte(reportData))
	if err != nil {
		return nil, err
	}
	return &macOSCrashReport{metadata: metadata, report: report}, nil
}

// sampleIPSReport builds a representative two-object diagnostic report fixture.
func sampleIPSReport(appName, bundleID, procPath, incidentID string) string {
	return `{"app_name":"` + appName + `","timestamp":"2026-07-16 12:34:56.12 -0700","app_version":"1.2.3","build_version":"123","platform":1,"bundleID":"` + bundleID + `","bug_type":"309","os_version":"macOS 15.0 (24A335)","incident_id":"` + incidentID + `","name":"` + appName + `"}` + "\n" + `{
  "uptime": 1000,
  "procLaunch": "2026-07-16 12:34:54.00 -0700",
  "procRole": "Foreground",
  "version": 2,
  "userID": 501,
  "modelCode": "MacBookPro18,3",
  "captureTime": "2026-07-16 12:34:56.1200 -0700",
  "incident": "` + incidentID + `",
  "bug_type": "309",
  "pid": 12345,
  "cpuType": "ARM-64",
  "procName": "` + appName + `",
  "procPath": "` + procPath + `",
  "bundleInfo": {"CFBundleIdentifier": "` + bundleID + `", "CFBundleVersion": "123", "private": "/Users/alice"},
  "exception": {"type": "EXC_BAD_ACCESS", "signal": "SIGSEGV", "private": "/Users/alice"},
  "termination": {"namespace": "SIGNAL", "code": 11, "indicator": "Segmentation fault: 11"},
  "parentProc": "launchd",
  "parentPid": 1,
  "responsibleProc": "` + appName + `",
  "responsiblePid": 12345,
  "storeInfo": {"deviceIdentifierForVendor": "00000000-0000-0000-0000-000000000001"},
  "threads": [{"id": 1, "frames": [{"imageOffset": 1}]}],
  "usedImages": [{"name": "ExampleApp"}]
}`
}
