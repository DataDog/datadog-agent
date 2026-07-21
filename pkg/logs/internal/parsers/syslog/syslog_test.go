// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	msg, err := Parse([]byte(`<34>1 2003-10-11T22:14:15.003Z mymachine.example.com su - ID47 - 'su root' failed for lonvick on /dev/pts/8`))
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
	assert.Equal(t, "'su root' failed for lonvick on /dev/pts/8", string(msg.Msg))
}

func TestParse_RFC5424_WithSD(t *testing.T) {
	msg, err := Parse([]byte(`<165>1 2003-10-11T22:14:15.003Z mymachine.example.com evntslog - ID47 [exampleSDID@32473 iut="3" eventSource="Application" eventID="1011"] An application event log entry...`))
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
	assert.Equal(t, "An application event log entry...", string(msg.Msg))
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

func TestParse_BSD_CEFNotConsumedAsTAG(t *testing.T) {
	// Per the CEF spec, "CEF:0|..." is the message body, not a TAG.
	// The parser must NOT split "CEF" into AppName when followed by ":digit".
	msg, err := Parse([]byte(`<14>May  5 14:55:43 PA-5260 CEF:0|Palo Alto Networks|PAN-OS|11.1.0|deny|TRAFFIC|1|src=10.0.0.1`))
	assert.NoError(t, err)
	assert.Equal(t, "PA-5260", msg.Hostname)
	assert.Equal(t, "-", msg.AppName, "CEF must not be parsed as AppName")
	assert.True(t, len(msg.Msg) > 0)
	assert.Equal(t, byte('C'), msg.Msg[0], "Msg must start with the CEF header")
}

func TestParse_BSD_LEEFNotConsumedAsTAG(t *testing.T) {
	msg, err := Parse([]byte(`<14>May  5 14:55:43 myhost LEEF:1.0|IBM|QRadar|1.0|12345|src=10.0.0.1`))
	assert.NoError(t, err)
	assert.Equal(t, "myhost", msg.Hostname)
	assert.Equal(t, "-", msg.AppName, "LEEF must not be parsed as AppName")
	assert.True(t, len(msg.Msg) > 0)
	assert.Equal(t, byte('L'), msg.Msg[0])
}

func TestParse_BSD_CEFWithSpaceIsTAG(t *testing.T) {
	// "CEF: CEF:0|..." — the first "CEF:" IS a valid TAG, followed by the body.
	msg, err := Parse([]byte(`<34>Oct 11 22:14:15 myhost CEF: CEF:0|Vendor|Product|1.0|200|Attack|8|src=1.2.3.4`))
	assert.NoError(t, err)
	assert.Equal(t, "myhost", msg.Hostname)
	assert.Equal(t, "CEF", msg.AppName)
	assert.Equal(t, "CEF:0|Vendor|Product|1.0|200|Attack|8|src=1.2.3.4", string(msg.Msg))
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
	msg, err := Parse([]byte("complete and utter gobbledegook"))
	assert.Error(t, err)
	assert.True(t, msg.Partial)
	assert.Equal(t, -1, msg.Pri)
	assert.Equal(t, "complete and utter gobbledegook", string(msg.Msg))
}

func TestParse_PRIOnly(t *testing.T) {
	msg, err := Parse([]byte(`<14>`))
	assert.Error(t, err)
	assert.True(t, msg.Partial)
	assert.Equal(t, 14, msg.Pri)
	assert.Equal(t, "<14>", string(msg.Msg))
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

func TestParseBSDLine_NonSyslogContent(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"plain text", "not syslog at all"},
		{"numeric prefix", "12345 some data"},
		{"json line", `{"level":"warn","msg":"oops"}`},
		{"error log", "ERROR 2025-01-15 connection refused"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseBSDLine([]byte(tt.input))
			assert.Error(t, err)
			assert.True(t, msg.Partial)
			assert.Equal(t, -1, msg.Pri)
			assert.Equal(t, tt.input, string(msg.Msg))
		})
	}
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

// ---------------------------------------------------------------------------
// Additional coverage: error paths, edge cases
// ---------------------------------------------------------------------------

func TestParse_PRIOutOfRange_BSD(t *testing.T) {
	// PRI > 191 combined with BSD body: both priErr and parse result preserved.
	msg, err := Parse([]byte(`<200>Feb 13 20:07:26 myhost sshd[123]: login`))
	assert.Error(t, err)
	assert.True(t, msg.Partial)
	assert.Equal(t, 200, msg.Pri)
	assert.Equal(t, "myhost", msg.Hostname)
	assert.Equal(t, "sshd", msg.AppName)
	assert.Contains(t, err.Error(), "PRI")
}

