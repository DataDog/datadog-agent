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
	Result    int    `json:"result"`
	RawError  string `json:"rawerror"`
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
	Summary counters     `json:"summary"`
}

var textDiagnosisRE = regexp.MustCompile(`^  (PASS|FAIL|WARNING|UNEXPECTED ERROR) (.+)$`)

func parseIncludeOutput(output, format string) (includeOutput, error) {
	var result includeOutput
	if format == "json" {
		// A missing summary is malformed, whereas an empty summary is valid for
		// a selected suite without configured checks.
		var envelope struct {
			Runs    []includeRun `json:"runs"`
			Summary *counters    `json:"summary"`
		}
		if err := json.Unmarshal([]byte(output), &envelope); err != nil {
			return result, err
		}
		if envelope.Summary == nil {
			return result, errors.New("diagnose JSON has no summary")
		}
		result.Runs, result.Summary = envelope.Runs, *envelope.Summary
		return result, nil
	}
	if !summaryRE.MatchString(output) {
		return result, errors.New("diagnose text has no summary")
	}
	result.Summary = getDiagnoseSummary(output)
	statuses := map[string]int{"PASS": 0, "FAIL": 1, "WARNING": 2, "UNEXPECTED ERROR": 3}
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		if name, ok := strings.CutPrefix(line, "Suite: "); ok {
			result.Runs = append(result.Runs, includeRun{Name: name})
		} else if match := textDiagnosisRE.FindStringSubmatch(line); match != nil {
			if len(result.Runs) == 0 {
				return result, errors.New("diagnose text has a result outside a suite")
			}
			run := &result.Runs[len(result.Runs)-1]
			run.Diagnoses = append(run.Diagnoses, includeDiagnosis{Result: statuses[match[1]], Name: match[2]})
		} else if rawError, ok := strings.CutPrefix(line, "  Error: "); ok && len(result.Runs) > 0 {
			diagnoses := result.Runs[len(result.Runs)-1].Diagnoses
			if len(diagnoses) > 0 {
				diagnoses[len(diagnoses)-1].RawError = rawError
			}
		}
	}
	return result, nil
}

func (result includeOutput) validate(expectedSuites []string) error {
	seen := make(map[string]bool)
	var actual counters
	for _, run := range result.Runs {
		allowed := false
		for _, expected := range expectedSuites {
			allowed = allowed || run.Name == expected
		}
		if !allowed || seen[run.Name] {
			return fmt.Errorf("unexpected or duplicate diagnose suite %q", run.Name)
		}
		if len(run.Diagnoses) == 0 {
			return fmt.Errorf("diagnose suite %q emitted no diagnoses", run.Name)
		}
		seen[run.Name] = true
		for _, diagnosis := range run.Diagnoses {
			actual.Total++
			switch diagnosis.Result {
			case 0:
				actual.Success++
			case 1:
				actual.Fail++
			case 2:
				actual.Warnings++
			case 3:
				actual.UnexpectedErr++
			default:
				return fmt.Errorf("invalid diagnose result %d", diagnosis.Result)
			}
		}
	}
	if actual != result.Summary {
		return fmt.Errorf("diagnose summary %+v does not match its results %+v", result.Summary, actual)
	}
	for _, expected := range expectedSuites {
		// This suite has no configured integration checks on the default host.
		// Other selections must produce results, so an ignored filter cannot pass.
		if expected != "check-datadog" && !seen[expected] {
			return fmt.Errorf("selected diagnose suite %q has no results", expected)
		}
	}
	return nil
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

// runIncludeSelection only retries a valid response containing transient
// connectivity failures. Parsing and selection failures consume no retry token.
func runIncludeSelection(run func() (includeOutput, error), suites []string, retryAvailable *bool) (includeOutput, error) {
	result, err := run()
	if err != nil {
		return result, err
	}
	if err := result.validate(suites); err != nil {
		return result, err
	}
	if *retryAvailable && result.hasOnlyTransientFailures() {
		*retryAvailable = false
		result, err = run()
		if err == nil {
			err = result.validate(suites)
		}
	}
	return result, err
}

func (result includeOutput) hasOnlyTransientFailures() bool {
	if result.Summary.Fail == 0 || result.Summary.UnexpectedErr != 0 {
		return false
	}
	for _, run := range result.Runs {
		for _, diagnosis := range run.Diagnoses {
			if diagnosis.Result != 1 {
				continue
			}
			if !strings.HasPrefix(run.Name, "connectivity-datadog-") {
				return false
			}
			transient := false
			for _, signature := range []string{"context deadline exceeded", "i/o timeout", "connection reset by peer", "connection refused"} {
				transient = transient || strings.Contains(strings.ToLower(diagnosis.RawError), signature)
			}
			if !transient {
				return false
			}
		}
	}
	return true
}

var summaryRE = createSummaryRegex()

func createSummaryRegex() *regexp.Regexp {
	// success, fail, warning and error are optional in the diagnose output (they're printed when their value != 0)
	successRegex := `(?:, Success:(?P<success>\d+))?`
	failRegex := `(?:, Fail:(?P<fail>\d+))?`
	warningRegex := `(?:, Warning:(?P<warning>\d+))?`
	errorRegex := `(?:, Error:(?P<error>\d+))?`
	regexTemplate := `Total:(?P<total>\d+)` + successRegex + failRegex + warningRegex + errorRegex
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
