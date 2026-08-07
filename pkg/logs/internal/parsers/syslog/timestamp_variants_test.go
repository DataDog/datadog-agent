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

// Each sample below is quoted from the log-pipeline test fixtures of the
// integration named in the case, except where a comment says otherwise, so these
// are formats real appliances emit. A layout that goes unrecognized costs the
// message every header field — they all come back as the nilvalue — and source
// routing on syslog.appname/hostname stops working.
//
// A sample carrying a PRI was captured off the wire and goes through Parse; one
// without a PRI is a file rendering and goes through ParseBSDLine. That says
// nothing about the sender: a PRI is mandatory on the wire, but the default file
// templates of both rsyslog and syslog-ng omit it.
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
			// Header and hostname as printed in Cisco's Nexus 9000 system message
			// logging guide, which shows this PRI-less form as what NX-OS writes
			// to its own logfile and console, separately from the wire format
			// below.
			name:      "cisco nx-os year first header, logfile form",
			line:      "2019 Mar 11 13:42:44 Cisco-customer %ETHPORT-5-IF_DOWN_ADMIN_DOWN: Interface Ethernet3/1 is down (Administratively down)",
			timestamp: "2019 Mar 11 13:42:44",
			hostname:  "Cisco-customer",
			appname:   nilvalue, // "%ETHPORT-..." is not a TAG
			procid:    nilvalue,
			msg:       "%ETHPORT-5-IF_DOWN_ADMIN_DOWN: Interface Ethernet3/1 is down (Administratively down)",
		},
		{
			// Header shape from Cisco's MDS 9000 system management guide, which
			// documents "<189>:2025 Mar 27 16:22:24 switch %SYSLOG-..." as the
			// default format sent to a remote server. Note the colon after PRI.
			name:      "cisco nx-os year first header, remote logging wire format",
			line:      "<189>:2019 Mar 11 13:42:44 Cisco-customer %ETHPORT-5-IF_DOWN_ADMIN_DOWN: Interface Ethernet3/1 is down (Administratively down)",
			timestamp: "2019 Mar 11 13:42:44",
			hostname:  "Cisco-customer",
			appname:   nilvalue,
			procid:    nilvalue,
			msg:       "%ETHPORT-5-IF_DOWN_ADMIN_DOWN: Interface Ethernet3/1 is down (Administratively down)",
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
			// The ISO variant with a full HOSTNAME and a TAG[PID], which several
			// appliances emit under an rfc5424-style timestamp setting. Values
			// are synthetic; only the header shape is under test.
			name:      "iso timestamp with host and tag",
			line:      `<6>2025-02-17T06:46:46Z app.example.com APPLIANCE[1]: {"common":{"unique_id":"609249cb"}}`,
			timestamp: "2025-02-17T06:46:46Z",
			hostname:  "app.example.com",
			appname:   "APPLIANCE",
			procid:    "1",
			msg:       `{"common":{"unique_id":"609249cb"}}`,
		},
		{
			// Delinea documents the ISO timestamp and the CEF payload but not the
			// framing; the fixture has no PRI, which is what rsyslog's file
			// template produces, so read this as the tailed-file form.
			name:      "iso timestamp with fractional seconds, no pri (delinea secret server)",
			line:      "2025-02-26T06:47:27.458Z DSS-01 CEF:0|Thycotic Software|Secret Server|11.7|10005|SECRET - EDIT|2|msg=x",
			timestamp: "2025-02-26T06:47:27.458Z",
			hostname:  "DSS-01",
			appname:   nilvalue, // CEF body is not a TAG
			procid:    nilvalue,
			msg:       "CEF:0|Thycotic Software|Secret Server|11.7|10005|SECRET - EDIT|2|msg=x",
		},
		{
			// Not every ASA emits "Z": this offset form is from a reported
			// capture of an ASA running "logging timestamp rfc5424".
			name:      "cisco asa iso timestamp with numeric offset",
			line:      "<190>2020-07-16T09:39:32+02:00: %ASA-6-110002: Failed to locate egress interface for 192.0.2.1/443",
			timestamp: "2020-07-16T09:39:32+02:00",
			hostname:  nilvalue,
			appname:   nilvalue,
			procid:    nilvalue,
			msg:       "%ASA-6-110002: Failed to locate egress interface for 192.0.2.1/443",
		},
		{
			// Cisco's EMBLEM format puts a colon after the PRI as well. Documented
			// in the ASA syslog guide as "<166>:2018-06-27T12:17:46Z: %ASA-...".
			name:      "cisco asa emblem, colon after pri",
			line:      "<166>:2018-06-27T12:17:46Z: %ASA-6-110002: Failed to locate egress interface for 192.0.2.1/443",
			timestamp: "2018-06-27T12:17:46Z",
			hostname:  nilvalue,
			appname:   nilvalue,
			procid:    nilvalue,
			msg:       "%ASA-6-110002: Failed to locate egress interface for 192.0.2.1/443",
		},
		{
			// "logging device-id" is off, so the Device-ID field is empty and only
			// whitespace separates the timestamp from the mnemonic. Cisco's syslog
			// guide tells collectors to allow an optional colon here "followed by
			// zero or more spaces", so the gap width carries no meaning. Sample is
			// from the cisco_secure_firewall pipeline fixtures.
			name:      "cisco ftd no device id, padded gap before mnemonic",
			line:      "2024-01-05T05:45:16Z   %FTD-1-430003: EventPriority: Low, SrcIP: 10.40.1.16",
			timestamp: "2024-01-05T05:45:16Z",
			hostname:  nilvalue,
			appname:   nilvalue,
			procid:    nilvalue,
			msg:       "%FTD-1-430003: EventPriority: Low, SrcIP: 10.40.1.16",
		},
		{
			name:      "cisco ftd no device id, single space before mnemonic",
			line:      "<181>2024-01-05T05:45:16Z %FTD-1-430003: EventPriority: Low",
			timestamp: "2024-01-05T05:45:16Z",
			hostname:  nilvalue,
			appname:   nilvalue,
			procid:    nilvalue,
			msg:       "%FTD-1-430003: EventPriority: Low",
		},
		{
			// FTD 7.0.5+/7.2+ add a colon between the (absent) Device-ID and the
			// mnemonic.
			name:      "cisco ftd no device id, colon before mnemonic",
			line:      "2024-01-05T05:45:16Z : %FTD-1-430003: EventPriority: Low",
			timestamp: "2024-01-05T05:45:16Z",
			hostname:  nilvalue,
			appname:   nilvalue,
			procid:    nilvalue,
			msg:       "%FTD-1-430003: EventPriority: Low",
		},
		{
			// ASA under "logging timestamp" writes the legacy MMM dd yyyy
			// HH:mm:ss form and terminates it with a colon, with no Device-ID,
			// so separator handling has to cover the BSD layouts too, not only
			// the ISO timestamp the same appliance emits under rfc5424.
			// Message body follows the 302013 template in Cisco's ASA syslog
			// messages guide, with documentation addresses.
			name:      "cisco asa legacy timestamp then colon, no host or tag",
			line:      "<166>May 17 2023 03:04:28: %ASA-6-302013: Built outbound TCP connection 1 for outside:192.0.2.1/80 to inside:198.51.100.2/1024",
			timestamp: "May 17 2023 03:04:28",
			hostname:  nilvalue,
			appname:   nilvalue,
			procid:    nilvalue,
			msg:       "%ASA-6-302013: Built outbound TCP connection 1 for outside:192.0.2.1/80 to inside:198.51.100.2/1024",
		},
		{
			// Legacy timestamp with the Device-ID still enabled, quoted from the
			// FTD syslog guide. The Device-ID is a real hostname and must be read
			// as one; only the separator that stands in for an absent Device-ID
			// is consumed, so the " : " that follows a present one stays in MSG.
			name:      "cisco ftd with device id keeps hostname",
			line:      "Apr 10 18:52:47 labuser-ftdv : %FTD-6-305012: Teardown dynamic UDP translation",
			timestamp: "Apr 10 18:52:47",
			hostname:  "labuser-ftdv",
			appname:   nilvalue,
			procid:    nilvalue,
			msg:       ": %FTD-6-305012: Teardown dynamic UDP translation",
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

// The colon ASA writes after an ISO timestamp is skipped whatever follows it,
// not only a Cisco mnemonic. Every ASA and FTD sample happens to carry one, so
// nothing else in the suite covers the general case: drop this handling and a
// body the mnemonic check does not recognize is lost along with the message.
func TestISOTimestampColonWithoutMnemonic(t *testing.T) {
	cases := []struct {
		name string
		line string
		msg  string
	}{
		{
			name: "space after the colon",
			line: "<190>2025-11-25T07:19:40Z: connection teardown, no mnemonic",
			msg:  "connection teardown, no mnemonic",
		},
		{
			name: "no space after the colon",
			line: "<190>2025-11-25T07:19:40Z:connection teardown, no mnemonic",
			msg:  "connection teardown, no mnemonic",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := Parse([]byte(tc.line))
			require.NoError(t, err)

			assert.Equal(t, "2025-11-25T07:19:40Z", msg.Timestamp)
			assert.Equal(t, nilvalue, msg.Hostname)
			assert.Equal(t, nilvalue, msg.AppName)
			assert.Equal(t, tc.msg, string(msg.Msg), "the body must reach MSG")
		})
	}
}

