// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadManifest(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.tsv")
	if err := os.WriteFile(manifestPath, []byte(strings.Join([]string{
		"",
		"github.com/DataDog/datadog-agent/pkg/foo\t/path/to/foo.log",
		"github.com/DataDog/datadog-agent/pkg/bar\t/path with spaces/bar.log",
	}, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := readManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}

	want := []manifestEntry{
		{pkg: "github.com/DataDog/datadog-agent/pkg/foo", logPath: "/path/to/foo.log"},
		{pkg: "github.com/DataDog/datadog-agent/pkg/bar", logPath: "/path with spaces/bar.log"},
	}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d: %#v", len(entries), len(want), entries)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Fatalf("entry %d = %#v, want %#v", i, entries[i], want[i])
		}
	}
}

func TestReadManifestRejectsInvalidLine(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.tsv")
	if err := os.WriteFile(manifestPath, []byte("github.com/DataDog/datadog-agent/pkg/foo /path/to/foo.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := readManifest(manifestPath)
	if err == nil || !strings.Contains(err.Error(), "invalid manifest line") {
		t.Fatalf("expected invalid manifest error, got %v", err)
	}
}

func TestConvertMultipleLogs(t *testing.T) {
	dir := t.TempDir()
	fooLog := filepath.Join(dir, "foo.log")
	barLog := filepath.Join(dir, "bar.log")
	if err := os.WriteFile(fooLog, []byte("=== RUN   TestFoo\n--- PASS: TestFoo (0.01s)\nPASS\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(barLog, []byte("=== RUN   TestBar\n    bar_test.go:12: boom\n--- FAIL: TestBar (0.02s)\nFAIL\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err := convert([]manifestEntry{
		{pkg: "github.com/DataDog/datadog-agent/pkg/foo", logPath: fooLog},
		{pkg: "github.com/DataDog/datadog-agent/pkg/bar", logPath: barLog},
	}, &out)
	if err != nil {
		t.Fatal(err)
	}

	var events []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("invalid JSON line %q: %v", line, err)
		}
		events = append(events, event)
	}

	assertEvent(t, events, "github.com/DataDog/datadog-agent/pkg/foo", "TestFoo", "pass")
	assertEvent(t, events, "github.com/DataDog/datadog-agent/pkg/foo", "", "pass")
	assertEvent(t, events, "github.com/DataDog/datadog-agent/pkg/bar", "TestBar", "fail")
	assertEvent(t, events, "github.com/DataDog/datadog-agent/pkg/bar", "", "fail")
}

func assertEvent(t *testing.T, events []map[string]any, pkg, testName, action string) {
	t.Helper()
	for _, event := range events {
		if event["Package"] == pkg && event["Action"] == action {
			if testName == "" {
				if _, ok := event["Test"]; !ok {
					return
				}
				continue
			}
			if event["Test"] == testName {
				return
			}
		}
	}
	t.Fatalf("did not find event package=%q test=%q action=%q in %#v", pkg, testName, action, events)
}
