// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// White-box fuzz tests for the Linux /proc/net/ parsing helpers.
// These run in the procnetsocket package to access unexported functions.
//
// Seed corpus is built from:
//
//	A. Implementation constants (MaxLineBytes, boundary values).
//	B. CVE-class inputs: null bytes, CRLF, invalid UTF-8, large values.
//	C. All hex strings used in procnetsocket_linux_parse_test.go.
package procnetsocket

import (
	"bytes"
	"testing"
)

// FuzzParseIPv4Proc fuzzes the IPv4 address parser with arbitrary hex strings.
// It must never panic regardless of input.
func FuzzParseIPv4Proc(f *testing.F) {
	// Source C: from existing unit tests
	f.Add("0100007F") // 127.0.0.1
	f.Add("00000000") // 0.0.0.0
	f.Add("FFFFFFFF") // 255.255.255.255
	f.Add("0100007f") // lowercase
	// Source A: boundary values
	f.Add("00000001")
	f.Add("01000000")
	// Source B: CVE-class
	f.Add("")                 // empty
	f.Add("010000")           // too short
	f.Add("0100007F00")       // too long
	f.Add("ZZZZZZZZ")         // invalid hex
	f.Add("00\x0000000")      // null byte
	f.Add("0000000G")         // invalid last char
	f.Add("        ")         // spaces
	f.Add("0x0100007F")       // C-style hex prefix
	f.Add("0100007F0100007F") // double length

	f.Fuzz(func(t *testing.T, s string) {
		// Must never panic; error return is fine.
		_, _ = parseIPv4Proc(s)
	})
}

// FuzzParseIPv6Proc fuzzes the IPv6 address parser with arbitrary hex strings.
func FuzzParseIPv6Proc(f *testing.F) {
	// Source C: from existing unit tests
	f.Add("00000000000000000000000001000000") // ::1
	f.Add("00000000000000000000000000000000") // ::
	// Source A: boundary values (32-char strings)
	f.Add("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF") // all F
	f.Add("0F0F0F0F0F0F0F0F0F0F0F0F0F0F0F0F")
	// Source B: CVE-class
	f.Add("")                                    // empty
	f.Add("0000000000000000000000000000000")     // 31 chars (too short)
	f.Add("000000000000000000000000000000001")   // 33 chars (too long)
	f.Add("ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ")    // invalid hex
	f.Add("00000000\x0000000000000000000000000") // null byte
	f.Add(string(bytes.Repeat([]byte("0"), 32))) // valid all-zero
	f.Add(string(bytes.Repeat([]byte("F"), 32))) // valid all-F
	f.Add(string(bytes.Repeat([]byte("G"), 32))) // invalid

	f.Fuzz(func(t *testing.T, s string) {
		_, _ = parseIPv6Proc(s)
	})
}

// FuzzFormatIPv6 fuzzes the IPv6 formatter with arbitrary byte slices.
// Only 16-byte inputs are used; shorter/longer are silently skipped.
func FuzzFormatIPv6(f *testing.F) {
	// Source C: from existing unit tests
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1})             // ::1
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})             // ::
	f.Add([]byte{0x20, 0x01, 0x0d, 0xb8, 0, 1, 0, 2, 0, 3, 0, 4, 0, 5, 0, 6}) // 2001:db8:...
	// Source A: boundary values
	f.Add(bytes.Repeat([]byte{0xFF}, 16))                         // all 0xFF
	f.Add([]byte{0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1, 0, 1}) // alternating
	// Source B: CVE-class
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0xFF, 0xFF, 0x7F, 0, 0, 1, 0, 0}) // IPv4-mapped

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) != 16 {
			return // only test exactly 16-byte inputs
		}
		var b [16]byte
		copy(b[:], raw)
		result := formatIPv6(b)
		if result == "" {
			t.Error("formatIPv6 returned empty string")
		}
	})
}

// FuzzParsePortProc fuzzes the port parser with arbitrary strings.
func FuzzParsePortProc(f *testing.F) {
	// Source C: from existing unit tests
	f.Add("0016") // 22
	f.Add("01BB") // 443
	f.Add("0000") // 0
	f.Add("FFFF") // 65535
	// Source B: CVE-class
	f.Add("")        // empty
	f.Add("ZZZZ")    // invalid hex
	f.Add("0")       // too short
	f.Add("00000")   // too long (parsePortProc accepts 1-4 chars since ParseUint is flexible)
	f.Add("\x00000") // null byte
	f.Add("0001")    // port 1

	f.Fuzz(func(t *testing.T, s string) {
		_, _ = parsePortProc(s)
	})
}
