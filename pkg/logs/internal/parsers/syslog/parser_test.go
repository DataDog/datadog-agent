// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

func newTestMessage(content []byte) *message.Message {
	source := sources.NewLogSource("test", &config.LogsConfig{})
	origin := message.NewOrigin(source)
	msg := message.NewMessage(content, origin, "", time.Now().UnixNano())
	msg.RawDataLen = len(content)
	return msg
}

func TestFileParser_NetworkFormat_RFC5424(t *testing.T) {
	parser := NewParser()

	input := newTestMessage([]byte(`<165>1 2003-10-11T22:14:15.003Z mymachine evntslog - ID47 [exampleSDID@32473 iut="3"] An application event log entry`))
	result, err := parser.Parse(input)
	require.NoError(t, err)

	// StateStructured
	assert.Equal(t, message.StateStructured, result.State)

	// Content is the MSG body
	assert.Equal(t, "An application event log entry", string(result.GetContent()))

	// Status from severity
	assert.Equal(t, message.StatusNotice, result.Status)

	// Appname stored as source/service override in ParsingExtra
	assert.Equal(t, "evntslog", result.ParsingExtra.SourceOverride)
	assert.Equal(t, "evntslog", result.ParsingExtra.ServiceOverride)

	// RawDataLen preserved
	assert.Equal(t, len(input.GetContent()), result.RawDataLen)

	// Timestamp extracted
	assert.Equal(t, "2003-10-11T22:14:15.003Z", result.ParsingExtra.Timestamp)
}

func TestFileParser_NetworkFormat_BSD(t *testing.T) {
	parser := NewParser()

	input := newTestMessage([]byte(`<34>Oct 11 22:14:15 mymachine su: 'su root' failed for lonvick on /dev/pts/8`))
	result, err := parser.Parse(input)
	require.NoError(t, err)

	assert.Equal(t, message.StateStructured, result.State)
	assert.Equal(t, message.StatusCritical, result.Status)

	// Appname stored as source/service override in ParsingExtra
	assert.Equal(t, "su", result.ParsingExtra.SourceOverride)
	assert.Equal(t, "su", result.ParsingExtra.ServiceOverride)
}

func TestFileParser_BSDLineFormat(t *testing.T) {
	parser := NewParser()

	// BSD line without PRI: "Oct 11 22:14:15 mymachine su: message"
	input := newTestMessage([]byte(`Oct 11 22:14:15 mymachine su: 'su root' failed for lonvick on /dev/pts/8`))
	result, err := parser.Parse(input)
	require.NoError(t, err)

	assert.Equal(t, message.StateStructured, result.State)

	// No PRI -> StatusInfo
	assert.Equal(t, message.StatusInfo, result.Status)

	// Appname stored as source/service override in ParsingExtra
	assert.Equal(t, "su", result.ParsingExtra.SourceOverride)
	assert.Equal(t, "su", result.ParsingExtra.ServiceOverride)
}

func TestFileParser_AppNameNILVALUE(t *testing.T) {
	parser := NewParser()

	input := newTestMessage([]byte(`<14>1 2003-10-11T22:14:15.003Z mymachine - - - - test message`))
	result, err := parser.Parse(input)
	require.NoError(t, err)
	assert.Equal(t, message.StateStructured, result.State)

	// NILVALUE appname should not set override
	assert.Equal(t, "", result.ParsingExtra.SourceOverride)
	assert.Equal(t, "", result.ParsingExtra.ServiceOverride)
}

func TestFileParser_Malformed(t *testing.T) {
	parser := NewParser()

	input := newTestMessage([]byte(`<14>`))
	result, err := parser.Parse(input)

	// Should return error but still produce a message
	assert.Error(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, message.StateStructured, result.State)
}

func TestFileParser_SupportsPartialLine(t *testing.T) {
	parser := NewParser()
	assert.False(t, parser.SupportsPartialLine())
}

func TestFileParser_AutoDetect_MixedFormats(t *testing.T) {
	// A single parser instance should auto-detect PRI vs non-PRI per line.
	parser := NewParser()

	// Line with PRI (network format)
	priInput := newTestMessage([]byte(`<14>1 2003-10-11T22:14:15.003Z myhost myapp - - - PRI message`))
	priResult, err := parser.Parse(priInput)
	require.NoError(t, err)
	assert.Equal(t, message.StateStructured, priResult.State)
	assert.Equal(t, "PRI message", string(priResult.GetContent()))
	assert.Equal(t, message.StatusInfo, priResult.Status) // 14%8=6 -> info

	// Line without PRI (BSD on-disk format)
	bsdInput := newTestMessage([]byte(`Oct 11 22:14:15 myhost su: BSD message`))
	bsdResult, err := parser.Parse(bsdInput)
	require.NoError(t, err)
	assert.Equal(t, message.StateStructured, bsdResult.State)
	assert.Equal(t, "BSD message", string(bsdResult.GetContent()))
	assert.Equal(t, message.StatusInfo, bsdResult.Status) // no PRI -> -1 -> info

	// Another PRI line to confirm no state lock-in
	pri2Input := newTestMessage([]byte(`<11>1 2003-10-11T22:14:16.003Z myhost otherapp - - - Second PRI`))
	pri2Result, err := parser.Parse(pri2Input)
	require.NoError(t, err)
	assert.Equal(t, "Second PRI", string(pri2Result.GetContent()))
	assert.Equal(t, message.StatusError, pri2Result.Status) // 11%8=3 -> error
}

