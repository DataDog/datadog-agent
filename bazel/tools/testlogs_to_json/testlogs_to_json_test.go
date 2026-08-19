// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const passingLog = "=== RUN   TestFoo\n--- PASS: TestFoo (0.01s)\nPASS\n"
const failingLog = "=== RUN   TestBar\n    bar_test.go:12: boom\n--- FAIL: TestBar (0.02s)\nFAIL\n"

func writeLog(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.log")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func convertToEvents(t *testing.T, label, contents string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := convert(label, writeLog(t, contents), &out); err != nil {
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
	return events
}

func TestConvertPassingLog(t *testing.T) {
	events := convertToEvents(t, "//pkg/foo:foo_test", passingLog)

	assertEvent(t, events, "//pkg/foo:foo_test", "TestFoo", "pass")
	assertEvent(t, events, "//pkg/foo:foo_test", "", "pass")
}

func TestConvertFailingLog(t *testing.T) {
	events := convertToEvents(t, "//pkg/bar:bar_test", failingLog)

	assertEvent(t, events, "//pkg/bar:bar_test", "TestBar", "fail")
	assertEvent(t, events, "//pkg/bar:bar_test", "", "fail")
}

// The label is the test2json package, so build-tag variants of one Go package
// stay distinct rather than looking like retries of each other.
func TestConvertRecordsLabelAsPackage(t *testing.T) {
	for _, label := range []string{"//pkg/foo:foo_test", "//pkg/foo:foo_test_containerd"} {
		events := convertToEvents(t, label, passingLog)
		for _, event := range events {
			if event["Package"] != label {
				t.Fatalf("event %#v: Package = %v, want %q", event, event["Package"], label)
			}
		}
	}
}

func TestRunWritesOutputFile(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.jsonl")

	err := run([]string{
		"-label", "//pkg/foo:foo_test",
		"-log", writeLog(t, passingLog),
		"-output", outPath,
	}, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"Package":"//pkg/foo:foo_test"`) {
		t.Fatalf("unexpected output: %s", data)
	}
}

func TestParseFlagsRequiresLabelAndLog(t *testing.T) {
	for name, args := range map[string][]string{
		"neither":    {},
		"label only": {"-label", "//pkg/foo:foo_test"},
		"log only":   {"-log", "/tmp/test.log"},
		"positional": {"-label", "//pkg/foo:foo_test", "-log", "/tmp/test.log", "extra"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseFlags(args); err == nil {
				t.Fatalf("expected an error for args %q", args)
			}
		})
	}
}

func assertEvent(t *testing.T, events []map[string]any, pkg, testName, action string) {
	t.Helper()
	for _, event := range events {
		if event["Package"] != pkg || event["Action"] != action {
			continue
		}
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
	t.Fatalf("did not find event package=%q test=%q action=%q in %#v", pkg, testName, action, events)
}
