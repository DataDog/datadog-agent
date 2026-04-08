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

// ---------------------------------------------------------------------------
// Parse() direct unit tests — RFC 5424
// ---------------------------------------------------------------------------

func TestParse_RFC5424_BasicNoSD(t *testing.T) {
	msg, err := Parse([]byte(`<34>1 2003-10-11T22:14:15.003Z mymachine.example.com su - ID47 - BOM'su root' failed for lonvick on /dev/pts/8`))
	assert.NoError(t, err)
	assert.False(t, msg.Partial)
	assert.Equal(t, 34, msg.Pri)
	assert.Equal(t, "1", msg.Version)
	assert.Equal(t, "2003-10-11T22:14:15.003Z", msg.Timestamp)
	assert.Equal(t, "mymachine.example.com", msg.Hostname)
	assert.Equal(t, "su", msg.AppName)
	assert.Equal(t, "-", msg.ProcID)
	assert.Equal(t, "ID47", msg.MsgID)
	assert.Nil(t, msg.StructuredData)
	assert.Equal(t, "BOM'su root' failed for lonvick on /dev/pts/8", string(msg.Msg))
}

func TestParse_RFC5424_WithSD(t *testing.T) {
	msg, err := Parse([]byte(`<165>1 2003-10-11T22:14:15.003Z mymachine.example.com evntslog - ID47 [exampleSDID@32473 iut="3" eventSource="Application" eventID="1011"] BOMAn application event log entry...`))
	assert.NoError(t, err)
	assert.False(t, msg.Partial)
	assert.Equal(t, 165, msg.Pri)
	assert.Equal(t, "1", msg.Version)
	assert.Equal(t, "2003-10-11T22:14:15.003Z", msg.Timestamp)
	assert.Equal(t, "mymachine.example.com", msg.Hostname)
	assert.Equal(t, "evntslog", msg.AppName)
	assert.Equal(t, "-", msg.ProcID)
	assert.Equal(t, "ID47", msg.MsgID)
	assert.Equal(t, map[string]map[string]string{
		"exampleSDID@32473": {
			"iut":         "3",
			"eventSource": "Application",
			"eventID":     "1011",
		},
	}, msg.StructuredData)
	assert.Equal(t, "BOMAn application event log entry...", string(msg.Msg))
}

func TestParse_RFC5424_EmptyParamValue(t *testing.T) {
	msg, err := Parse([]byte(`<165>1 2003-10-11T22:14:15.003Z mymachine evntslog - ID47 [exampleSDID@32473 iut="3" eventSource="" eventID="1011"] msg`))
	assert.NoError(t, err)
	assert.Equal(t, "", msg.StructuredData["exampleSDID@32473"]["eventSource"])
	assert.Equal(t, "3", msg.StructuredData["exampleSDID@32473"]["iut"])
	assert.Equal(t, "1011", msg.StructuredData["exampleSDID@32473"]["eventID"])
}

func TestParse_RFC5424_MultipleSD(t *testing.T) {
	msg, err := Parse([]byte(`<13>1 2019-02-13T19:48:34+00:00 74794bfb6795 root 8449 - [meta sequenceId="1" sysUpTime="37" language="EN"][origin ip="192.168.0.1" software="test"] i am foobar`))
	assert.NoError(t, err)
	assert.Equal(t, 13, msg.Pri)
	assert.Equal(t, "1", msg.Version)
	assert.Equal(t, "2019-02-13T19:48:34+00:00", msg.Timestamp)
	assert.Equal(t, "74794bfb6795", msg.Hostname)
	assert.Equal(t, "root", msg.AppName)
	assert.Equal(t, "8449", msg.ProcID)
	assert.Equal(t, map[string]map[string]string{
		"meta":   {"sequenceId": "1", "sysUpTime": "37", "language": "EN"},
		"origin": {"ip": "192.168.0.1", "software": "test"},
	}, msg.StructuredData)
	assert.Equal(t, "i am foobar", string(msg.Msg))
}

func TestParse_RFC5424_NILVALUETimestamp(t *testing.T) {
	msg, err := Parse([]byte(`<14>1 - 10.0.4.87 Serial-Debugger - - - Serializer started!`))
	assert.NoError(t, err)
	assert.Equal(t, 14, msg.Pri)
	assert.Equal(t, "1", msg.Version)
	assert.Equal(t, "-", msg.Timestamp)
	assert.Equal(t, "10.0.4.87", msg.Hostname)
	assert.Equal(t, "Serial-Debugger", msg.AppName)
	assert.Equal(t, "-", msg.ProcID)
	assert.Equal(t, "-", msg.MsgID)
	assert.Nil(t, msg.StructuredData)
	assert.Equal(t, "Serializer started!", string(msg.Msg))
}

