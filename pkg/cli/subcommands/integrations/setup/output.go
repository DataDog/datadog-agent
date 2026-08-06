// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package setup

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// RenderText writes human-readable output for a setup result to w.
func RenderText(w io.Writer, result *SetupResult) {
	if result.RestartNeeded {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "RESTART REQUIRED")
		fmt.Fprintln(w, "The following settings won't take effect until PostgreSQL is restarted:")
		for _, op := range result.Operations {
			if op.RequiresRestart && op.Status == StatusCompleted {
				fmt.Fprintf(w, "  * %s\n", op.SettingName)
			}
		}
		fmt.Fprintln(w)
	}

	if result.ManualSteps {
		fmt.Fprintln(w, "Manual steps required before per-DB setup will fully take effect:")
		fmt.Fprintln(w)
		for _, op := range result.Operations {
			if op.Kind == KindManualStep {
				fmt.Fprintf(w, "  [%s]  %s\n", op.SettingName, op.ManualInstruction)
			}
		}
		fmt.Fprintln(w)
	}

	for _, op := range result.Operations {
		if op.Kind == KindManualStep {
			continue
		}
		prefix := statusPrefix(op.Status)
		detail := op.Description
		if op.Database != "" {
			detail = fmt.Sprintf("[%s] %s", op.Database, op.Description)
		}
		fmt.Fprintf(w, "  %s %s\n", prefix, detail)
		if op.Status == StatusFailed && op.Error != "" {
			fmt.Fprintf(w, "       error: %s\n", op.Error)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "Outcome: %s\n", result.Outcome)
}

func statusPrefix(s OperationStatus) string {
	switch s {
	case StatusCompleted:
		return "[OK]"
	case StatusSkipped:
		return "[SKIP]"
	case StatusFailed:
		return "[FAIL]"
	case StatusPending:
		return "[PENDING]"
	case StatusManual:
		return "[MANUAL]"
	default:
		return "[?]"
	}
}

// jsonResult is the JSON-serialisable form of SetupResult for --output json.
type jsonResult struct {
	Flavor         string   `json:"flavor"`
	PGVersion      int      `json:"pg_version"`
	RestartNeeded  bool     `json:"restart_needed"`
	ManualSteps    bool     `json:"manual_steps"`
	Outcome        string   `json:"outcome"`
	Operations     []jsonOp `json:"operations"`
	ManualStepList []string `json:"manual_step_list,omitempty"`
}

type jsonOp struct {
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Database    string `json:"database,omitempty"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
}

// RenderJSON writes JSON output for a setup result to w.
func RenderJSON(w io.Writer, result *SetupResult) error {
	ops := make([]jsonOp, 0, len(result.Operations))
	var manualList []string
	for _, op := range result.Operations {
		if op.Kind == KindManualStep {
			manualList = append(manualList, op.ManualInstruction)
			continue
		}
		ops = append(ops, jsonOp{
			Kind:        string(op.Kind),
			Description: op.Description,
			Database:    op.Database,
			Status:      string(op.Status),
			Error:       op.Error,
		})
	}
	jr := jsonResult{
		Flavor:         string(result.Flavor),
		PGVersion:      result.PGVersion,
		RestartNeeded:  result.RestartNeeded,
		ManualSteps:    result.ManualSteps,
		Outcome:        result.Outcome,
		Operations:     ops,
		ManualStepList: manualList,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(jr)
}

// Render dispatches to RenderText or RenderJSON based on the output format.
func Render(w io.Writer, result *SetupResult, format string) error {
	switch strings.ToLower(format) {
	case "json":
		return RenderJSON(w, result)
	default:
		RenderText(w, result)
		return nil
	}
}
