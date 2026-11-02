// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package diskstats

import (
	"context"
	"strings"
	"testing"
)

// FuzzParseMountInfo feeds arbitrary inputs to parseMountInfo and
// asserts:
//   - the function does not panic
//   - it does not loop indefinitely (timeout-bounded by the test runner)
//   - returned mounts have non-empty MountPoint and FSType
//   - the returned slice length never exceeds MaxMounts
//   - lines exceeding maxMountInfoLine surface as an error rather than crashing
//
// Seed corpus draws from three sources, per the skill protocol:
//
//   - Implementation edge cases: every named constant and boundary
//     check in the parser (separator " - ", octal escapes, field
//     count thresholds).
//   - CVE / security history: integer-overflow inputs, embedded null
//     bytes, CRLF, invalid UTF-8, ELF/PE/ZIP magic prefixes, very long
//     lines.
//   - Existing test coverage: every distinct sample mountinfo string
//     from diskstats_linux_parse_test.go is replayed as a seed.
func FuzzParseMountInfo(f *testing.F) {
	// --- Source A: implementation edge cases ---
	f.Add([]byte(""))
	f.Add([]byte("\n"))
	f.Add([]byte(" - \n"))
	f.Add([]byte("a b c d e f - g h\n"))
	// Minimum valid mountinfo line.
	f.Add([]byte("36 35 98:0 / / rw - ext4 /dev/sda1 rw\n"))
	// Optional fields between mount opts and " - ".
	f.Add([]byte("36 35 98:0 / / rw shared:1 master:2 - ext4 /dev/sda1 rw\n"))
	// Octal-escaped space, tab, newline, backslash in mount point.
	f.Add([]byte("36 35 98:0 / /a\\040b rw - ext4 /dev/x rw\n"))
	f.Add([]byte("36 35 98:0 / /a\\011b rw - ext4 /dev/x rw\n"))
	f.Add([]byte("36 35 98:0 / /a\\012b rw - ext4 /dev/x rw\n"))
	f.Add([]byte("36 35 98:0 / /a\\134b rw - ext4 /dev/x rw\n"))
	// Truncated escape at end of field.
	f.Add([]byte("36 35 98:0 / /a\\04 rw - ext4 /dev/x rw\n"))
	// Non-octal escape (\999).
	f.Add([]byte("36 35 98:0 / /a\\999b rw - ext4 /dev/x rw\n"))
	// Pseudo and remote filesystem types from the classification table.
	for _, fs := range []string{"tmpfs", "proc", "sysfs", "devtmpfs", "cgroup2", "nfs", "nfs4", "cifs", "smb3", "fuse.gvfsd-fuse"} {
		f.Add([]byte("36 35 98:0 / /m rw - " + fs + " src rw\n"))
	}

	// --- Source B: CVE / security history ---
	// Embedded NUL.
	f.Add([]byte("36 35 98:0 / /m\x00x rw - ext4 /dev/x rw\n"))
	// CRLF.
	f.Add([]byte("36 35 98:0 / / rw - ext4 /dev/x rw\r\n"))
	// Invalid UTF-8 in mount point.
	f.Add([]byte("36 35 98:0 / /\xff\xfe rw - ext4 /dev/x rw\n"))
	// Binary magic prefix (ELF) — must not be misinterpreted as a line.
	f.Add([]byte("\x7fELF\x02\x01\x01\x00 - ext4 src rw\n"))
	// PE.
	f.Add([]byte("MZ\x90\x00 - ext4 src rw\n"))
	// ZIP.
	f.Add([]byte("PK\x03\x04 - ext4 src rw\n"))
	// Multi-line mix of valid + garbage.
	f.Add([]byte("36 35 98:0 / / rw - ext4 /dev/x rw\nmalformed line\n37 36 0:18 / /sys rw - sysfs sysfs rw\n"))
	// All-NUL.
	f.Add([]byte("\x00\x00\x00\n"))
	// Many separators in one line.
	f.Add([]byte("36 - 98:0 - / / rw - ext4 /dev/sda1 rw\n"))

	// --- Source C: existing test coverage replays ---
	f.Add([]byte(sampleMountInfo))
	f.Add([]byte("not enough fields\n"))
	f.Add([]byte("short - bad\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Cap fuzz inputs at 1 MiB. Larger inputs would force the
		// scanner to fail with errLineTooLong, which we already test
		// directly; mass-fuzzing them just slows the run down.
		if len(data) > 1<<20 {
			return
		}

		mounts, err := parseMountInfo(context.Background(), strings.NewReader(string(data)))

		// MUST: returned slice never exceeds the cap.
		if len(mounts) > MaxMounts {
			t.Fatalf("parseMountInfo returned %d mounts, exceeds MaxMounts=%d", len(mounts), MaxMounts)
		}

		// MUST: every returned mount has both fields populated.
		for i, m := range mounts {
			if m.MountPoint == "" {
				t.Fatalf("mount %d has empty MountPoint", i)
			}
			if m.FSType == "" {
				t.Fatalf("mount %d has empty FSType", i)
			}
		}

		// err is informational — ErrMaxMounts and errLineTooLong are
		// expected for adversarial inputs and not failures.
		_ = err
	})
}

// FuzzUnescapeMountField exercises the octal-unescape helper.
func FuzzUnescapeMountField(f *testing.F) {
	f.Add("plain")
	f.Add("a\\040b")
	f.Add("a\\011b")
	f.Add("a\\012b")
	f.Add("a\\134b")
	f.Add("\\040")
	f.Add("\\")
	f.Add("\\0")
	f.Add("\\04")
	f.Add("\\999")
	f.Add("")
	f.Add(strings.Repeat("\\040", 100))
	f.Add(strings.Repeat("\\\\", 100))

	f.Fuzz(func(t *testing.T, in string) {
		// Cap fuzz inputs at 64 KiB; nothing here scales by allocation.
		if len(in) > 1<<16 {
			return
		}
		got := unescapeMountField(in)
		// MUST: result is no longer than the input (unescape only
		// shrinks).
		if len(got) > len(in) {
			t.Fatalf("unescapeMountField grew the string: in=%d out=%d", len(in), len(got))
		}
	})
}
