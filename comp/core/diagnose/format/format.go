// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package format provides the output format for the diagnose suite.
package format

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/fatih/color"

	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
)

// Text outputs the diagnose result in a human readable format
func Text(w io.Writer, diagCfg diagnose.Config, diagnoseResult *diagnose.Result) error {
	if w != color.Output {
		color.NoColor = true
	}

	fmt.Fprintf(w, "=== Starting diagnose ===\n")

	lastDot := false
	idx := 0
	for _, ds := range diagnoseResult.Runs {
		suiteAlreadyReported := false
		for _, d := range ds.Diagnoses {
			idx++
			if d.Status == diagnose.DiagnosisSuccess && !diagCfg.Verbose {
				outputDot(w, &lastDot)
				continue
			}

			outputSuiteIfNeeded(w, ds.Name, &suiteAlreadyReported)

			outputNewLineIfNeeded(w, &lastDot)
			outputDiagnosis(w, diagCfg, d.Status.ToString(true), idx, d)
		}
	}

	outputNewLineIfNeeded(w, &lastDot)
	summary(w, diagnoseResult.Summary)

	return nil
}

// JSON outputs the diagnose result in JSON format
func JSON(w io.Writer, diagnoseResult *diagnose.Result) error {
	diagJSON, err := json.MarshalIndent(diagnoseResult, "", "  ")
	if err != nil {
		body, _ := json.Marshal(map[string]string{"error": fmt.Sprintf("marshalling diagnose results to JSON: %s", err)})
		fmt.Fprintln(w, string(body))
		return err
	}
	fmt.Fprintln(w, string(diagJSON))

	return nil
}

func outputDot(w io.Writer, lastDot *bool) {
	fmt.Fprint(w, ".")
	*lastDot = true
}

func outputSuiteIfNeeded(w io.Writer, suiteName string, suiteAlreadyReported *bool) {
	if !*suiteAlreadyReported {
		fmt.Fprintf(w, "==============\nSuite: %s\n", suiteName)
		*suiteAlreadyReported = true
	}
}

func outputNewLineIfNeeded(w io.Writer, lastDot *bool) {
	if *lastDot {
		fmt.Fprintf(w, "\n")
		*lastDot = false
	}
}

func outputDiagnosis(w io.Writer, cfg diagnose.Config, result string, diagnosisIdx int, d diagnose.Diagnosis) {
	// Running index (1, 2, 3, etc)
	fmt.Fprintf(w, "%d. --------------\n", diagnosisIdx)

	// [Required] Diagnosis name (and category if it us not empty)
	if len(d.Category) > 0 {
		fmt.Fprintf(w, "  %s [%s] %s\n", result, d.Category, d.Name)
	} else {
		fmt.Fprintf(w, "  %s %s\n", result, d.Name)
	}

	// [Optional] For verbose output diagnosis description
	if cfg.Verbose {
		if len(d.Description) > 0 {
			fmt.Fprintf(w, "  Description: %s\n", d.Description)
		}
	}

	// [Required] Diagnosis
	fmt.Fprintf(w, "  Diagnosis: %s\n", d.Diagnosis)

	// [Optional] Remediation if exists
	if len(d.Remediation) > 0 {
		fmt.Fprintf(w, "  Remediation: %s\n", d.Remediation)
	}

	// [Optional] Error
	if len(d.RawError) > 0 {
		// Do not output error for diagnose.DiagnosisSuccess unless verbose
		if d.Status != diagnose.DiagnosisSuccess || cfg.Verbose {
			fmt.Fprintf(w, "  Error: %s\n", d.RawError)
		}
	}

	fmt.Fprint(w, "\n")
}

func summary(w io.Writer, c diagnose.Counters) {
	fmt.Fprintf(w, "-------------------------\n  Total:%d", c.Total)
	if c.Success > 0 {
		fmt.Fprintf(w, ", Success:%d", c.Success)
	}
	if c.Fail > 0 {
		fmt.Fprintf(w, ", Fail:%d", c.Fail)
	}
	if c.Warnings > 0 {
		fmt.Fprintf(w, ", Warning:%d", c.Warnings)
	}
	if c.UnexpectedErr > 0 {
		fmt.Fprintf(w, ", Error:%d", c.UnexpectedErr)
	}
	fmt.Fprint(w, "\n")
}
