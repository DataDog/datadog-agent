// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package diagnose provides helpers for the diagnose subcommand E2E tests.
package diagnose

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// counters contains the count of the diagnosis results
type counters struct {
	Total         int `json:"total,omitempty"`
	Success       int `json:"success,omitempty"`
	Fail          int `json:"fail,omitempty"`
	Warnings      int `json:"warnings,omitempty"`
	UnexpectedErr int `json:"unexpected_error,omitempty"`
}

type includeDiagnosis struct {
	Name      string `json:"name"`
	CheckName string `json:"check_name"`
	Category  string `json:"category"`
}

type includeRun struct {
	Name      string             `json:"suite_name"`
	Diagnoses []includeDiagnosis `json:"diagnoses"`
}

type includeOutput struct {
	Runs    []includeRun `json:"runs"`
	Summary *counters    `json:"summary"`
}

var textDiagnosisRE = regexp.MustCompile(`^  (?:PASS|FAIL|WARNING|UNEXPECTED ERROR) (.+)$`)

func parseIncludeOutput(output, format string) (includeOutput, error) {
	var result includeOutput
	if format == "json" {
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			return result, err
		}
		if result.Summary == nil {
			return result, errors.New("diagnose JSON has no summary")
		}
		return result, nil
	}
	if !summaryRE.MatchString(output) {
		return result, errors.New("diagnose text has no summary")
	}
	summary := getDiagnoseSummary(output)
	result.Summary = &summary
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		if name, ok := strings.CutPrefix(line, "Suite: "); ok {
			result.Runs = append(result.Runs, includeRun{Name: name})
		} else if match := textDiagnosisRE.FindStringSubmatch(line); match != nil {
			if len(result.Runs) == 0 {
				return result, errors.New("diagnose text has a result outside a suite")
			}
			run := &result.Runs[len(result.Runs)-1]
			run.Diagnoses = append(run.Diagnoses, includeDiagnosis{Name: match[1]})
		}
	}
	return result, nil
}

// identities preserves diagnosis membership and multiplicity without tying
// selection equivalence to the outcomes of independent network requests.
func (result includeOutput) identities() map[[4]string]int {
	identities := make(map[[4]string]int)
	for _, run := range result.Runs {
		for _, diagnosis := range run.Diagnoses {
			identities[[4]string{run.Name, diagnosis.Name, diagnosis.CheckName, diagnosis.Category}]++
		}
	}
	return identities
}

var summaryRE = createSummaryRegex()

func createSummaryRegex() *regexp.Regexp {
	// success, fail, warning and error are optional in the diagnose output (they're printed when their value != 0)
	successRegex := `(?:, Success:(?P<success>\d+))?`
	failRegex := `(?:, Fail:(?P<fail>\d+))?`
	warningRegex := `(?:, Warning:(?P<warning>\d+))?`
	errorRegex := `(?:, Error:(?P<error>\d+))?`
	// Diagnostic details may contain arbitrary multiline endpoint responses,
	// including counters and separators. Only the final summary block counts.
	regexTemplate := `(?m)^-------------------------\r?\n  Total:(?P<total>\d+)` + successRegex + failRegex + warningRegex + errorRegex + `(?:\r?\n)?\z`
	return regexp.MustCompile(regexTemplate)
}

// getDiagnoseSummary parses the diagnose output and returns a struct containing number of success, fail, error and warning
func getDiagnoseSummary(diagnoseOutput string) counters {
	matches := summaryRE.FindStringSubmatch(diagnoseOutput)
	return counters{
		Total:         getRegexGroupValue(summaryRE, matches, "total"),
		Success:       getRegexGroupValue(summaryRE, matches, "success"),
		Warnings:      getRegexGroupValue(summaryRE, matches, "warning"),
		Fail:          getRegexGroupValue(summaryRE, matches, "fail"),
		UnexpectedErr: getRegexGroupValue(summaryRE, matches, "error"),
	}
}

// getRegexGroupValue returns the value of a specific named group, or 0 if there is no value for this group
func getRegexGroupValue(re *regexp.Regexp, matches []string, groupName string) int {
	index := re.SubexpIndex(groupName)
	if index < 0 || index >= len(matches) {
		panic(fmt.Sprintf("An error occurred while looking for group '%v' in diagnose output", groupName))
	}
	val, err := strconv.Atoi(matches[index])
	if err != nil {
		return 0
	}

	return val
}
