// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package framer

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A line that merely begins with digits and a space must not be taken for an
// RFC 6587 octet-counted frame. Consuming the digit run as a MSG-LEN would let
// the declared body swallow every following frame, because MSG-LEN is the
// authoritative boundary and the body is never re-scanned for frame starts.
func TestSyslogDigitPrefixIsNotAlwaysOctetCount(t *testing.T) {
	good := "<134>Feb 10 12:00:00 flushhost FLUSHTAG[1]: well_formed_message"

	cases := []struct {
		name string
		line string
	}{
		{
			// Cisco NX-OS year-first header as rendered without a PRI, the form a
			// file template produces; "2019 " would be read as MSG-LEN=2019.
			name: "cisco nx-os year first, no pri",
			line: "2019 Mar 11 13:42:44 Cisco-customer %ETHPORT-5-IF_DOWN_ADMIN_DOWN: Interface Ethernet3/1 is down",
		},
		{
			name: "barracuda secure edge year first",
			line: "2025 09 26 22:28:00 +05:30 Info     : pam_unix(sudo:session): session opened",
		},
		{
			name: "mysql error log",
			line: "171113 14:14:20  InnoDB: Shutdown completed; log sequence number 1595675",
		},
		{
			name: "java thread prefix",
			line: "54 [main] INFO MyApp.foo.bar - Entering application.",
		},
		{
			name: "epoch prefix",
			line: "1726124251      0 10.10.10.10 TCP_DENIED/403 3920 CONNECT example.com:443 -",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stream := tc.line + "\n" + good + "\n"
			got, _ := processSyslog(t, 262144, [][]byte{[]byte(stream)})

			require.Len(t, got, 2, "each line is framed on its LF delimiter")
			assert.Equal(t, tc.line, got[0], "content preserved, no length prefix stripped")
			assert.Equal(t, good, got[1], "the following frame is not swallowed")
		})
	}

	t.Run("many following frames survive", func(t *testing.T) {
		// Read as a MSG-LEN, "2019" declares a body long enough to absorb dozens
		// of frames, so the count here is what proves none were absorbed.
		nxos := "2019 Mar 11 13:42:44 Cisco-customer %ETHPORT-5-IF_DOWN_ADMIN_DOWN: Interface Ethernet3/1 is down"
		stream := nxos + "\n" + strings.Repeat(good+"\n", 40)
		got, _ := processSyslog(t, 262144, [][]byte{[]byte(stream)})

		require.Len(t, got, 41)
		assert.Equal(t, nxos, got[0])
		for _, frame := range got[1:] {
			assert.Equal(t, good, frame)
		}
	})
}

// Digits and a space followed by "<" and a digit are still not an octet count
// unless the PRI is complete. Prose supplies that shape readily — "<1 minute",
// "<2 sec" — and without the closing ">" the digits ahead of it are read as a
// MSG-LEN whose body runs past the newline and tears apart the frames behind
// it, which is the corruption the length prefix is supposed to prevent.
func TestSyslogPartialPRIIsNotOctetCount(t *testing.T) {
	good := "<134>Feb 10 12:00:00 flushhost FLUSHTAG[1]: well_formed_message"

	cases := []struct {
		name string
		line string
	}{
		{
			// "54 " would be read as MSG-LEN=54, long enough to reach well
			// into the frame that follows.
			name: "elapsed time in angle brackets",
			line: "54 <1 minute elapsed",
		},
		{
			name: "countdown in angle brackets",
			line: "100 <2 sec remaining until timeout occurs and the job is retried",
		},
		{
			// A closing ">" is present but too far along to form a PRI.
			name: "bracketed item count",
			line: "12 <3 items> pending",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stream := tc.line + "\n" + good + "\n" + good + "\n"
			got, _ := processSyslog(t, 262144, [][]byte{[]byte(stream)})

			require.Len(t, got, 3, "each line is framed on its LF delimiter")
			assert.Equal(t, tc.line, got[0], "emitted whole, not split at the '<'")
			assert.Equal(t, good, got[1], "the following frame is neither swallowed nor truncated")
			assert.Equal(t, good, got[2])
		})
	}
}

