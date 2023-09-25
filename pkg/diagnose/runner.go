// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnose

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
	_ "github.com/DataDog/datadog-agent/pkg/diagnose/connectivity" // no direct calls to connectivity but there is a callback
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/fatih/color"
)

// Overall running statistics
type counters struct {
	total         int
	success       int
	fail          int
	warnings      int
	unexpectedErr int
}

// diagnose suite filter
type diagSuiteFilter struct {
	include []*regexp.Regexp
	exclude []*regexp.Regexp
}

// Output summary
func (c *counters) summary(w io.Writer) {
	fmt.Fprintf(w, "-------------------------\n  Total:%d", c.total)
	if c.success > 0 {
		fmt.Fprintf(w, ", Success:%d", c.success)
	}
	if c.fail > 0 {
		fmt.Fprintf(w, ", Fail:%d", c.fail)
	}
	if c.warnings > 0 {
		fmt.Fprintf(w, ", Warning:%d", c.warnings)
	}
	if c.unexpectedErr > 0 {
		fmt.Fprintf(w, ", Error:%d", c.unexpectedErr)
	}
	fmt.Fprint(w, "\n")
}

func (c *counters) increment(r diagnosis.Result) {
	c.total++

	if r == diagnosis.DiagnosisSuccess {
		c.success++
	} else if r == diagnosis.DiagnosisFail {
		c.fail++
	} else if r == diagnosis.DiagnosisWarning {
		c.warnings++
	} else if r == diagnosis.DiagnosisUnexpectedError {
		c.unexpectedErr++
	}
}

func getDiagnosisResultForOutput(r diagnosis.Result) string {
	var result string
	if r == diagnosis.DiagnosisSuccess {
		result = color.GreenString("PASS")
	} else if r == diagnosis.DiagnosisFail {
		result = color.RedString("FAIL")
	} else if r == diagnosis.DiagnosisWarning {
		result = color.YellowString("WARNING")
	} else { //if d.Result == diagnosis.DiagnosisUnexpectedError
		result = color.HiRedString("UNEXPECTED ERROR")
	}

	return result
}

func outputDiagnosis(w io.Writer, cfg diagnosis.Config, result string, diagnosisIdx int, d diagnosis.Diagnosis) {
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
		// Do not output error for diagnosis.DiagnosisSuccess unless verbose
		if d.Result != diagnosis.DiagnosisSuccess || cfg.Verbose {
			fmt.Fprintf(w, "  Error: %s\n", d.RawError)
		}
	}

	fmt.Fprint(w, "\n")
}

func outputNewLineIfNeeded(w io.Writer, lastDot *bool) {
	if *lastDot {
		fmt.Fprintf(w, "\n")
		*lastDot = false
	}
}

func outputSuiteIfNeeded(w io.Writer, suiteName string, suiteAlreadyReported *bool) {
	if !*suiteAlreadyReported {
		fmt.Fprintf(w, "==============\nSuite: %s\n", suiteName)
		*suiteAlreadyReported = true
	}
}

func outputDot(w io.Writer, lastDot *bool) {
	fmt.Fprint(w, ".")
	*lastDot = true
}

func matchRegExList(regexList []*regexp.Regexp, s string) bool {
	for _, re := range regexList {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func strToRegexList(patterns []string) ([]*regexp.Regexp, error) {
	if len(patterns) > 0 {
		res := make([]*regexp.Regexp, 0)
		for _, pattern := range patterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				return nil, fmt.Errorf("failed to compile regex pattern %s: %s", pattern, err.Error())
			}
			res = append(res, re)
		}
		return res, nil
	}
	return nil, nil
}

// Currently used only to match Diagnose Suite name. In future will be
// extended to diagnose name or category
func matchConfigFilters(filter diagSuiteFilter, s string) bool {
	if len(filter.include) > 0 && len(filter.exclude) > 0 {
		return matchRegExList(filter.include, s) && !matchRegExList(filter.exclude, s)
	} else if len(filter.include) > 0 {
		return matchRegExList(filter.include, s)
	} else if len(filter.exclude) > 0 {
		return !matchRegExList(filter.exclude, s)
	}
	return true
}

