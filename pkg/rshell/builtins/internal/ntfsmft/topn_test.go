// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ntfsmft

import (
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func utf16NameBytes(t *testing.T, name string) []byte {
	t.Helper()
	u, err := windows.UTF16FromString(name)
	if err != nil {
		t.Fatalf("UTF16FromString(%q): %v", name, err)
	}
	b := make([]byte, (len(u)-1)*2)
	for i, c := range u[:len(u)-1] {
		b[i*2] = byte(c)
		b[i*2+1] = byte(c >> 8)
	}
	return b
}

func TestExtractAsciiExtensionScansFullName(t *testing.T) {
	for _, ext := range []string{
		strings.Repeat("a", 23),
		strings.Repeat("b", 24),
		strings.Repeat("C", 254),
	} {
		var buf [255]byte
		n := extractAsciiExtension(utf16NameBytes(t, "file."+ext), buf[:])
		if n != len(ext) {
			t.Fatalf("extension length for %d chars = %d, want %d", len(ext), n, len(ext))
		}
		if got, want := string(buf[:n]), strings.ToLower(ext); got != want {
			t.Errorf("extension = %q, want %q", got, want)
		}
	}
}

func TestDotfilesDoNotHaveExtensions(t *testing.T) {
	for _, name := range []string{".claude", ".agents", "..config", ".claude.json"} {
		t.Run(name, func(t *testing.T) {
			var buf [255]byte
			got := extractAsciiExtension(utf16NameBytes(t, name), buf[:])
			if name == ".claude.json" {
				if got != len("json") || string(buf[:got]) != "json" {
					t.Fatalf("extension = %q (%d), want json", string(buf[:max(got, 0)]), got)
				}
				return
			}
			if got != noExtension {
				t.Fatalf("extension result = %d, want noExtension", got)
			}
		})
	}

	nameBytes := utf16NameBytes(t, ".claude")
	agg := newExtAggregator(true)
	agg.addFromName(nameBytes, 42)
	if got := agg.bySize[""]; got != 42 {
		t.Errorf("dotfile no-extension aggregate = %d, want 42", got)
	}
	if _, found := agg.bySize["claude"]; found {
		t.Error("dotfile must not create a claude extension bucket")
	}

	m, err := newMatchSet([]FindQuery{{Type: "ext", Value: "claude", Limit: 1}}, 0)
	if err != nil {
		t.Fatalf("newMatchSet: %v", err)
	}
	m.consider(7, &mftEntry{nameBytes: nameBytes}, 42)
	if got := len(m.drained()[0]); got != 0 {
		t.Errorf("dotfile extension matches = %d, want 0", got)
	}
}

func TestLongASCIIExtensionAggregatesAndMatches(t *testing.T) {
	ext := strings.Repeat("x", 24)
	nameBytes := utf16NameBytes(t, "file."+ext)

	agg := newExtAggregator(true)
	agg.addFromName(nameBytes, 42)
	if got := agg.bySize[ext]; got != 42 {
		t.Errorf("long ASCII extension aggregate = %d, want 42", got)
	}

	m, err := newMatchSet([]FindQuery{{Type: "ext", Value: ext, Limit: 1}}, 0)
	if err != nil {
		t.Fatalf("newMatchSet: %v", err)
	}
	m.consider(7, &mftEntry{nameBytes: nameBytes}, 42)
	if got := len(m.drained()[0]); got != 1 {
		t.Errorf("long ASCII extension matches = %d, want 1", got)
	}
}

func TestNonASCIIExtensionsDecodeForAggregationAndFind(t *testing.T) {
	for _, ext := range []string{"\u0080", "\u0101", "日本語"} {
		t.Run(ext, func(t *testing.T) {
			nameBytes := utf16NameBytes(t, "file."+ext)
			var buf [255]byte
			if got := extractAsciiExtension(nameBytes, buf[:]); got != nonASCIIExtension {
				t.Fatalf("extractAsciiExtension = %d, want nonASCIIExtension", got)
			}

			agg := newExtAggregator(true)
			agg.addFromName(nameBytes, 42)
			if got := agg.bySize[ext]; got != 42 {
				t.Errorf("non-ASCII extension aggregate = %d, want 42", got)
			}

			m, err := newMatchSet([]FindQuery{{Type: "ext", Value: ext, Limit: 1}}, 0)
			if err != nil {
				t.Fatalf("newMatchSet: %v", err)
			}
			m.consider(7, &mftEntry{nameBytes: nameBytes}, 42)
			if got := len(m.drained()[0]); got != 1 {
				t.Errorf("non-ASCII extension matches = %d, want 1", got)
			}
		})
	}
}