func TestFileParser_NonSyslogText(t *testing.T) {
	parser := NewParser()

	lines := []string{
		"this is not syslog at all",
		"ERROR 2025-01-15 connection refused",
		"12345 plain numeric line",
		`{"json":"log","level":"warn"}`,
	}

	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			input := newTestMessage([]byte(line))
			result, err := parser.Parse(input)

			// Parser returns an error but still produces a usable message.
			assert.Error(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, message.StateStructured, result.State)

			// Status falls back to info (Pri=-1 → no severity).
			assert.Equal(t, message.StatusInfo, result.Status)

			// The entire input is preserved as the structured message body.
			assert.Equal(t, line, string(result.GetContent()))

			rendered, rerr := result.Render()
			require.NoError(t, rerr)
			assert.Contains(t, string(rendered), `"message"`)
			assert.Contains(t, string(rendered), `"syslog"`)

			// Syslog metadata is sparse — no source/service override.
			assert.Empty(t, result.ParsingExtra.SourceOverride)
			assert.Empty(t, result.ParsingExtra.ServiceOverride)
		})
	}
}

func TestFileParser_RenderedContent_RFC5424(t *testing.T) {
	parser := NewParser()

	input := newTestMessage([]byte(`<165>1 2003-10-11T22:14:15.003Z mymachine evntslog - ID47 [exampleSDID@32473 iut="3"] An application event log entry`))
	result, err := parser.Parse(input)
	require.NoError(t, err)

	// Render should produce valid JSON with message and syslog fields
	rendered, err := result.Render()
	require.NoError(t, err)
	assert.Contains(t, string(rendered), `"message"`)
	assert.Contains(t, string(rendered), `"syslog"`)
	assert.Contains(t, string(rendered), `"An application event log entry"`)
}

// ---------------------------------------------------------------------------
// CEF/LEEF integration tests (full syslog envelope + CEF/LEEF body)
// ---------------------------------------------------------------------------

func TestFileParser_CEF_RFC5424(t *testing.T) {
	parser := NewParser()

	input := newTestMessage([]byte(
		`<14>1 2026-03-30T12:00:00Z firewall01 CEF - - - CEF:0|Security|threatmanager|1.0|100|worm successfully stopped|10|src=10.0.0.1 dst=2.1.2.2 spt=1232`,
	))
	result, err := parser.Parse(input)
	require.NoError(t, err)

	assert.Equal(t, message.StateStructured, result.State)

	// CEF/LEEF messages have an empty message body; all data is in "siem"
	assert.Equal(t, "", string(result.GetContent()))

	// Render and verify JSON structure
	rendered, err := result.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(rendered, &data))

	// Syslog envelope fields should be present
	syslog, ok := data["syslog"].(map[string]interface{})
	require.True(t, ok, "syslog key missing")
	assert.Equal(t, "firewall01", syslog["hostname"])
	assert.Equal(t, "CEF", syslog["appname"])

	// SIEM fields should be present
	siem, ok := data["siem"].(map[string]interface{})
	require.True(t, ok, "siem key missing from rendered output")
	assert.Equal(t, "CEF", siem["format"])
	assert.Equal(t, "0", siem["version"])
	assert.Equal(t, "Security", siem["device_vendor"])
	assert.Equal(t, "threatmanager", siem["device_product"])
	assert.Equal(t, "1.0", siem["device_version"])
	assert.Equal(t, "100", siem["event_id"])
	assert.Equal(t, "worm successfully stopped", siem["name"])
	assert.Equal(t, "10", siem["severity"])

	ext, ok := siem["extension"].(map[string]interface{})
	require.True(t, ok, "extension missing")
	assert.Equal(t, "10.0.0.1", ext["src"])
	assert.Equal(t, "2.1.2.2", ext["dst"])
	assert.Equal(t, "1232", ext["spt"])
}

