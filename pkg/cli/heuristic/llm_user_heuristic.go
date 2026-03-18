// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package heuristic

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	sessionInactivityTimeout = 10 * time.Minute
)

var errNoPPID = errors.New("parent process id is unavailable")

type ScorePayload struct {
	Command                     string   `json:"command"`
	CommandArgs                 []string `json:"command_args"`
	HeuristicScore              int      `json:"heuristic_score"`
	ShellAgingMS                int64    `json:"shell_aging_ms"`
	InterCommandLatencyMS       *int64   `json:"inter_command_latency_ms,omitempty"`
	FirstInvocationInSession    bool     `json:"first_invocation_in_session"`
	UsedMachineFirstFlags       bool     `json:"used_machine_first_flags"`
	ParentProcessHasPipeFilters bool     `json:"parent_process_has_pipe_filters"`
	ParentProcessCommand        string   `json:"parent_process_command,omitempty"`
	TriggeredSignals            []string `json:"triggered_signals"`
	SessionID                   string   `json:"session_id"`
	SessionCommandCount         int      `json:"session_command_count"`
	Timestamp                   int64    `json:"timestamp"`
}

type state struct {
	SessionID               string `json:"session_id"`
	SessionStartUnixMilli   int64  `json:"session_start_unix_milli"`
	LastCommandUnixMilli    int64  `json:"last_command_unix_milli"`
	LastComplexUnixMilli    int64  `json:"last_complex_unix_milli"`
	SessionCommandCount     int    `json:"session_command_count"`
	FirstCommandSeenInState bool   `json:"first_command_seen_in_state"`
}

func BuildScorePayload(command string, commandArgs []string, now time.Time) (*ScorePayload, error) {
	st, _ := loadState()
	nowUnixMilli := now.UnixMilli()

	if st == nil || st.LastCommandUnixMilli == 0 || now.Sub(time.UnixMilli(st.LastCommandUnixMilli)) > sessionInactivityTimeout {
		st = &state{
			SessionID:             fmt.Sprintf("%d", nowUnixMilli),
			SessionStartUnixMilli: nowUnixMilli,
		}
	}

	firstInvocation := st.SessionCommandCount == 0
	shellAgingMS := nowUnixMilli - st.SessionStartUnixMilli
	if shellAgingMS < 0 {
		shellAgingMS = 0
	}

	var interCommandLatencyMS *int64
	isComplexCommand := isComplexDiagnosticCommand(command)
	if isComplexCommand && st.LastComplexUnixMilli > 0 {
		diff := nowUnixMilli - st.LastComplexUnixMilli
		if diff < 0 {
			diff = 0
		}
		interCommandLatencyMS = &diff
	}

	usedMachineFirstFlags := firstInvocation && hasMachineFirstFlags(commandArgs)
	parentCommand, _ := parentProcessCommandline()
	hasPipeFilters := hasPipeToFilters(parentCommand)

	score := 0
	triggeredSignals := make([]string, 0, 8)

	if interCommandLatencyMS != nil && *interCommandLatencyMS < 500 {
		score += 60
		triggeredSignals = append(triggeredSignals, "complex_diag_sub_500ms")
	}
	if shellAgingMS < 2000 {
		score += 30
		triggeredSignals = append(triggeredSignals, "shell_aging_sub_2s")
	}
	if usedMachineFirstFlags {
		score += 6
		triggeredSignals = append(triggeredSignals, "machine_first_flags_on_first_invocation")
	}
	if hasPipeFilters {
		score += 4
		triggeredSignals = append(triggeredSignals, "pipe_to_structured_filters")
	}
	if score > 100 {
		score = 100
	}

	st.LastCommandUnixMilli = nowUnixMilli
	if isComplexCommand {
		st.LastComplexUnixMilli = nowUnixMilli
	}
	st.SessionCommandCount++
	st.FirstCommandSeenInState = true
	_ = saveState(st)

	return &ScorePayload{
		Command:                     command,
		CommandArgs:                 commandArgs,
		HeuristicScore:              score,
		ShellAgingMS:                shellAgingMS,
		InterCommandLatencyMS:       interCommandLatencyMS,
		FirstInvocationInSession:    firstInvocation,
		UsedMachineFirstFlags:       usedMachineFirstFlags,
		ParentProcessHasPipeFilters: hasPipeFilters,
		ParentProcessCommand:        parentCommand,
		TriggeredSignals:            triggeredSignals,
		SessionID:                   st.SessionID,
		SessionCommandCount:         st.SessionCommandCount,
		Timestamp:                   nowUnixMilli,
	}, nil
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

func isComplexDiagnosticCommand(command string) bool {
	switch strings.TrimSpace(strings.ToLower(command)) {
	case "agent status", "status", "agent health", "health":
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

func parentProcessCommandline() (string, error) {
	ppid := syscall.Getppid()
	if ppid <= 0 {
		return "", errNoPPID
	}

	cmdlinePath := filepath.Join("/proc", strconv.Itoa(ppid), "cmdline")
	body, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return "", err
	}

	rawParts := strings.Split(string(body), "\x00")
	parts := make([]string, 0, len(rawParts))
	for _, p := range rawParts {
		if p == "" {
			continue
		}
		parts = append(parts, p)
	}

	return strings.Join(parts, " "), nil
}

func hasPipeToFilters(commandline string) bool {
	if commandline == "" {
		return false
	}

	c := strings.ToLower(commandline)
	if strings.Contains(c, "|jq") || strings.Contains(c, "| jq") {
		return true
	}
	if strings.Contains(c, "|grep") || strings.Contains(c, "| grep") {
		return true
	}
	return strings.Contains(c, "|yq") || strings.Contains(c, "| yq")
}