func TestParse_RFC5424_IPv4Hostname(t *testing.T) {
	msg, err := Parse([]byte(`<34>1 2003-10-11T22:14:15.003Z 42.52.1.1 su - ID47 - bananas and peas`))
	assert.NoError(t, err)
	assert.Equal(t, "42.52.1.1", msg.Hostname)
	assert.Equal(t, "su", msg.AppName)
	assert.Equal(t, "ID47", msg.MsgID)
	assert.Equal(t, "bananas and peas", string(msg.Msg))
}

func TestParse_RFC5424_IPv6Hostname(t *testing.T) {
	msg, err := Parse([]byte(`<34>1 2003-10-11T22:14:15.003Z ::FFFF:129.144.52.38 su - ID47 - bananas and peas`))
	assert.NoError(t, err)
	assert.Equal(t, "::FFFF:129.144.52.38", msg.Hostname)
	assert.Equal(t, "su", msg.AppName)
	assert.Equal(t, "bananas and peas", string(msg.Msg))
}

func TestParse_RFC5424_ColonInAppname(t *testing.T) {
	msg, err := Parse([]byte(`<28>1 2020-05-22T14:59:09.250-03:00 OX-XXX-MX204 OX-XXX-CONTEUDO:rpd 6589 - - bgp_listen_accept: Connection attempt from unconfigured neighbor`))
	assert.NoError(t, err)
	assert.Equal(t, 28, msg.Pri)
	assert.Equal(t, "OX-XXX-MX204", msg.Hostname)
	assert.Equal(t, "OX-XXX-CONTEUDO:rpd", msg.AppName)
	assert.Equal(t, "6589", msg.ProcID)
	assert.Equal(t, "bgp_listen_accept: Connection attempt from unconfigured neighbor", string(msg.Msg))
}

func TestParse_RFC5424_ColonInMsgID(t *testing.T) {
	msg, err := Parse([]byte(`<131>1 2025-05-09T09:56:18.906539+02:00 Host-Name.network.example appname 1234 01230456:1: [F5@1234 hostname="Host-Name.network.example" errdefs_msgno="01230456:1:"] RST sent from 192.0.2.1:443`))
	assert.NoError(t, err)
	assert.Equal(t, 131, msg.Pri)
	assert.Equal(t, "Host-Name.network.example", msg.Hostname)
	assert.Equal(t, "appname", msg.AppName)
	assert.Equal(t, "1234", msg.ProcID)
	assert.Equal(t, "01230456:1:", msg.MsgID)
	assert.Equal(t, map[string]map[string]string{
		"F5@1234": {
			"hostname":      "Host-Name.network.example",
			"errdefs_msgno": "01230456:1:",
		},
	}, msg.StructuredData)
	assert.Equal(t, "RST sent from 192.0.2.1:443", string(msg.Msg))
}

func TestParse_RFC5424_EmptyMsg(t *testing.T) {
	msg, err := Parse([]byte(`<75>1 1969-12-03T23:58:58Z - - - - -`))
	assert.NoError(t, err)
	assert.Equal(t, 75, msg.Pri)
	assert.Equal(t, "1", msg.Version)
	assert.Equal(t, "1969-12-03T23:58:58Z", msg.Timestamp)
	assert.Equal(t, "-", msg.Hostname)
	assert.Equal(t, "-", msg.AppName)
	assert.Equal(t, "-", msg.ProcID)
	assert.Equal(t, "-", msg.MsgID)
	assert.Nil(t, msg.StructuredData)
	assert.Empty(t, msg.Msg)
}

func TestParse_RFC5424_EmptySDElement(t *testing.T) {
	msg, err := Parse([]byte(`<13>1 2019-02-13T19:48:34+00:00 74794bfb6795 root 8449 - [empty] qwerty`))
	assert.NoError(t, err)
	assert.Equal(t, map[string]map[string]string{
		"empty": {},
	}, msg.StructuredData)
	assert.Equal(t, "qwerty", string(msg.Msg))
}

