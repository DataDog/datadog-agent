// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
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
	parser := NewFileParser()

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
	parser := NewFileParser()

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
	parser := NewFileParser()

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
	parser := NewFileParser()

	input := newTestMessage([]byte(`<14>1 2003-10-11T22:14:15.003Z mymachine - - - - test message`))
	result, err := parser.Parse(input)
	require.NoError(t, err)
	assert.Equal(t, message.StateStructured, result.State)

	// NILVALUE appname should not set override
	assert.Equal(t, "", result.ParsingExtra.SourceOverride)
	assert.Equal(t, "", result.ParsingExtra.ServiceOverride)
}

func TestFileParser_Malformed(t *testing.T) {
	parser := NewFileParser()

	input := newTestMessage([]byte(`<14>`))
	result, err := parser.Parse(input)

	// Should return error but still produce a message
	assert.Error(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, message.StateStructured, result.State)
}

func TestFileParser_SupportsPartialLine(t *testing.T) {
	parser := NewFileParser()
	assert.False(t, parser.SupportsPartialLine())
}

func TestFileParser_AutoDetect_MixedFormats(t *testing.T) {
	// A single parser instance should auto-detect PRI vs non-PRI per line.
	parser := NewFileParser()

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
	parser := NewFileParser()

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
	parser := NewFileParser()

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
