// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each sample below is taken from the log-pipeline test fixtures of the
// integration named in the case, i.e. these are formats real appliances emit.
// Before these layouts were recognized, every header field came back as the
// nilvalue and source routing on syslog.appname/hostname could not work.
func TestParseUnrecognizedTimestampLayouts(t *testing.T) {
	cases := []struct {
		name      string
		line      string
		timestamp string
		hostname  string
		appname   string
		procid    string
		msg       string
	}{
		{
			name:      "non-padded single digit day (beyondtrust privileged remote access)",
			line:      "<133>Jan 9 03:47:40 pf60fc91 BG[81869] 1427:01:01:event=fido2_credential_added",
			timestamp: "Jan 9 03:47:40",
			hostname:  "pf60fc91",
			appname:   "BG",
			procid:    "81869",
			msg:       "1427:01:01:event=fido2_credential_added",
		},
		{
			name:      "non-padded single digit day (infoblox ddi)",
			line:      "<30>Mar 7 08:32:59 infoblox.localdomain dhcpd[20397]: DHCPDECLINE of 1.1.1.1",
			timestamp: "Mar 7 08:32:59",
			hostname:  "infoblox.localdomain",
			appname:   "dhcpd",
			procid:    "20397",
			msg:       "DHCPDECLINE of 1.1.1.1",
		},
		{
			name:      "space padded day still parses (rfc 3164 conformant)",
			line:      "<133>Jan  9 03:47:40 pf60fc91 BG[81869] payload",
			timestamp: "Jan  9 03:47:40",
			hostname:  "pf60fc91",
			appname:   "BG",
			procid:    "81869",
			msg:       "payload",
		},
		{
			name:      "cisco nx-os year first header",
			line:      "<187>2024 Apr 04 08:05:06 MDS9148S-S4 %MODULE-5-ACTIVE_SUP_OK: Supervisor 1 is active",
			timestamp: "2024 Apr 04 08:05:06",
			hostname:  "MDS9148S-S4",
			appname:   nilvalue, // "%MODULE-..." is not a TAG
			procid:    nilvalue,
			msg:       "%MODULE-5-ACTIVE_SUP_OK: Supervisor 1 is active",
		},
		{
			name:      "cisco asa iso timestamp then colon, no host or tag",
			line:      "<190>2025-11-25T07:17:18Z: %ASA-4-733102: Threat-detection adds host 192.0.2.3 to shun list",
			timestamp: "2025-11-25T07:17:18Z",
			hostname:  nilvalue,
			appname:   nilvalue,
			procid:    nilvalue,
			msg:       "%ASA-4-733102: Threat-detection adds host 192.0.2.3 to shun list",
		},
		{
			name:      "cisco ftd iso timestamp then colon",
			line:      "<190>2025-12-05T10:51:21Z: %FTD-6-113039: Group RemoteUsers User harper.green IP 8.8.4.4",
			timestamp: "2025-12-05T10:51:21Z",
			hostname:  nilvalue,
			appname:   nilvalue,
			procid:    nilvalue,
			msg:       "%FTD-6-113039: Group RemoteUsers User harper.green IP 8.8.4.4",
		},
		{
			name:      "picus iso timestamp with host and tag",
			line:      `<6>2025-02-17T06:46:46Z app.picussecurity.com PICUS[1]: {"common":{"unique_id":"609249cb"}}`,
			timestamp: "2025-02-17T06:46:46Z",
			hostname:  "app.picussecurity.com",
			appname:   "PICUS",
			procid:    "1",
			msg:       `{"common":{"unique_id":"609249cb"}}`,
		},
		{
			name:      "iso timestamp with fractional seconds and host (delinea secret server)",
			line:      "2025-02-26T06:47:27.458Z DSS-01 CEF:0|Thycotic Software|Secret Server|11.7|10005|SECRET - EDIT|2|msg=x",
			timestamp: "2025-02-26T06:47:27.458Z",
			hostname:  "DSS-01",
			appname:   nilvalue, // CEF body is not a TAG
			procid:    nilvalue,
			msg:       "CEF:0|Thycotic Software|Secret Server|11.7|10005|SECRET - EDIT|2|msg=x",
		},
		{
			name:      "iso timestamp with numeric offset (ivanti connect secure)",
			line:      `<134>2024-12-04T01:18:01-05:00 test.com PulseSecure[7]: {"message_id":"WEB20174"}`,
			timestamp: "2024-12-04T01:18:01-05:00",
			hostname:  "test.com",
			appname:   "PulseSecure",
			procid:    "7",
			msg:       `{"message_id":"WEB20174"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var msg SyslogMessage
			var err error
			if tc.line[0] == '<' {
				msg, err = Parse([]byte(tc.line))
			} else {
				msg, err = ParseBSDLine([]byte(tc.line))
			}
			require.NoError(t, err)
			assert.Equal(t, tc.timestamp, msg.Timestamp, "timestamp")
			assert.Equal(t, tc.hostname, msg.Hostname, "hostname")
			assert.Equal(t, tc.appname, msg.AppName, "appname")
			assert.Equal(t, tc.procid, msg.ProcID, "procid")
			assert.Equal(t, tc.msg, string(msg.Msg), "msg")
		})
	}
}

// Cisco ISE repeats the BSD timestamp after the management IP. The month
// abbreviation of that second timestamp used to be extracted as the APP-NAME.
func TestParseCiscoISEDoubleBSDHeader(t *testing.T) {
	line := "<184>Apr 27 11:16:44 172.0.0.10 Apr 27 11:16:44 xyz12 CISE_Passed_Authentications 0000000487 1 0"
	msg, err := Parse([]byte(line))
	require.NoError(t, err)

	assert.Equal(t, "Apr 27 11:16:44", msg.Timestamp)
	assert.Equal(t, "172.0.0.10", msg.Hostname)
	assert.Equal(t, nilvalue, msg.AppName, "the second timestamp must not become the appname")
	assert.Equal(t, "Apr 27 11:16:44 xyz12 CISE_Passed_Authentications 0000000487 1 0", string(msg.Msg))
}

// Claroty CTD emits two spaces between the RFC 5424 VERSION and the TIMESTAMP.
func TestParseRFC5424ExtraSpaceAfterVersion(t *testing.T) {
	line := "<134>1  2025-05-13T04:57:18Z EMC syslog-healthcheck-Central - - - CEF:0|Claroty|CTD|5.0.1.0|HealthCheck|Health|0|k=v"
	msg, err := Parse([]byte(line))
	require.NoError(t, err)

	assert.Equal(t, "1", msg.Version)
	assert.Equal(t, "2025-05-13T04:57:18Z", msg.Timestamp)
	assert.Equal(t, "EMC", msg.Hostname)
	assert.Equal(t, "syslog-healthcheck-Central", msg.AppName)
	assert.Equal(t, nilvalue, msg.ProcID)
	assert.Equal(t, nilvalue, msg.MsgID)
	assert.Equal(t, "CEF:0|Claroty|CTD|5.0.1.0|HealthCheck|Health|0|k=v", string(msg.Msg))
}

func TestISOTimestampLen(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"2025-11-25T07:19:40Z", 20},
		{"2025-11-25T07:19:40Z rest", 20},
		{"2025-02-26T06:47:27.458Z", 24},
		{"2024-12-04T01:18:01-05:00", 25},
		{"2024-12-04T01:18:01-0500", 24},
		{"2024-12-04T01:18:01", 19},
		{"2024-12-04T01:18:01.", 19}, // dangling '.' is not fractional seconds
		{"2024-12-04 01:18:01", 0},   // space separator is not accepted
		{"2024-12-04T01:18", 0},      // too short
		{"2024 Apr 04 08:05:06", 0},  // year-first BSD, not ISO
		{"Jan  9 03:47:40", 0},       // BSD
		{"20251125T071940Z", 0},      // basic format not accepted
		{"2025-13-25T07:19:40Z", 20}, // structural check only, not calendar validation
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, isoTimestampLen([]byte(tc.in)), "input %q", tc.in)
	}
}

func TestBSDTimestampLen(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"Jan  9 03:47:40", 15},
		{"Jan 09 03:47:40", 15},
		{"Jan 9 03:47:40", 14},
		{"Apr 04 2024 08:05:06", 20},
		{"2024 Apr 04 08:05:06", 20},
		{"2024 Apr  4 08:05:06", 20},
		{"December sales were up", 0},
		{"Jan 9 03-47-40", 0},
		{"2024-04-04T08:05:06Z", 0},
		{"", 0},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, bsdTimestampLen([]byte(tc.in)), "input %q", tc.in)
	}
}
