// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func TestSeverityToStatus(t *testing.T) {
	tests := []struct {
		pri    int
		status string
	}{
		{0, message.StatusEmergency}, // severity 0
		{1, message.StatusAlert},     // severity 1
		{2, message.StatusCritical},  // severity 2
		{3, message.StatusError},     // severity 3
		{4, message.StatusWarning},   // severity 4
		{5, message.StatusNotice},    // severity 5
		{6, message.StatusInfo},      // severity 6
		{7, message.StatusDebug},     // severity 7
		{8, message.StatusEmergency}, // facility 1, severity 0
		{14, message.StatusInfo},     // facility 1, severity 6
		{165, message.StatusNotice},  // facility 20, severity 5
		{-1, message.StatusInfo},     // absent PRI
	}

	for _, tt := range tests {
		assert.Equal(t, tt.status, SeverityToStatus(tt.pri), "pri=%d", tt.pri)
	}
}

func TestBuildSyslogFields_RFC5424(t *testing.T) {
	parsed := SyslogMessage{
		Pri:       165,
		Version:   "1",
		Timestamp: "2003-10-11T22:14:15.003Z",
		Hostname:  "mymachine",
		AppName:   "evntslog",
		ProcID:    "-",
		MsgID:     "ID47",
		StructuredData: map[string]map[string]string{
			"exampleSDID@32473": {"iut": "3"},
		},
		Msg: []byte("An application event log entry"),
	}

	fields := BuildSyslogFields(parsed)

	assert.Equal(t, "2003-10-11T22:14:15.003Z", fields["timestamp"])
	assert.Equal(t, "mymachine", fields["hostname"])
	assert.Equal(t, "evntslog", fields["appname"])
	assert.Equal(t, "-", fields["procid"])
	assert.Equal(t, "ID47", fields["msgid"])
	assert.Equal(t, 5, fields["severity"])  // 165 % 8
	assert.Equal(t, 20, fields["facility"]) // 165 / 8
	assert.Equal(t, "1", fields["version"])
	assert.Equal(t, map[string]map[string]string{
		"exampleSDID@32473": {"iut": "3"},
	}, fields["structured_data"])
}

func TestBuildSyslogFields_BSD(t *testing.T) {
	parsed := SyslogMessage{
		Pri:       38,
		Timestamp: "Oct 11 22:14:15",
		Hostname:  "mymachine",
		AppName:   "su",
		ProcID:    "-",
		MsgID:     "-",
	}

	fields := BuildSyslogFields(parsed)

	// BSD: no version, no structured_data
	_, hasVersion := fields["version"]
	assert.False(t, hasVersion, "BSD messages should not have version")
	_, hasSD := fields["structured_data"]
	assert.False(t, hasSD, "BSD messages should not have structured_data")

	// severity = 38 % 8 = 6, facility = 38 / 8 = 4
	assert.Equal(t, 6, fields["severity"])
	assert.Equal(t, 4, fields["facility"])
}

func TestBuildSyslogFields_NoPri(t *testing.T) {
	parsed := SyslogMessage{
		Pri: -1,
	}

	fields := BuildSyslogFields(parsed)

	_, hasSev := fields["severity"]
	assert.False(t, hasSev, "Pri=-1 should omit severity")
	_, hasFac := fields["facility"]
	assert.False(t, hasFac, "Pri=-1 should omit facility")
}

// ---------------------------------------------------------------------------
// parseStructuredData unit tests
// ---------------------------------------------------------------------------

func TestParseStructuredData_SingleElement(t *testing.T) {
	input := []byte(`[exampleSDID@32473 iut="3" eventSource="Application"]`)
	sd, n, err := parseStructuredData(input)
	assert.NoError(t, err)
	assert.Equal(t, len(input), n)
	assert.Equal(t, map[string]map[string]string{
		"exampleSDID@32473": {
			"iut":         "3",
			"eventSource": "Application",
		},
	}, sd)
}

func TestParseStructuredData_MultipleElements(t *testing.T) {
	input := []byte(`[id1 a="1"][id2 b="2" c="3"]`)
	sd, n, err := parseStructuredData(input)
	assert.NoError(t, err)
	assert.Equal(t, len(input), n)
	assert.Equal(t, map[string]map[string]string{
		"id1": {"a": "1"},
		"id2": {"b": "2", "c": "3"},
	}, sd)
}

func TestParseStructuredData_NILVALUE(t *testing.T) {
	input := []byte(`-`)
	sd, n, err := parseStructuredData(input)
	assert.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.Nil(t, sd)
}

func TestParseStructuredData_ElementNoParams(t *testing.T) {
	input := []byte(`[myID]`)
	sd, n, err := parseStructuredData(input)
	assert.NoError(t, err)
	assert.Equal(t, len(input), n)
	assert.Equal(t, map[string]map[string]string{
		"myID": {},
	}, sd)
}

func TestParseStructuredData_EscapedQuote(t *testing.T) {
	input := []byte(`[id1 key="val\"ue"]`)
	sd, _, err := parseStructuredData(input)
	assert.NoError(t, err)
	assert.Equal(t, `val"ue`, sd["id1"]["key"])
}

func TestParseStructuredData_EscapedBackslash(t *testing.T) {
	input := []byte(`[id1 key="val\\ue"]`)
	sd, _, err := parseStructuredData(input)
	assert.NoError(t, err)
	assert.Equal(t, `val\ue`, sd["id1"]["key"])
}

func TestParseStructuredData_EscapedBracket(t *testing.T) {
	input := []byte(`[id1 key="val\]ue"]`)
	sd, _, err := parseStructuredData(input)
	assert.NoError(t, err)
	assert.Equal(t, `val]ue`, sd["id1"]["key"])
}

func TestParseStructuredData_NoEscapeFastPath(t *testing.T) {
	input := []byte(`[id1 key="simple"]`)
	sd, _, err := parseStructuredData(input)
	assert.NoError(t, err)
	assert.Equal(t, "simple", sd["id1"]["key"])
}

func TestParseStructuredData_Empty(t *testing.T) {
	_, _, err := parseStructuredData([]byte{})
	assert.Error(t, err)
}

func TestParseStructuredData_InvalidStart(t *testing.T) {
	_, _, err := parseStructuredData([]byte(`x`))
	assert.Error(t, err)
}

func TestParseStructuredData_MultipleEscapes(t *testing.T) {
	input := []byte(`[id1 key="a\"b\\c\]d"]`)
	sd, _, err := parseStructuredData(input)
	assert.NoError(t, err)
	assert.Equal(t, `a"b\c]d`, sd["id1"]["key"])
}