func TestParse_TruncatedHostname_BSD(t *testing.T) {
	// Timestamp present but line ends before hostname can be fully parsed.
	msg, err := Parse([]byte(`<13>Feb 13 20:07:26 `))
	assert.Error(t, err)
	assert.True(t, msg.Partial)
	assert.Equal(t, 13, msg.Pri)
}

func TestParse_EmptySDParamName(t *testing.T) {
	// SD with an empty param name: [id@1 ="val"]
	msg, err := Parse([]byte(`<14>1 - - - - - [id@1 ="val"] test`))
	// The parser should handle this gracefully (best-effort or error).
	if err != nil {
		assert.True(t, msg.Partial)
	}
	assert.Equal(t, 14, msg.Pri)
}

func TestParse_MultipleSD_ErrorInSecond(t *testing.T) {
	// First SD is valid, second is malformed (missing closing bracket).
	msg, err := Parse([]byte(`<14>1 - - - - - [ok@1 a="b"][bad@2 c="d" test`))
	assert.Error(t, err)
	assert.True(t, msg.Partial)
	assert.Equal(t, 14, msg.Pri)
	// First SD element should be preserved (best-effort partial return).
	require.NotNil(t, msg.StructuredData, "partial SD should be returned on error")
	params, ok := msg.StructuredData["ok@1"]
	assert.True(t, ok, "first SD element should be preserved")
	assert.Equal(t, "b", params["a"])
}

func TestParse_RFC5424_VersionErrors(t *testing.T) {
	// A digit run followed by SP looks like an RFC 5424 VERSION SP header, so
	// these route to parseRFC5424 and surface a precise, malformed-VERSION
	// error (rather than being silently reinterpreted as BSD content).
	tests := []struct {
		name  string
		input string
	}{
		{"version starts with 0", "<14>0 - - - - - - test"},
		{"version too long", "<14>1234 - - - - - - test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := Parse([]byte(tt.input))
			assert.Error(t, err)
			assert.True(t, msg.Partial)
			assert.Equal(t, 14, msg.Pri, "PRI should be preserved on version errors")
		})
	}

	// A leading digit that is NOT a "digit run + SP" is not an RFC 5424 header.
	// Per RFC 3164 §4.3.2 the remainder is MSG CONTENT (no partial error), which
	// is what makes PAN-OS/Cisco digit-prefixed CSV lines parse cleanly.
	notHeader := []struct {
		name  string
		input string
		msg   string
	}{
		{"digit then non-digit is section 4.3.2", "<14>1a - - - - - - test", "1a - - - - - - test"},
		{"lone digit, no space, is section 4.3.2", "<14>1", "1"},
	}
	for _, tt := range notHeader {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := Parse([]byte(tt.input))
			require.NoError(t, err)
			assert.False(t, msg.Partial)
			assert.Equal(t, 14, msg.Pri)
			assert.Equal(t, tt.msg, string(msg.Msg))
			assert.Equal(t, nilvalue, msg.Timestamp)
			assert.Equal(t, nilvalue, msg.AppName)
		})
	}

	// Space after PRI (previously "empty version") is now a valid §4.3.2
	// message: PRI extracted, remainder is MSG CONTENT.
	t.Run("space after PRI is section 4.3.2", func(t *testing.T) {
		msg, err := Parse([]byte("<14> - - - - - - test"))
		require.NoError(t, err)
		assert.Equal(t, 14, msg.Pri)
		assert.Equal(t, " - - - - - - test", string(msg.Msg))
		assert.False(t, msg.Partial)
	})
}

func TestParseBSDLine_EmptyInput_Pri(t *testing.T) {
	msg, err := ParseBSDLine([]byte{})
	assert.Error(t, err)
	assert.Equal(t, -1, msg.Pri, "empty input should yield Pri=-1")
}

func TestParse_Empty_Pri(t *testing.T) {
	msg, err := Parse([]byte{})
	assert.Error(t, err)
	assert.Equal(t, -1, msg.Pri, "empty input should yield Pri=-1")
}

func TestParse_NoPRI_Pri(t *testing.T) {
	msg, err := Parse([]byte("no angle bracket at all"))
	assert.Error(t, err)
	assert.Equal(t, -1, msg.Pri, "missing PRI should yield Pri=-1")
	assert.True(t, msg.Partial)
}

