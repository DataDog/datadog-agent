// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package newerrors

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	sharedStack = "pkg/a.Handle\n\t/go/src/pkg/a/a.go:10 +0x1a4\nmain.run\n\t/go/src/cmd/main.go:5 +0x20"
	// Same error as sharedStack as built by another release: other offsets,
	// shifted lines, another build path.
	sharedStackOtherBuild = "pkg/a.Handle\n\t/build/pkg/a/a.go:42 +0x2f0\nmain.run\n\t/build/cmd/main.go:9 +0x88"
	seededStack           = "pkg/b.Regress\n\t/go/src/pkg/b/b.go:7 +0x10\nmain.run\n\t/go/src/cmd/main.go:5 +0x20"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name  string
		stack string
		want  string
	}{
		{"offsets lines and paths dropped", sharedStack, "pkg/a.Handle a.go\nmain.run main.go"},
		{"call arguments dropped", "main.f(0xc000123, 0x2)\n\t/x/f.go:1 +0x3", "main.f f.go"},
		{"ellipsis arguments dropped", "main.f(...)\n\t/x/f.go:1", "main.f f.go"},
		{"empty", "", ""},
		{"non frame text ignored", "goroutine 1 [running]:", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Normalize(tt.stack))
		})
	}
}

func TestFingerprint_StableAcrossCallerAndNumberingChanges(t *testing.T) {
	frame := func(fn, file string) string { return fn + "\n\t/x/" + file + ":1 +0x1\n" }
	logging := frame("github.com/DataDog/datadog-agent/pkg/util/log.Errorf", "log.go") +
		frame("github.com/DataDog/datadog-agent/pkg/util/log/errortracking.(*handler).Handle", "handler.go")
	core := frame("pkg/a.Fail", "a.go") + frame("pkg/a.Run.func1", "a.go") + frame("pkg/b.Loop", "b.go") +
		frame("pkg/c.Start", "c.go") + frame("pkg/d.Main", "d.go")

	base := logging + core + frame("runtime.goexit", "asm.s")
	tests := []struct {
		name  string
		stack string
	}{
		{"extra deep caller", logging + core + frame("pkg/e.Extra", "e.go") + frame("runtime.goexit", "asm.s")},
		{"no goexit", logging + core},
		{"closure renumbered", logging + strings.Replace(core, "Run.func1", "Run.func3", 1) + frame("runtime.goexit", "asm.s")},
		{"no logging frames", core + frame("runtime.goexit", "asm.s")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, Fingerprint(base), Fingerprint(tt.stack))
		})
	}

	// A change in the top frames is a different error.
	assert.NotEqual(t, Fingerprint(base), Fingerprint(logging+strings.Replace(core, "pkg/a.Fail", "pkg/a.Other", 1)))
}

func TestFindNew(t *testing.T) {
	tests := []struct {
		name      string
		baseline  Cohort
		candidate Cohort
		wantStack []string
	}{
		{
			name:      "seeded error found, shared error across builds not reported",
			baseline:  Cohort{Events: []Event{{Host: "b1", StackTrace: sharedStack}}},
			candidate: Cohort{Events: []Event{{Host: "c1", StackTrace: sharedStackOtherBuild}, {Host: "c1", StackTrace: seededStack}}},
			wantStack: []string{Normalize(seededStack)},
		},
		{
			name:      "only shared errors",
			baseline:  Cohort{Events: []Event{{Host: "b1", StackTrace: sharedStack}}},
			candidate: Cohort{Events: []Event{{Host: "c1", StackTrace: sharedStackOtherBuild}}},
		},
		{
			name:      "empty baseline reports every candidate error",
			candidate: Cohort{Events: []Event{{Host: "c1", StackTrace: sharedStack}, {Host: "c1", StackTrace: seededStack}}},
			wantStack: []string{Normalize(sharedStack), Normalize(seededStack)},
		},
		{
			name:     "empty candidate",
			baseline: Cohort{Events: []Event{{Host: "b1", StackTrace: sharedStack}}},
		},
		{
			name: "both empty",
		},
		{
			name:      "events without frames are ignored",
			candidate: Cohort{Events: []Event{{Host: "c1", StackTrace: ""}, {Host: "c1", StackTrace: "no frames here"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindNew(tt.baseline, tt.candidate)
			var stacks []string
			for _, n := range got {
				stacks = append(stacks, n.Stack)
			}
			assert.ElementsMatch(t, tt.wantStack, stacks)
		})
	}
}

func TestFindNew_DuplicatesCollapseAndRateIsPerHost(t *testing.T) {
	candidate := Cohort{Events: []Event{
		{Host: "c1", StackTrace: seededStack, Count: 3},
		{Host: "c1", StackTrace: seededStack}, // Count below 1 counts as 1
		{Host: "c2", StackTrace: seededStack, Count: 2},
		{Host: "c3", StackTrace: sharedStack},
	}}
	got := FindNew(Cohort{Events: []Event{{Host: "b1", StackTrace: sharedStack}}}, candidate)

	require.Len(t, got, 1)
	assert.Equal(t, 6, got[0].Occurrences)
	// Three distinct hosts in the candidate cohort.
	assert.InDelta(t, 2.0, got[0].RatePerHost, 1e-9)
	assert.Equal(t, Fingerprint(seededStack), got[0].Fingerprint)
}

func TestFindNew_ExplicitHostCountIncludesSilentHosts(t *testing.T) {
	candidate := Cohort{Hosts: 4, Events: []Event{{Host: "c1", StackTrace: seededStack, Count: 2}}}
	got := FindNew(Cohort{}, candidate)

	require.Len(t, got, 1)
	assert.InDelta(t, 0.5, got[0].RatePerHost, 1e-9)
}

func TestFindNew_OrderedByRateThenFingerprint(t *testing.T) {
	candidate := Cohort{Events: []Event{
		{Host: "c1", StackTrace: sharedStack},
		{Host: "c1", StackTrace: seededStack, Count: 5},
	}}
	got := FindNew(Cohort{}, candidate)

	require.Len(t, got, 2)
	assert.Equal(t, Fingerprint(seededStack), got[0].Fingerprint)
}
