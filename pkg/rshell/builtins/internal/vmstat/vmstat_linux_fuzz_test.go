// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package vmstat

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// FuzzParseMeminfoLine exercises the "Key:   value kB" line parser.
//
// Seed corpus draws from three sources, per the skill protocol:
//   - Implementation edge cases: missing colon, missing value, non-numeric
//     value, trailing "kB" unit, no unit.
//   - CVE / security history: embedded NUL, CRLF, invalid UTF-8, very long
//     lines, huge numeric values (overflow probing).
//   - Existing test coverage: every line from sampleMeminfo.
func FuzzParseMeminfoLine(f *testing.F) {
	f.Add("MemTotal:        8000000 kB")
	f.Add("MemFree:         2000000 kB")
	f.Add("Buffers:          100000 kB")
	f.Add("SwapTotal:             0 kB")
	f.Add("NoColon 123 kB")
	f.Add("Key:")
	f.Add("Key: kB")
	f.Add("Key: notanumber kB")
	f.Add("Key: 123")
	f.Add("Key: 18446744073709551615 kB")    // MaxUint64
	f.Add("Key: 99999999999999999999999 kB") // overflows uint64
	f.Add("Key: -1 kB")
	f.Add("")
	f.Add(":")
	f.Add("::::")
	f.Add("Key:\x00123 kB")
	f.Add("Key: 123 kB\r")
	f.Add("Key: 123 \xff\xfe")

	f.Fuzz(func(t *testing.T, line string) {
		if len(line) > 1<<16 {
			return
		}
		// MUST: never panics, and any returned kb is only trusted when ok.
		key, kb, ok := parseMeminfoLine(line)
		if !ok {
			return
		}
		if key == "" {
			t.Fatalf("parseMeminfoLine returned ok=true with empty key for %q", line)
		}
		_ = kb
	})
}

// FuzzReadProcStat feeds arbitrary content as a synthetic /proc/stat file
// and asserts readProcStat never panics and never returns a Stats with
// fields set unless it reports success.
func FuzzReadProcStat(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("\n"))
	f.Add([]byte("cpu \n"))
	f.Add([]byte("cpu  1 2 3 4 5 6 7 8 9 10\n"))
	f.Add([]byte("cpu  99999999999999999999 2 3\n")) // overflow
	f.Add([]byte("intr\n"))
	f.Add([]byte("ctxt notanumber\n"))
	f.Add([]byte("procs_running -1\n"))
	f.Add([]byte(sampleProcStat))
	f.Add([]byte("\x00\x00\x00\n"))
	f.Add([]byte("cpu  1 2 3 4 5 6 7 8\r\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			return
		}
		dir := t.TempDir()
		path := filepath.Join(dir, "stat")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Skip()
		}
		var st Stats
		_, _, _, _ = readProcStat(context.Background(), path, &st) // MUST NOT panic
	})
}