func TestSeverityToStatus_NegativePri(t *testing.T) {
	assert.Equal(t, message.StatusInfo, SeverityToStatus(-1))
	assert.Equal(t, message.StatusInfo, SeverityToStatus(-999))
}

func TestParse_SDName_ControlCharsRejected(t *testing.T) {
	// PRINTUSASCII is %d33-126; control chars and DEL must be rejected.
	// Control char in SD-ID:
	msg, err := Parse([]byte("<14>1 - - - - - [\x01bad@1 a=\"b\"] test"))
	assert.Error(t, err)
	assert.True(t, msg.Partial)

	// DEL (127) in PARAM-NAME:
	msg, err = Parse([]byte("<14>1 - - - - - [ok@1 \x7Fname=\"b\"] test"))
	assert.Error(t, err)
	assert.True(t, msg.Partial)

	// High byte (128+) in SD-ID:
	msg, err = Parse([]byte("<14>1 - - - - - [\x80bad@1 a=\"b\"] test"))
	assert.Error(t, err)
	assert.True(t, msg.Partial)
}

func TestParse_BOM_InMSG_RFC5424(t *testing.T) {
	// RFC 5424 §6.4: MSG may start with a BOM; the parser strips it.
	msg, err := Parse([]byte("<14>1 - - - - - - \xEF\xBB\xBFhello"))
	assert.NoError(t, err)
	assert.Equal(t, "hello", string(msg.Msg))
}

func TestParse_BOM_InMSG_BSD(t *testing.T) {
	// BSD MSG does not have BOM stripping — BOM is preserved verbatim.
	// This documents the intentional asymmetry with RFC 5424.
	msg, err := Parse([]byte("<13>Feb 13 20:07:26 myhost sshd[123]: \xEF\xBB\xBFlogin"))
	assert.NoError(t, err)
	assert.Equal(t, "myhost", msg.Hostname)
	assert.Equal(t, "\xEF\xBB\xBFlogin", string(msg.Msg))
}

func TestParse_BOM_BeforePRI(t *testing.T) {
	// BOM before PRI is not handled — Parse() sees 0xEF as the first byte,
	// not '<'. This documents the known limitation.
	input := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`<13>Feb 13 20:07:26 myhost sshd: login`)...)
	_, err := Parse(input)
	assert.Error(t, err, "BOM before PRI is not supported")
}

func TestParse_DuplicateSDParamName(t *testing.T) {
	// Duplicate param names within the same SD-ELEMENT: last write wins.
	msg, err := Parse([]byte(`<14>1 - - - - - [id@1 key="first" key="second"] test`))
	assert.NoError(t, err)
	require.NotNil(t, msg.StructuredData)
	params := msg.StructuredData["id@1"]
	assert.Equal(t, "second", params["key"], "last-write-wins for duplicate param names")
}

func TestParse_VersionZero(t *testing.T) {
	// VERSION starting with '0' should route to parseRFC5424 and return
	// errVersionStart (not errUnknownFormat).
	msg, err := Parse([]byte(`<14>0 - - - - - - test`))
	assert.Error(t, err)
	assert.True(t, msg.Partial)
	assert.Equal(t, 14, msg.Pri, "PRI should be preserved")
	assert.Contains(t, err.Error(), "VERSION", "error should mention VERSION, not unknown format")
}

func TestParse_FalsePositiveBSD(t *testing.T) {
	// A line starting with a month name but without valid BSD timestamp
	// structure. Per RFC 3164 §4.3.2 this is a valid-PRI-no-timestamp
	// message: PRI is extracted, remainder is MSG CONTENT.
	msg, err := Parse([]byte(`<14>December sales report for Q4`))
	require.NoError(t, err)
	assert.False(t, msg.Partial)
	assert.Equal(t, 14, msg.Pri)
	assert.Equal(t, "December sales report for Q4", string(msg.Msg))
	assert.Equal(t, nilvalue, msg.Timestamp)
	assert.Equal(t, nilvalue, msg.AppName)
}

// ---------------------------------------------------------------------------
// Benchmarks
// ---------------------------------------------------------------------------

func BenchmarkParse_RFC5424_NoSD(b *testing.B) {
	input := []byte(`<165>1 2003-10-11T22:14:15.003Z mymachine.example.com evntslog - ID47 - An application event log entry`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Parse(input) //nolint:errcheck
	}
}

func BenchmarkParse_RFC5424_WithSD(b *testing.B) {
	input := []byte(`<165>1 2003-10-11T22:14:15.003Z mymachine.example.com evntslog - ID47 [exampleSDID@32473 iut="3" eventSource="Application" eventID="1011"] An application event log entry`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Parse(input) //nolint:errcheck
	}
}

