// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package seclog holds seclog related files
package seclog

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestTopCounts(t *testing.T) {
	tests := []struct {
		name   string
		counts map[string]int
		n      int
		want   string
	}{
		{name: "empty", counts: map[string]int{}, n: 3, want: ""},
		{name: "sorted by count", counts: map[string]int{"a": 1, "b": 5, "c": 3}, n: 3, want: "b:5,c:3,a:1"},
		{name: "truncated", counts: map[string]int{"a": 1, "b": 5, "c": 3}, n: 2, want: "b:5,c:3"},
		// ties break on the key so the output is stable across runs, which matters when the
		// value is compared between platforms.
		{name: "stable ties", counts: map[string]int{"z": 2, "a": 2}, n: 2, want: "a:2,z:2"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := topCounts(test.counts, test.n); got != test.want {
				t.Errorf("topCounts() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestReadProcStat(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name      string
		content   string
		wantOK    bool
		wantState string
	}{
		// The comm field is attacker-controlled and may hold spaces and parentheses, so parsing
		// has to key off the last ')' rather than splitting on whitespace.
		{name: "plain", content: "42 (bash) S 1 42 42 0 -1 4194560 100 0 0 0 11 12\n", wantOK: true, wantState: "S"},
		{name: "comm with spaces and parens", content: "42 (weird (name) here) D 1 42 42 0 -1 4194560 100 0 0 0 11 12\n", wantOK: true, wantState: "D"},
		{name: "no closing paren", content: "42 bash S 1\n", wantOK: false},
		{name: "empty", content: "", wantOK: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(dir, "stat")
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}

			fields, ok := readProcStat(path)
			if ok != test.wantOK {
				t.Fatalf("readProcStat() ok = %v, want %v", ok, test.wantOK)
			}
			if !test.wantOK {
				return
			}
			if len(fields) == 0 || fields[0] != test.wantState {
				t.Errorf("readProcStat() state = %v, want %q", fields, test.wantState)
			}
		})
	}

	if _, ok := readProcStat(filepath.Join(dir, "does-not-exist")); ok {
		t.Error("readProcStat() on a missing file = ok, want not ok")
	}
}

func TestProcessIntrospection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("reads /proc")
	}

	cpu, ok := processCPUTime()
	if !ok {
		t.Fatal("processCPUTime() not ok, want ok on linux")
	}
	if cpu < 0 {
		t.Errorf("processCPUTime() = %v, want >= 0", cpu)
	}

	threads, states, _ := sampleThreads()
	if threads == 0 {
		t.Error("sampleThreads() found no threads, want at least this one")
	}
	if len(states) == 0 {
		t.Error("sampleThreads() found no thread states, want at least one")
	}
}

func TestStartPhaseProfileDisabledIsNoop(t *testing.T) {
	previous := phaseProfilingEnabled
	phaseProfilingEnabled = false
	defer func() { phaseProfilingEnabled = previous }()

	// Must not panic nor sample anything when profiling is off, since it sits on the shutdown
	// path of the shipped agent.
	StartPhaseProfile("test")()
}