func getSortedAndFilteredDiagnoseSuites(diagCfg diagnosis.Config) ([]diagnosis.Suite, error) {

	var filter diagSuiteFilter
	var err error

	if len(diagCfg.Include) > 0 {
		filter.include, err = strToRegexList(diagCfg.Include)
		if err != nil {
			includes := strings.Join(diagCfg.Include, " ")
			return nil, fmt.Errorf("invalid --include option value(s) provided (%s) compiled with error: %w", includes, err)
		}
	}

	if len(diagCfg.Exclude) > 0 {
		filter.exclude, err = strToRegexList(diagCfg.Exclude)
		if err != nil {
			excludes := strings.Join(diagCfg.Exclude, " ")
			return nil, fmt.Errorf("invalid --exclude option value(s) provided (%s) compiled with error: %w", excludes, err)
		}
	}

	sortedSuites := make([]diagnosis.Suite, len(diagnosis.Catalog))
	copy(sortedSuites, diagnosis.Catalog)
	sort.Slice(sortedSuites, func(i, j int) bool {
		return sortedSuites[i].SuitName < sortedSuites[j].SuitName
	})

	var sortedFilteredSuites []diagnosis.Suite
	for _, ds := range sortedSuites {
		if matchConfigFilters(filter, ds.SuitName) {
			sortedFilteredSuites = append(sortedFilteredSuites, ds)
		}
	}

	return sortedFilteredSuites, nil
}

func getSuiteDiagnoses(ds diagnosis.Suite, diagCfg diagnosis.Config, senderManager sender.SenderManager) []diagnosis.Diagnosis {
	diagnoses := ds.Diagnose(diagCfg, senderManager)

	// validate each diagnoses
	for i, d := range diagnoses {
		if d.Result < diagnosis.DiagnosisResultMIN ||
			d.Result > diagnosis.DiagnosisResultMAX ||
			len(d.Name) == 0 ||
			len(d.Diagnosis) == 0 {

			if len(d.RawError) > 0 {
				// If error already reported, append to it
				diagnoses[i].RawError = fmt.Sprintf("required diagnosis fields are invalid. Result:%d, Name:%s, Diagnosis:%s. Reported Error: %s",
					d.Result, d.Name, d.Diagnosis, d.RawError)
			} else {
				diagnoses[i].RawError = fmt.Sprintf("required diagnosis fields are invalid. Result:%d, Name:%s, Diagnosis:%s", d.Result, d.Name, d.Diagnosis)
			}

			diagnoses[i].Result = diagnosis.DiagnosisUnexpectedError
			if len(d.Name) == 0 {
				diagnoses[i].Name = ds.SuitName
			}
		}
	}

	return diagnoses
}

// Enumerate registered Diagnose suites and get their diagnoses
// for human consumption
func ListStdOut(w io.Writer, diagCfg diagnosis.Config) {
	if w != color.Output {
		color.NoColor = true
	}

	fmt.Fprintf(w, "Diagnose suites ...\n")

	sortedSuites, err := getSortedAndFilteredDiagnoseSuites(diagCfg)
	if err != nil {
		fmt.Fprintf(w, "Failed to get list of diagnose suites. Validate your command line options. Error: %s\n", err.Error())
		return
	}

	count := 0
	for _, ds := range sortedSuites {
		count++
		fmt.Fprintf(w, "  %d. %s\n", count, ds.SuitName)
	}
}

// Enumerate registered Diagnose suites and get their diagnoses
// for structural output
func getDiagnosesFromCurrentProcess(diagCfg diagnosis.Config, senderManager sender.SenderManager) ([]diagnosis.Diagnoses, error) {
	suites, err := getSortedAndFilteredDiagnoseSuites(diagCfg)
	if err != nil {
		return nil, err
	}

	var suitesDiagnoses []diagnosis.Diagnoses
	for _, ds := range suites {
		// Run particular diagnose
		diagnoses := getSuiteDiagnoses(ds, diagCfg, senderManager)
		if len(diagnoses) > 0 {
			suitesDiagnoses = append(suitesDiagnoses, diagnosis.Diagnoses{
				SuiteName:      ds.SuitName,
				SuiteDiagnoses: diagnoses,
			})
		}
	}

	return suitesDiagnoses, nil
}

