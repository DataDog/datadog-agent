// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package syslog

import (
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