// ---------------------------------------------------------------------------
// RFC 3164 §4.3.2: Valid PRI, no valid TIMESTAMP
// ---------------------------------------------------------------------------

func TestParseBSDNoTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		pri     int
		msg     string
		wantErr bool
	}{
		{
			name:  "Python SysLogHandler TCP (warning)",
			input: "<132>Python SysLogHandler TCP test",
			pri:   132,
			msg:   "Python SysLogHandler TCP test",
		},
		{
			name:  "Python SysLogHandler UDP (info)",
			input: "<134>Python SysLogHandler UDP test",
			pri:   134,
			msg:   "Python SysLogHandler UDP test",
		},
		{
			name:  "BSD with ident prefix",
			input: "<134>myapp: Hello world",
			pri:   134,
			msg:   "myapp: Hello world",
		},
		{
			name:  "minimal content (1 byte)",
			input: "<13>x",
			pri:   13,
			msg:   "x",
		},
		{
			name:  "content with leading space",
			input: "<134> message with leading space",
			pri:   134,
			msg:   " message with leading space",
		},
		{
			name:  "content too short for timestamp",
			input: "<134>short",
			pri:   134,
			msg:   "short",
		},
		{
			// PAN-OS / Cisco CSV dialect: valid PRI, content begins with a
			// digit but is NOT an RFC 5424 VERSION SP token. It must be treated
			// as no-timestamp MSG CONTENT, not forced through the RFC 5424
			// parser (which previously produced a partial parse).
			name:  "PAN-OS CSV digit-prefixed content",
			input: "<134>1,2026/06/23 10:15:30,001801000000,TRAFFIC,end,2560",
			pri:   134,
			msg:   "1,2026/06/23 10:15:30,001801000000,TRAFFIC,end,2560",
		},
		{
			// Multi-digit prefix (e.g. sequence number) followed by a CSV
			// delimiter is content, not a 1-3 digit VERSION SP header.
			name:  "digit run then comma is content",
			input: "<13>12,alpha,beta",
			pri:   13,
			msg:   "12,alpha,beta",
		},
		{
			name:    "empty after PRI remains error",
			input:   "<14>",
			pri:     14,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := Parse([]byte(tt.input))

			if tt.wantErr {
				assert.Error(t, err)
				assert.True(t, parsed.Partial)
				return
			}

			require.NoError(t, err, "RFC 3164 §4.3.2 messages should parse successfully")
			assert.Equal(t, tt.pri, parsed.Pri)
			assert.Equal(t, tt.msg, string(parsed.Msg))
			assert.False(t, parsed.Partial)

			// §4.3.2: TIMESTAMP, HOSTNAME, TAG all indeterminate
			assert.Equal(t, nilvalue, parsed.Timestamp)
			assert.Equal(t, nilvalue, parsed.Hostname)
			assert.Equal(t, nilvalue, parsed.AppName)
			assert.Equal(t, nilvalue, parsed.ProcID)
			assert.Equal(t, nilvalue, parsed.MsgID)
		})
	}
}

func TestParse_BSD_SingleLetterTag(t *testing.T) {
	// A single-letter program name (e.g. "q") is unusual but valid. It must be
	// recognized as the TAG rather than discarded as a numeric/data fragment.
	msg, err := Parse([]byte(`<13>Feb 13 20:07:26 myhost q: short tag message`))
	assert.NoError(t, err)
	assert.Equal(t, "myhost", msg.Hostname)
	assert.Equal(t, "q", msg.AppName)
	assert.Equal(t, "short tag message", string(msg.Msg))
}

func TestParse_BSD_SingleDigitTagRejected(t *testing.T) {
	// A single *digit* in the TAG position is a data fragment (e.g. PAN-OS
	// FUTURE_USE), not a program name, and must remain part of MSG.
	msg, err := Parse([]byte(`<13>Feb 13 20:07:26 myhost 1 firewall event`))
	assert.NoError(t, err)
	assert.Equal(t, "myhost", msg.Hostname)
	assert.Equal(t, nilvalue, msg.AppName)
	assert.Equal(t, "1 firewall event", string(msg.Msg))
}

func BenchmarkParse_BSD(b *testing.B) {
	input := []byte(`<13>Feb 13 20:07:26 74794bfb6795 root[8539]: i am foobar`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Parse(input) //nolint:errcheck
	}
}