func requestDiagnosesFromAgentProcess(diagCfg diagnosis.Config) ([]diagnosis.Diagnoses, error) {
	// Get client to Agent's RPC call
	c := util.GetClient(false)
	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return nil, fmt.Errorf("error getting IPC address for the agent: %w", err)
	}

	// Make sure we have a session token (for privileged information)
	if err = util.SetAuthToken(); err != nil {
		return nil, fmt.Errorf("auth error: %w", err)
	}

	// Form call end-point
	diagnoseUrl := fmt.Sprintf("https://%v:%v/agent/diagnose", ipcAddress, pkgconfig.Datadog.GetInt("cmd_port"))

	// Serialized diag config to pass it to Agent execution context
	var cfgSer []byte
	if cfgSer, err = json.Marshal(diagCfg); err != nil {
		return nil, fmt.Errorf("error while encoding diagnose configuration: %s", err)
	}

	// Run diagnose code inside Agent process
	var response []byte
	response, err = util.DoPost(c, diagnoseUrl, "application/json", bytes.NewBuffer(cfgSer))
	if err != nil {
		if response != nil && string(response) != "" {
			return nil, fmt.Errorf("error getting diagnoses from running agent: %sn", string(response))
		}
		return nil, fmt.Errorf("the agent was unable to get diagnoses from running agent: %w", err)
	}

	// Deserialize results
	var diagnoses []diagnosis.Diagnoses
	err = json.NewDecoder(bytes.NewReader(response)).Decode(&diagnoses)
	if err != nil {
		return nil, fmt.Errorf("error while decoding diagnose results returned from Agent: %w", err)
	}

	return diagnoses, nil
}

// Run runs diagnoses.
func Run(diagCfg diagnosis.Config, senderManager sender.SenderManager) ([]diagnosis.Diagnoses, error) {

	// Make remote call to get diagnoses
	if !diagCfg.RunLocal {
		return requestDiagnosesFromAgentProcess(diagCfg)
	}

	// Collect local diagnoses
	diagnoses, err := getDiagnosesFromCurrentProcess(diagCfg, senderManager)
	if err != nil {
		return nil, err
	}

	return diagnoses, nil
}

// Enumerate registered Diagnose suites and get their diagnoses
// for human consumption
func RunStdOut(w io.Writer, diagCfg diagnosis.Config, senderManager sender.SenderManager) error {
	if w != color.Output {
		color.NoColor = true
	}

	fmt.Fprintf(w, "=== Starting diagnose ===\n")

	diagnoses, err := Run(diagCfg, senderManager)
	if err != nil && !diagCfg.RunLocal {
		fmt.Fprintln(w, color.YellowString(fmt.Sprintf("Error running diagnose in Agent process: %s", err)))
		fmt.Fprintln(w, "Running diagnose command locally (may take extra time to run checks locally) ...")

		// attempt to do so locally
		diagCfg.RunLocal = true
		diagnoses, err = Run(diagCfg, senderManager)
	}

	if err != nil {
		fmt.Fprintln(w, color.RedString(fmt.Sprintf("Error running diagnose: %s", err)))
		return err
	}

	var c counters

	lastDot := false
	for _, ds := range diagnoses {
		suiteAlreadyReported := false
		for _, d := range ds.SuiteDiagnoses {
			c.increment(d.Result)

			if d.Result == diagnosis.DiagnosisSuccess && !diagCfg.Verbose {
				outputDot(w, &lastDot)
				continue
			}

			outputSuiteIfNeeded(w, ds.SuiteName, &suiteAlreadyReported)

			outputNewLineIfNeeded(w, &lastDot)
			outputDiagnosis(w, diagCfg, getDiagnosisResultForOutput(d.Result), c.total, d)
		}
	}

	outputNewLineIfNeeded(w, &lastDot)
	c.summary(w)

	return nil
}
