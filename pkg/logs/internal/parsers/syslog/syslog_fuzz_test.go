// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
	"strings"
	"testing"
)

func FuzzParse(f *testing.F) {
	// RFC 5424 seeds
	f.Add([]byte(`<165>1 2003-10-11T22:14:15.003Z mymachine evntslog - ID47 [exampleSDID@32473 iut="3"] msg`))
	f.Add([]byte(`<14>1 - - - - - - test`))
	f.Add([]byte(`<75>1 1969-12-03T23:58:58Z - - - - -`))
	f.Add([]byte(`<13>1 2019-02-13T19:48:34+00:00 host root 8449 - [meta sequenceId="1"][origin ip="10.0.0.1"] msg`))

	// BSD seeds
	f.Add([]byte(`<13>Feb 13 20:07:26 74794bfb6795 root[8539]: i am foobar`))
	f.Add([]byte(`<190>Dec 28 16:49:07 myhost nginx: 127.0.0.1 - - request`))
	f.Add([]byte(`<46>Jan  5 15:33:03 myhost rsyslogd: start`))
	f.Add([]byte(`<4>Jan 26 05:59:54 ubnt kernel: [WAN_LOCAL-default-D]IN=eth0`))

	// Edge cases
	f.Add([]byte(`<14>`))
	f.Add([]byte(`<>`))
	f.Add([]byte(`<999>1 - - - - - - test`))
	f.Add([]byte(`<0>1 - - - - - - test`))
	f.Add([]byte(``))

	// CEF/LEEF envelope seeds
	f.Add([]byte(`<14>1 2026-03-30T12:00:00Z host app - - - CEF:0|Security|FW|1.0|100|Attack|10|src=10.0.0.1`))
	f.Add([]byte(`<14>1 2026-03-30T12:00:00Z host app - - - LEEF:1.0|Vendor|Product|1.0|100|src=10.0.0.1`))
	f.Add([]byte(`not syslog`))
	f.Add([]byte("\x00\x01\x02"))
	f.Add([]byte("\xff\xfe\xfd"))
	f.Add([]byte(`<14>1 ts host app pid mid [sd key="val\"ue"] msg`))

	f.Fuzz(func(t *testing.T, data []byte) {
		msg, err := Parse(data)

		if err == nil {
			if msg.Pri < 0 || msg.Pri > 191 {
				if !msg.Partial {
					t.Errorf("Pri %d out of range without Partial flag", msg.Pri)
				}
			}
		} else {
			if msg.Pri >= 0 {
				// PRI was successfully parsed but a later stage failed.
				if msg.Pri > 191 && !msg.Partial {
					t.Errorf("Pri %d out of range without Partial flag", msg.Pri)
				}
			}
			// Pri == -1 means PRI was never parsed — always valid on error.
		}
	})
}

func FuzzParseBSDLine(f *testing.F) {
	f.Add([]byte(`Dec 28 16:49:07 myhost nginx: 127.0.0.1 - - request`))
	f.Add([]byte(`Jan  5 15:33:03 myhost rsyslogd: start`))
	f.Add([]byte(`not syslog at all`))
	f.Add([]byte(`12345 numeric prefix`))
	f.Add([]byte(``))
	f.Add([]byte("\x00\x01\x02"))
	f.Add([]byte(`Feb 13 20:07:26 host root[8539]: msg`))

	f.Fuzz(func(t *testing.T, data []byte) {
		msg, _ := ParseBSDLine(data)

		if msg.Pri != -1 {
			t.Errorf("ParseBSDLine should always return Pri=-1, got %d", msg.Pri)
		}
	})
}

func FuzzParseCEFLEEF(f *testing.F) {
	// Escape-heavy inputs exercise the splitter and unescaper.
	f.Add([]byte("CEF:0|Security|threatmanager|1.0|100|worm successfully stopped|10|src=10.0.0.1 dst=2.1.2.2"))
	f.Add([]byte(`CEF:0|Ven\|dor|Prod\\uct|1.0|100|Na\|me|High|act=blocked a \= b dst=1.1.1.1`))
	f.Add([]byte("LEEF:1.0|Microsoft|MSExchange|2013 SP1|15345|src=10.0.1.7\tdst=10.0.0.5"))
	f.Add([]byte("LEEF:2.0|Vendor|Product|1.0|100|^|src=10.0.1.8^dst=10.0.0.5"))
	f.Add([]byte("LEEF:2|Vendor|Product|1.0|100|^|src=a^dst=b"))
	f.Add([]byte("CEF:"))
	f.Add([]byte("LEEF:"))
	f.Add([]byte("CEF:0|A|B|C|D|E|F|" + strings.Repeat("k0=v0 ", 64)))
	f.Add([]byte("\x00\x01\x02"))

	f.Fuzz(func(t *testing.T, data []byte) {
		header, ext, _, ok := ParseCEFLEEF(data)
		if !ok {
			return
		}
		if header.Format != "CEF" && header.Format != "LEEF" {
			t.Errorf("unexpected format %q", header.Format)
		}
		// Every key=value pair costs at least 2 bytes (key + '=').
		if len(ext)*2 > len(data) {
			t.Errorf("extension has %d keys for %d-byte input", len(ext), len(data))
		}
		// For CEF, every extension key must consist of valid key characters.
		if header.Format == "CEF" {
			for k := range ext {
				for i := 0; i < len(k); i++ {
					if !isKeyChar(k[i]) {
						t.Errorf("invalid key char %q in key %q", k[i], k)
					}
				}
			}
		}
		// Determinism: re-parsing identical input must produce the same header.
		// SIEMHeader is all-string fields, so == works directly.
		h2, _, _, ok2 := ParseCEFLEEF(data)
		if !ok2 {
			t.Fatal("re-parse returned ok=false for previously ok input")
		}
		if h2 != header {
			t.Errorf("non-deterministic header: first=%+v second=%+v", header, h2)
		}
	})
}