func TestFileParser_CEF_BSD(t *testing.T) {
	parser := NewParser()

	input := newTestMessage([]byte(
		`<34>Oct 11 22:14:15 myhost CEF: CEF:0|Vendor|Product|1.0|200|Attack|8|src=1.2.3.4`,
	))
	result, err := parser.Parse(input)
	require.NoError(t, err)

	assert.Equal(t, "", string(result.GetContent()))

	rendered, err := result.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(rendered, &data))

	siem, ok := data["siem"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "CEF", siem["format"])
	assert.Equal(t, "Vendor", siem["device_vendor"])
}

func TestFileParser_LEEF10_RFC5424(t *testing.T) {
	parser := NewParser()

	input := newTestMessage([]byte(
		"<13>1 2026-03-30T12:00:00Z server01 LEEF - - - LEEF:1.0|Microsoft|MSExchange|2013 SP1|15345|src=10.0.1.7\tdst=10.0.0.5\tsev=5",
	))
	result, err := parser.Parse(input)
	require.NoError(t, err)

	assert.Equal(t, message.StateStructured, result.State)

	rendered, err := result.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(rendered, &data))

	siem, ok := data["siem"].(map[string]interface{})
	require.True(t, ok, "siem key missing from rendered output")
	assert.Equal(t, "LEEF", siem["format"])
	assert.Equal(t, "1.0", siem["version"])
	assert.Equal(t, "Microsoft", siem["device_vendor"])
	assert.Equal(t, "MSExchange", siem["device_product"])
	assert.Equal(t, "2013 SP1", siem["device_version"])
	assert.Equal(t, "15345", siem["event_id"])

	// LEEF has no name or severity in header
	_, hasName := siem["name"]
	assert.False(t, hasName)
	_, hasSev := siem["severity"]
	assert.False(t, hasSev)

	ext, ok := siem["extension"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "10.0.1.7", ext["src"])
	assert.Equal(t, "10.0.0.5", ext["dst"])
	assert.Equal(t, "5", ext["sev"])
}

func TestFileParser_LEEF20_CustomDelimiter(t *testing.T) {
	parser := NewParser()

	input := newTestMessage([]byte(
		`<113>1 2026-03-30T12:00:00Z hostname4 LEEF - - - LEEF:2.0|Lancope|StealthWatch|1.0|41|^|src=10.0.1.8^dst=10.0.0.5^sev=5`,
	))
	result, err := parser.Parse(input)
	require.NoError(t, err)

	rendered, err := result.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(rendered, &data))

	siem, ok := data["siem"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "LEEF", siem["format"])
	assert.Equal(t, "2.0", siem["version"])
	assert.Equal(t, "41", siem["event_id"])

	ext, ok := siem["extension"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "10.0.1.8", ext["src"])
	assert.Equal(t, "10.0.0.5", ext["dst"])
}

func TestFileParser_NonCEFLEEF_Unchanged(t *testing.T) {
	parser := NewParser()

	// A normal syslog message should NOT have a "siem" key.
	input := newTestMessage([]byte(
		`<165>1 2003-10-11T22:14:15.003Z mymachine evntslog - ID47 - Just a regular message`,
	))
	result, err := parser.Parse(input)
	require.NoError(t, err)

	rendered, err := result.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(rendered, &data))

	_, hasSiem := data["siem"]
	assert.False(t, hasSiem, "siem key should not be present for non-CEF/LEEF messages")
	assert.Equal(t, "Just a regular message", data["message"])
}

func TestFileParser_CEF_EscapedPipesInHeader(t *testing.T) {
	parser := NewParser()

	input := newTestMessage([]byte(
		`<14>1 2026-03-30T12:00:00Z host app - - - CEF:0|Vendor\|Inc|Product|1.0|100|Name|5|src=1.2.3.4`,
	))
	result, err := parser.Parse(input)
	require.NoError(t, err)

	rendered, err := result.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(rendered, &data))

	siem := data["siem"].(map[string]interface{})
	assert.Equal(t, "Vendor|Inc", siem["device_vendor"])
}

func TestFileParser_CEF_ExtensionWithSpacesInValue(t *testing.T) {
	parser := NewParser()

	input := newTestMessage([]byte(
		`<14>1 2026-03-30T12:00:00Z host app - - - CEF:0|V|P|1.0|100|N|5|filePath=/user/dir/my file.txt dst=10.0.0.1`,
	))
	result, err := parser.Parse(input)
	require.NoError(t, err)

	rendered, err := result.Render()
	require.NoError(t, err)

	var data map[string]interface{}
	require.NoError(t, json.Unmarshal(rendered, &data))

	ext := data["siem"].(map[string]interface{})["extension"].(map[string]interface{})
	assert.Equal(t, "/user/dir/my file.txt", ext["filePath"])
	assert.Equal(t, "10.0.0.1", ext["dst"])
}
