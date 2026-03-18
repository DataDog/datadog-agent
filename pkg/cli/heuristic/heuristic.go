// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package heuristic classifies agent CLI invocations as human or LLM-driven.
package heuristic

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const sessionInactivityTimeout = 10 * time.Minute

type state struct {
	SessionID               string `json:"session_id"`
	LastCommandUnixMilli    int64  `json:"last_command_unix_milli"`
	LastDiagnosticUnixMilli int64  `json:"last_diagnostic_unix_milli"`
}

// BuildScore computes a 0–100 heuristic score for the given CLI invocation
// and returns it along with a label: "llm" (≥65), "human" (<25), or "unknown".
func BuildScore(command string, args []string, now time.Time) (score int, label string) {
	st, _ := loadState()
	nowMS := now.UnixMilli()

	if st == nil || st.LastCommandUnixMilli == 0 || now.Sub(time.UnixMilli(st.LastCommandUnixMilli)) > sessionInactivityTimeout {
		st = &state{
			SessionID: strconv.FormatInt(nowMS, 10),
		}
	}

	// Signal: inter-command latency <500ms between diagnostic commands (+40)
	if isDiagnosticCommand(command) && st.LastDiagnosticUnixMilli > 0 {
		diff := nowMS - st.LastDiagnosticUnixMilli
		if diff >= 0 && diff < 500 {
			score += 40
		}
	}

	// Signal: machine-readable output flags (+30)
	if hasMachineFirstFlags(args) {
		score += 30
	}

	// Signal: stdin is not a TTY (+15)
	if !isStdinTTY() {
		score += 15
	}

	// Signal: parent process uptime — Linux-only (+45 if <5s, +20 if 5–30s)
	if uptimeMS, ok := parentProcessUptimeMS(); ok {
		switch {
		case uptimeMS < 5000:
			score += 45
		case uptimeMS < 30000:
			score += 20
		}
	}

	if score > 100 {
		score = 100
	}

	// Update state
	st.LastCommandUnixMilli = nowMS
	if isDiagnosticCommand(command) {
		st.LastDiagnosticUnixMilli = nowMS
	}
	_ = saveState(st)

	switch {
	case score >= 65:
		label = "llm"
	case score < 25:
		label = "human"
	default:
		label = "unknown"
	}

	return score, label
}

func isDiagnosticCommand(command string) bool {
	switch command {
	case "agent status", "status",
		"agent health", "health",
		"agent flare", "flare",
		"agent config", "config",
		"agent configcheck", "configcheck",
		"agent diagnose", "diagnose",
		"agent hostname", "hostname",
		"agent secret", "secret",
		"agent tagger-list", "tagger-list",
		"agent workload-list", "workload-list",
		"agent dogstatsd-stats", "dogstatsd-stats",
		"agent stream-logs", "stream-logs",
		"agent stream-event-platform", "stream-event-platform":
		return true
	default:
		return false
	}
}

func hasMachineFirstFlags(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "--json", "-j", "-p", "--pretty-json":
			return true
		}
	}
	return false
}

func isStdinTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return true // assume TTY on error
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func statePath() string {
	return filepath.Join(os.TempDir(), "dd-agent-cli-heuristic-state.json")
}

func loadState() (*state, error) {
	body, err := os.ReadFile(statePath())
	if err != nil {
		return nil, err
	}
	var st state
	if err := json.Unmarshal(body, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

func saveState(st *state) error {
	body, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(), body, 0o600)
}