// A colon after the PRI is only skipped when a timestamp follows it, so content
// that merely starts with a colon keeps its leading colon in MSG.
func TestColonAfterPRIWithoutTimestamp(t *testing.T) {
	msg, err := Parse([]byte("<134>: not a timestamp"))
	require.NoError(t, err)

	assert.Equal(t, nilvalue, msg.Timestamp)
	assert.Equal(t, ": not a timestamp", string(msg.Msg))
}

// Formats that place something other than a HOSTNAME in the HOSTNAME position.
// Each of these parses without error, so a misattribution here is worse than a
// failure: syslog.hostname and syslog.appname drive source and service routing,
// and a bogus value misroutes the log with nothing to signal it.
func TestNoHostnameInHostnamePosition(t *testing.T) {
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
			// BIND with print-category/print-severity emits
			// "<category>: <severity>: <message>" and no hostname at all, so
			// the category is the TAG and the severity belongs to the body.
			name:      "bind9 category in hostname position",
			line:      "2024-09-20T10:20:26.751Z network: info: no longer listening on 10.10.10.10#50",
			timestamp: "2024-09-20T10:20:26.751Z",
			hostname:  nilvalue,
			appname:   "network",
			procid:    nilvalue,
			msg:       "info: no longer listening on 10.10.10.10#50",
		},
		{
			// A yum transaction line tailed by Wazuh: the action sits in the
			// HOSTNAME position and the package name is the whole body.
			name:      "yum action in hostname position",
			line:      "Aug 20 12:51:21 Erased: libhugetlbfs-lib",
			timestamp: "Aug 20 12:51:21",
			hostname:  nilvalue,
			appname:   "Erased",
			procid:    nilvalue,
			msg:       "libhugetlbfs-lib",
		},
		{
			name:      "tag with pid in hostname position",
			line:      "Aug 20 12:51:21 sshd[1234]: Accepted publickey for root",
			timestamp: "Aug 20 12:51:21",
			hostname:  nilvalue,
			appname:   "sshd",
			procid:    "1234",
			msg:       "Accepted publickey for root",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := ParseBSDLine([]byte(tc.line))
			require.NoError(t, err)
			assert.Equal(t, tc.timestamp, msg.Timestamp, "timestamp")
			assert.Equal(t, tc.hostname, msg.Hostname, "hostname")
			assert.Equal(t, tc.appname, msg.AppName, "appname")
			assert.Equal(t, tc.procid, msg.ProcID, "procid")
			assert.Equal(t, tc.msg, string(msg.Msg), "msg")
		})
	}
}