func BenchmarkParseBSDLine(b *testing.B) {
	input := []byte(`Dec 28 16:49:07 plertrood-thinkpad-x220 nginx: 127.0.0.1 - - request`)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseBSDLine(input) //nolint:errcheck
	}
}

// ---------------------------------------------------------------------------
// BSD 20-byte "with-year" TIMESTAMP variant: "Mmm DD YYYY HH:MM:SS"
// ---------------------------------------------------------------------------

func TestParse_BSD_TimestampWithYear(t *testing.T) {
	// Some network appliances insert a 4-digit year between the day and time.
	// The 20-byte variant must be accepted, with HOSTNAME and TAG still parsed.
	msg, err := Parse([]byte(`<13>May 04 2026 21:09:42 myhost myapp: hello world`))
	require.NoError(t, err)
	assert.False(t, msg.Partial)
	assert.Equal(t, "May 04 2026 21:09:42", msg.Timestamp)
	assert.Equal(t, "myhost", msg.Hostname)
	assert.Equal(t, "myapp", msg.AppName)
	assert.Equal(t, "hello world", string(msg.Msg))
}

func TestParse_BSD_TimestampWithYear_Malformed(t *testing.T) {
	// A 20-byte-shaped prefix that fails structural validation is not a valid
	// timestamp. With a valid PRI, RFC 3164 §4.3.2 applies and the whole
	// remainder becomes MSG CONTENT (no error, not partial).
	msg, err := Parse([]byte(`<13>May 04 2026-21:09:42 myhost data`))
	require.NoError(t, err)
	assert.False(t, msg.Partial)
	assert.Equal(t, nilvalue, msg.Timestamp)
	assert.Equal(t, "May 04 2026-21:09:42 myhost data", string(msg.Msg))
}

func TestIsValidBSDTimestampWithYear(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"valid", "May 04 2026 21:09:42", true},
		{"valid december", "Dec 31 1999 00:00:00", true},
		{"too short", "May 04 2026 21:09:4", false},
		{"bad month", "Foo 04 2026 21:09:42", false},
		{"missing space after day", "May 04X2026 21:09:42", false},
		{"missing space after year", "May 04 2026-21:09:42", false},
		{"non-digit year", "May 04 20X6 21:09:42", false},
		{"bad first colon", "May 04 2026 21-09:42", false},
		{"bad second colon", "May 04 2026 21:09-42", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isValidBSDTimestampWithYear([]byte(tt.in)))
		})
	}
}

// ---------------------------------------------------------------------------
// BSD "double-header": ISO 8601 timestamp in the TAG position
// ---------------------------------------------------------------------------

func TestParse_BSD_DoubleHeaderISO(t *testing.T) {
	// Cisco FTD and similar devices embed a second, ISO 8601 timestamp after
	// the BSD hostname. No real TAG is present; the whole remainder is MSG.
	msg, err := Parse([]byte(`<134>May  5 14:55:43 fw01 2026-07-09T14:55:43Z realhost %ASA-6-302013: Built connection`))
	require.NoError(t, err)
	assert.Equal(t, "May  5 14:55:43", msg.Timestamp)
	assert.Equal(t, "fw01", msg.Hostname)
	assert.Equal(t, nilvalue, msg.AppName, "ISO timestamp must not be parsed as a TAG")
	assert.Equal(t, "2026-07-09T14:55:43Z realhost %ASA-6-302013: Built connection", string(msg.Msg))
}

func TestLooksLikeISOTimestamp(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"iso datetime", "2026-07-09T14:55:43Z rest", true},
		{"iso date minimal", "2026-", true},
		{"too short", "2026", false},
		{"not a year", "20a6-07-09", false},
		{"no dash", "20260709T00", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, looksLikeISOTimestamp([]byte(tt.in)))
		})
	}
}

func TestParse_BSD_ImplausibleTag_NoDelimiter(t *testing.T) {
	// After a valid BSD timestamp and hostname, a delimiter-free remainder that
	// is not a plausible program name must stay in MSG rather than becoming the
	// AppName (e.g. a bare year or a CSV fragment).
	tests := []struct {
		name string
		in   string
		msg  string
	}{
		{"all-digit fragment", "<13>Feb 13 20:07:26 myhost 2026", "2026"},
		{"csv fragment", "<13>Feb 13 20:07:26 myhost 1,2,3", "1,2,3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := Parse([]byte(tt.in))
			require.NoError(t, err)
			assert.Equal(t, "myhost", msg.Hostname)
			assert.Equal(t, nilvalue, msg.AppName)
			assert.Equal(t, tt.msg, string(msg.Msg))
		})
	}
}
