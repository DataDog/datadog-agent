// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package sysctl

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// FuzzIntPairGet exercises IntPair.Get() against fuzzer-controlled file
// content. Without bounds checking, strings.Fields("") returns an empty slice
// and vals[0] panics; a single-field value causes vals[1] to panic.
func FuzzIntPairGet(f *testing.F) {
	// Normal: two integers separated by whitespace
	f.Add([]byte("1024\t60999\n"))
	f.Add([]byte("32768 60999\n"))
	// Crash seed: empty file → strings.Fields("") == [] → vals[0] OOB
	f.Add([]byte(""))
	f.Add([]byte("\n"))
	// Crash seed: single field → vals[1] OOB
	f.Add([]byte("1024\n"))
	f.Add([]byte("1024"))
	// Other edge cases
	f.Add([]byte("abc def\n"))
	f.Add([]byte("1024 abc\n"))
	f.Add([]byte("   \n"))
	f.Add([]byte("1024 60999 extra\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "sys", "net", "ipv4"), 0o700); err != nil {
			t.Fatal(err)
		}
		sysctl := "net/ipv4/ip_local_port_range"
		path := filepath.Join(dir, "sys", sysctl)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		ip := NewIntPair(dir, sysctl, 0)
		_, _, _ = ip.get(time.Now())
	})
}