// A relay that prepends its own header leaves the originating device's header
// in place. The second timestamp sits where a TAG would be, and must not be
// mined for an APP-NAME.
func TestRelayDoubleHeader(t *testing.T) {
	t.Run("bsd timestamp in tag position", func(t *testing.T) {
		line := "May 31 14:06:58 10.50.3.134 May 31 08:36:58 ise CISE_System_Statistics 0000000718"
		msg, err := ParseBSDLine([]byte(line))
		require.NoError(t, err)

		assert.Equal(t, "May 31 14:06:58", msg.Timestamp)
		assert.Equal(t, "10.50.3.134", msg.Hostname)
		assert.Equal(t, nilvalue, msg.AppName, "the month abbrev is not an appname")
		assert.Equal(t, "May 31 08:36:58 ise CISE_System_Statistics 0000000718", string(msg.Msg))
	})

	t.Run("bsd timestamp in tag position after year-first header", func(t *testing.T) {
		line := "2019 Mar 11 13:42:44 Cisco-customer Mar 11 13:42:44 %KERN-3-SYSTEM_MSG: TOTAL PORTS = 48"
		msg, err := ParseBSDLine([]byte(line))
		require.NoError(t, err)

		assert.Equal(t, "2019 Mar 11 13:42:44", msg.Timestamp)
		assert.Equal(t, "Cisco-customer", msg.Hostname)
		assert.Equal(t, nilvalue, msg.AppName)
		assert.Equal(t, "Mar 11 13:42:44 %KERN-3-SYSTEM_MSG: TOTAL PORTS = 48", string(msg.Msg))
	})
}