func TestParse_RFC5424_EmptyAndNonEmptySDElements(t *testing.T) {
	msg, err := Parse([]byte(`<13>1 2019-02-13T19:48:34+00:00 74794bfb6795 root 8449 - [non_empty x="1"][empty] qwerty`))
	assert.NoError(t, err)
	assert.Equal(t, map[string]map[string]string{
		"non_empty": {"x": "1"},
		"empty":     {},
	}, msg.StructuredData)
	assert.Equal(t, "qwerty", string(msg.Msg))
}

func TestParse_RFC5424_IncorrectSDElement(t *testing.T) {
	// SD element with a bare param name (no ="value") is malformed.
	msg, err := Parse([]byte(`<13>1 2019-02-13T19:48:34+00:00 74794bfb6795 root 8449 - [incorrect x] qwerty`))
	assert.Error(t, err)
	assert.True(t, msg.Partial)
	assert.Equal(t, "74794bfb6795", msg.Hostname)
	assert.Equal(t, "root", msg.AppName)
	assert.Equal(t, "8449", msg.ProcID)
}

func TestParse_RFC5424_BOMStripping(t *testing.T) {
	// UTF-8 BOM (0xEF 0xBB 0xBF) before the message body should be stripped.
	input := append(
		[]byte(`<34>1 2003-10-11T22:14:15.003Z myhost su - ID47 - `),
		0xEF, 0xBB, 0xBF,
	)
	input = append(input, []byte("test message")...)
	msg, err := Parse(input)
	assert.NoError(t, err)
	assert.Equal(t, "test message", string(msg.Msg))
}

func TestParse_RFC5424_NegativeTimezoneOffset(t *testing.T) {
	msg, err := Parse([]byte(`<28>1 2020-05-22T14:59:09.250-03:00 myhost myapp 6589 - - some message`))
	assert.NoError(t, err)
	assert.Equal(t, "2020-05-22T14:59:09.250-03:00", msg.Timestamp)
	assert.Equal(t, "myhost", msg.Hostname)
	assert.Equal(t, "myapp", msg.AppName)
}

// ---------------------------------------------------------------------------
// Parse() direct unit tests — BSD / RFC 3164
// ---------------------------------------------------------------------------

func TestParse_BSD_WithPID(t *testing.T) {
	msg, err := Parse([]byte(`<13>Feb 13 20:07:26 74794bfb6795 root[8539]: i am foobar`))
	assert.NoError(t, err)
	assert.Equal(t, 13, msg.Pri)
	assert.Equal(t, "Feb 13 20:07:26", msg.Timestamp)
	assert.Equal(t, "74794bfb6795", msg.Hostname)
	assert.Equal(t, "root", msg.AppName)
	assert.Equal(t, "8539", msg.ProcID)
	assert.Equal(t, "i am foobar", string(msg.Msg))
}

func TestParse_BSD_AppnameColonMsg(t *testing.T) {
	msg, err := Parse([]byte(`<190>Dec 28 16:49:07 plertrood-thinkpad-x220 nginx: 127.0.0.1 - - request`))
	assert.NoError(t, err)
	assert.Equal(t, 190, msg.Pri)
	assert.Equal(t, "Dec 28 16:49:07", msg.Timestamp)
	assert.Equal(t, "plertrood-thinkpad-x220", msg.Hostname)
	assert.Equal(t, "nginx", msg.AppName)
	assert.Equal(t, "-", msg.ProcID)
	assert.Equal(t, "127.0.0.1 - - request", string(msg.Msg))
}

func TestParse_BSD_SingleDigitDay(t *testing.T) {
	msg, err := Parse([]byte(`<46>Jan  5 15:33:03 plertrood-ThinkPad-X220 rsyslogd: start`))
	assert.NoError(t, err)
	assert.Equal(t, 46, msg.Pri)
	assert.Equal(t, "Jan  5 15:33:03", msg.Timestamp)
	assert.Equal(t, "plertrood-ThinkPad-X220", msg.Hostname)
	assert.Equal(t, "rsyslogd", msg.AppName)
	assert.Equal(t, "start", string(msg.Msg))
}

func TestParse_BSD_NoTag(t *testing.T) {
	// Double space after hostname: the parser consumes one SP after the
	// hostname, leaving the second SP as the first byte of rest. That byte
	// is non-alphanumeric, so parseBSDTag treats the whole rest as MSG.
	msg, err := Parse([]byte(`<46>Jan  5 15:33:03 plertrood-ThinkPad-X220  [some content] start`))
	assert.NoError(t, err)
	assert.Equal(t, "plertrood-ThinkPad-X220", msg.Hostname)
	assert.Equal(t, "-", msg.AppName)
	assert.Equal(t, " [some content] start", string(msg.Msg))
}

