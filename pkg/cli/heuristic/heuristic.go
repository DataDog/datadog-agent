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
	"strings"
	"time"
)

const sessionInactivityTimeout = 10 * time.Minute

type state struct {
	SessionID               string `json:"session_id"`
	LastCommandUnixMilli    int64  `json:"last_command_unix_milli"`
	LastDiagnosticUnixMilli int64  `json:"last_diagnostic_unix_milli"`
}

// BuildScore computes a 0–100 heuristic score for the given CLI invocation
// and returns it along with a label: "self_reported" (agent mode detected),
// "llm" (score ≥65), "human" (score <25), or "unknown".
func BuildScore(command string, args []string, now time.Time) (score int, label string) {
	if isAgentMode(args) {
		_ = updateStateForCommand(command, now) // best-effort; keeps latency tracking current
		return 100, "self_reported"
	}

	st, _ := loadState()
	nowMS := now.UnixMilli()

	st = ensureSession(st, nowMS, now)

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

// isAgentMode returns true when the caller has self-identified as an LLM agent,
// either via the --agent flag or a known agent environment variable.
//
// Cobra serialises bool flags as "--agent", "--agent=true", or "--agent=false".
// We scan all args and honour the last explicit value (matching Cobra's last-wins
// semantics). An explicit "--agent=false" suppresses env-var detection.
func isAgentMode(args []string) bool {
	flagSeen, flagValue := false, false
	for _, a := range args {
		if a == "--agent" {
			flagSeen, flagValue = true, true
		} else if strings.HasPrefix(a, "--agent=") {
			val := strings.ToLower(strings.TrimPrefix(a, "--agent="))
			switch val {
			case "1", "t", "true":
				flagSeen, flagValue = true, true
			case "0", "f", "false":
				flagSeen, flagValue = true, false
			}
		}
	}
	if flagSeen {
		return flagValue
	}
	for _, env := range []string{"CLAUDECODE", "CURSOR_AGENT", "AIDER_MODEL", "CLINE", "WINDSURF_AGENT", "COPILOT_AGENT"} {
		if os.Getenv(env) != "" {
			return true
		}
	}
	return false
}

// ensureSession returns a valid session state, creating a new one if the existing
// state is nil, has never recorded a command, or has exceeded the inactivity timeout.
func ensureSession(st *state, nowMS int64, now time.Time) *state {
	if st == nil || st.LastCommandUnixMilli == 0 || now.Sub(time.UnixMilli(st.LastCommandUnixMilli)) > sessionInactivityTimeout {
		return &state{SessionID: strconv.FormatInt(nowMS, 10)}
	}
	return st
}

// updateStateForCommand updates the latency-tracking state file for a command
// that was detected as self-reported, so subsequent heuristic calls remain accurate.
func updateStateForCommand(command string, now time.Time) error {
	st, _ := loadState()
	nowMS := now.UnixMilli()
	st = ensureSession(st, nowMS, now)
	st.LastCommandUnixMilli = nowMS
	if isDiagnosticCommand(command) {
		st.LastDiagnosticUnixMilli = nowMS
	}
	return saveState(st)
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

// statePathOverride allows tests to redirect the state file to an isolated location.
var statePathOverride string

func statePath() string {
	if statePathOverride != "" {
		return statePathOverride
	}
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