// RFC 5424 mandates a single SP between HEADER fields. Padding must not shift
// the fields along, leaving TIMESTAMP empty and moving every other value one
// position to the right.
func TestRFC5424PaddedHeaderSeparators(t *testing.T) {
	line := "<134>1  2025-05-13T04:57:18Z host01 syslog-healthcheck - - - CEF:0|Vendor|Product|1.0|HealthCheck"
	msg, err := Parse([]byte(line))
	require.NoError(t, err)

	assert.Equal(t, 134, msg.Pri)
	assert.Equal(t, "1", msg.Version)
	assert.Equal(t, "2025-05-13T04:57:18Z", msg.Timestamp)
	assert.Equal(t, "host01", msg.Hostname)
	assert.Equal(t, "syslog-healthcheck", msg.AppName)
	assert.Equal(t, nilvalue, msg.ProcID)
	assert.Equal(t, nilvalue, msg.MsgID)
	assert.Equal(t, "CEF:0|Vendor|Product|1.0|HealthCheck", string(msg.Msg))
}

// A TAG is only recognized by its terminator. Without one there is no TAG, and
// the remainder must reach MSG rather than being claimed as an APP-NAME and
// silently dropped.
func TestDelimiterlessTagIsContent(t *testing.T) {
	msg, err := ParseBSDLine([]byte("Aug 20 12:51:21 myhost libhugetlbfs-lib"))
	require.NoError(t, err)

	assert.Equal(t, "myhost", msg.Hostname)
	assert.Equal(t, nilvalue, msg.AppName)
	assert.Equal(t, "libhugetlbfs-lib", string(msg.Msg))
}

// An IPv6 address may end in "::", which must not be mistaken for a TAG
// terminator and cost the message its HOSTNAME.
func TestIPv6HostnameIsNotATag(t *testing.T) {
	msg, err := ParseBSDLine([]byte("Aug 20 12:51:21 2001:db8:: sshd[1]: payload"))
	require.NoError(t, err)

	assert.Equal(t, "2001:db8::", msg.Hostname)
	assert.Equal(t, "sshd", msg.AppName)
	assert.Equal(t, "1", msg.ProcID)
	assert.Equal(t, "payload", string(msg.Msg))
}

// Skipping the gap before a Cisco mnemonic must not turn into a general
// tolerance for ragged whitespace: anything that is not a mnemonic is read as
// a HOSTNAME, and a run of spaces followed by a non-mnemonic is still an error.
func TestCiscoMnemonicDoesNotOverCapture(t *testing.T) {
	t.Run("padded gap before a non-mnemonic still fails", func(t *testing.T) {
		_, err := ParseBSDLine([]byte("2024-01-05T05:45:16Z   somehost app: payload"))
		assert.ErrorIs(t, err, errBSDHostname)
	})

	t.Run("token starting with percent but not a mnemonic is a hostname", func(t *testing.T) {
		msg, err := ParseBSDLine([]byte("2024-01-05T05:45:16Z %notamnemonic payload"))
		require.NoError(t, err)
		assert.Equal(t, "%notamnemonic", msg.Hostname)
	})

	t.Run("ordinary hostname is untouched", func(t *testing.T) {
		msg, err := ParseBSDLine([]byte("2024-01-05T05:45:16Z firepower sshd[42]: payload"))
		require.NoError(t, err)
		assert.Equal(t, "firepower", msg.Hostname)
		assert.Equal(t, "sshd", msg.AppName)
		assert.Equal(t, "42", msg.ProcID)
		assert.Equal(t, "payload", string(msg.Msg))
	})
}

func TestStartsWithCiscoMnemonic(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"%FTD-1-430003: EventPriority: Low", true},
		{"%ASA-6-302013: Built outbound TCP connection", true},
		{"%MODULE-5-ACTIVE_SUP_OK: Supervisor 1 is active", true},
		{"%ETHPORT-5-IF_DOWN_CFG_CHANGE: Interface down", true},
		{"%FTD-1-430003", true},  // trailing colon is not required
		{"%FTD-8-430003", false}, // severity is a single 0-7 digit
		{"%FTD--430003", false},  // severity missing
		{"%FTD-1-", false},       // mnemonic missing
		{"%FTD-1", false},        // truncated
		{"%FTD", false},
		{"%-1-430003", false}, // facility missing
		{"%notamnemonic payload", false},
		{"firepower : %FTD-6-110002:", false}, // must be at the start
		{"FTD-1-430003:", false},              // no leading '%'
		{"%", false},
		{"", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, startsWithCiscoMnemonic([]byte(tc.in)), "input %q", tc.in)
	}
}

func TestSkipCiscoSeparator(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"   %FTD", 3},
		{" %FTD", 1},
		{" : %FTD", 3},
		{"  :  %FTD", 5},
		{" :%FTD", 2},
		{":%FTD", 1}, // ASA writes the colon with no preceding space
		{": %FTD", 2},
		{"%FTD", 0},   // neither space nor colon, nothing consumed
		{"::%FTD", 1}, // only one colon belongs to the separator
		{"", 0},
		{"   ", 3},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, skipCiscoSeparator([]byte(tc.in), 0), "input %q", tc.in)
	}
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