// Resynchronizing must accept exactly what the frame reader accepts, otherwise
// a malformed run is cut short at a candidate that would then be rejected.
func TestIsSyslogFrameStartRequiresCompletePRI(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"<134>x", true},
		{"<0>", true},
		{"<191>x", true},
		{"62 <134>x", true},
		{"<1 minute", false},
		{"<1234>x", false}, // PRIVAL is at most 3 digits
		{"<>", false},
		{"<13", false}, // undecidable here; the look-behind re-examines it
		{"54 <1 min", false},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, isSyslogFrameStart([]byte(tc.in), 0), "input %q", tc.in)
	}
}

// Genuine octet-counted frames must keep working, including when the length
// prefix is split across reads: an incomplete signature has to wait for more
// bytes rather than be declared malformed.
func TestSyslogOctetCountedStillDetected(t *testing.T) {
	body := "<134>1 2024-12-04T01:18:01-05:00 test.com app - - - octet counted body"
	frame := fmt.Sprintf("%d %s", len(body), body)

	t.Run("single read", func(t *testing.T) {
		got, _ := processSyslog(t, 4096, [][]byte{[]byte(frame)})
		require.Len(t, got, 1)
		assert.Equal(t, body, got[0], "MSG-LEN SP header is stripped as framing")
	})

	// Split at every position inside the "70 <1" signature, plus a later offset,
	// so no read boundary can turn a valid frame into malformed bytes.
	for _, at := range []int{1, 2, 3, 4, 5, 10} {
		t.Run("split_after_"+strconv.Itoa(at)+"_bytes", func(t *testing.T) {
			chunks := [][]byte{[]byte(frame[:at]), []byte(frame[at:])}
			got, _ := processSyslog(t, 4096, chunks)
			require.Len(t, got, 1, "frame split at %d bytes should still be one frame", at)
			assert.Equal(t, body, got[0])
		})
	}

	t.Run("length prefix arrives one byte at a time", func(t *testing.T) {
		// The digits arrive with no SP yet: the matcher must wait, not reject.
		chunks := [][]byte{[]byte(frame[:1]), []byte(frame[1:2]), []byte(frame[2:3]), []byte(body)}
		got, _ := processSyslog(t, 4096, chunks)
		require.Len(t, got, 1)
		assert.Equal(t, body, got[0])
	})
}

func TestClassifyOctetPrefix(t *testing.T) {
	cases := []struct {
		in   string
		want octetPrefixVerdict
	}{
		{"71 <134>1 x", octetPrefixYes},
		{"9 <0>", octetPrefixYes},
		{"2024 Apr 04", octetPrefixNo},
		{"54 [main] INFO", octetPrefixNo},
		{"171113 14:14:20", octetPrefixNo},
		{"2024-04-04T08:05:06", octetPrefixNo},
		{"71 x134>", octetPrefixNo},
		{"71 <x", octetPrefixNo},
		{"71 <>", octetPrefixNo},
		{"12345678901 <134>", octetPrefixNo}, // more digits than any plausible length
		{"71 <1234>", octetPrefixNo},         // PRIVAL is at most 3 digits
		{"54 <1 minute elapsed", octetPrefixNo},
		{"71", octetPrefixNeedMore},    // digit run may continue
		{"71 ", octetPrefixNeedMore},   // need the byte after SP
		{"71 <", octetPrefixNeedMore},  // need the digit after '<'
		{"71 <1", octetPrefixNeedMore}, // PRIVAL may continue
		{"71 <134", octetPrefixNeedMore},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, classifyOctetPrefix([]byte(tc.in)), "input %q", tc.in)
	}
}
