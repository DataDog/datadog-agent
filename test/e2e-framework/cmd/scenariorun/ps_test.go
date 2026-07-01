// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
)

func TestPsCmdListsProvisionedStacks(t *testing.T) {
	t.Setenv("SCENARIORUN_STATE_DIR", t.TempDir())

	ps := scenario.ProvisionedStack{
		Scenario:  "ec2-host",
		Stack:     "my-test-stack",
		Config:    map[string]string{"os": "ubuntu-22.04"},
		Resources: map[string]json.RawMessage{"host": json.RawMessage(`{"ip":"1.2.3.4"}`)},
		CreatedAt: time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC),
	}
	if err := scenario.SaveProvisionedStack(ps); err != nil {
		t.Fatalf("SaveProvisionedStack: %v", err)
	}

	root := rootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"ps"})

	if err := root.Execute(); err != nil {
		t.Fatalf("ps command error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "my-test-stack") {
		t.Errorf("output missing stack name; got:\n%s", out)
	}
	if !strings.Contains(out, "ec2-host") {
		t.Errorf("output missing scenario name; got:\n%s", out)
	}
	if !strings.Contains(out, "STACK") || !strings.Contains(out, "SCENARIO") || !strings.Contains(out, "CREATED") {
		t.Errorf("output missing header; got:\n%s", out)
	}
}

func TestPsCmdEmptyStateShowsHeaderOnly(t *testing.T) {
	t.Setenv("SCENARIORUN_STATE_DIR", t.TempDir())

	root := rootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"ps"})

	if err := root.Execute(); err != nil {
		t.Fatalf("ps command error: %v", err)
	}

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	// Should have exactly the header line.
	if len(lines) != 1 {
		t.Errorf("expected 1 line (header), got %d:\n%s", len(lines), out)
	}
	if !strings.Contains(lines[0], "STACK") {
		t.Errorf("expected header line, got: %q", lines[0])
	}
}