func TestParse_BSD_BracketContentInMsg(t *testing.T) {
	// iptables-style content: brackets in MSG that are NOT structured data.
	msg, err := Parse([]byte(`<4>Jan 26 05:59:54 ubnt kernel: [WAN_LOCAL-default-D]IN=eth0 OUT= SRC=135.148.25.121`))
	assert.NoError(t, err)
	assert.Equal(t, 4, msg.Pri)
	assert.Equal(t, "ubnt", msg.Hostname)
	assert.Equal(t, "kernel", msg.AppName)
	assert.Equal(t, "[WAN_LOCAL-default-D]IN=eth0 OUT= SRC=135.148.25.121", string(msg.Msg))
}

func TestParse_BSD_ImmediateColonNoSpace(t *testing.T) {
	// TAG with colon immediately followed by content (no space after colon).
	msg, err := Parse([]byte(`<13>Feb 13 20:07:26 74794bfb6795 root[8539]:syslog message`))
	assert.NoError(t, err)
	assert.Equal(t, "root", msg.AppName)
	assert.Equal(t, "8539", msg.ProcID)
	assert.Equal(t, "syslog message", string(msg.Msg))
}

func TestParseBSDLine_NoPRI(t *testing.T) {
	msg, err := ParseBSDLine([]byte(`Dec 28 16:49:07 plertrood-thinkpad-x220 nginx: 127.0.0.1 - - request`))
	assert.NoError(t, err)
	assert.Equal(t, -1, msg.Pri)
	assert.Equal(t, "Dec 28 16:49:07", msg.Timestamp)
	assert.Equal(t, "plertrood-thinkpad-x220", msg.Hostname)
	assert.Equal(t, "nginx", msg.AppName)
	assert.Equal(t, "127.0.0.1 - - request", string(msg.Msg))
}

// ---------------------------------------------------------------------------
// Parse() edge cases
// ---------------------------------------------------------------------------

func TestParse_EmptyInput(t *testing.T) {
	_, err := Parse([]byte{})
	assert.Error(t, err)
}

func TestParse_InvalidMessage(t *testing.T) {
	_, err := Parse([]byte("complete and utter gobbledegook"))
	assert.Error(t, err)
}

func TestParse_PRIOnly(t *testing.T) {
	msg, err := Parse([]byte(`<14>`))
	assert.Error(t, err)
	_ = msg
}

func TestParse_PRIOutOfRange(t *testing.T) {
	// PRI > 191 is out of spec but the parser accepts it best-effort.
	msg, err := Parse([]byte(`<192>1 2003-10-11T22:14:15.003Z myhost myapp - - - test`))
	assert.Error(t, err)
	assert.True(t, msg.Partial)
	assert.Equal(t, 192, msg.Pri)
	assert.Equal(t, "myhost", msg.Hostname)
}

func TestParse_AllNILVALUEFields(t *testing.T) {
	msg, err := Parse([]byte(`<14>1 - - - - - - test`))
	assert.NoError(t, err)
	assert.Equal(t, "1", msg.Version)
	assert.Equal(t, "-", msg.Timestamp)
	assert.Equal(t, "-", msg.Hostname)
	assert.Equal(t, "-", msg.AppName)
	assert.Equal(t, "-", msg.ProcID)
	assert.Equal(t, "-", msg.MsgID)
	assert.Nil(t, msg.StructuredData)
	assert.Equal(t, "test", string(msg.Msg))
}

func TestParseBSDLine_EmptyInput(t *testing.T) {
	_, err := ParseBSDLine([]byte{})
	assert.Error(t, err)
}

func TestParse_BSD_HighPRI(t *testing.T) {
	// PRI=190 → facility=LOG_LOCAL7 (23), severity=SEV_INFO (6). Valid range.
	msg, err := Parse([]byte(`<190>Feb 13 21:31:56 74794bfb6795 liblogging-stdlog: start`))
	assert.NoError(t, err)
	assert.Equal(t, 190, msg.Pri)
	assert.Equal(t, "74794bfb6795", msg.Hostname)
	assert.Equal(t, "liblogging-stdlog", msg.AppName)
	assert.Equal(t, "start", string(msg.Msg))
}
