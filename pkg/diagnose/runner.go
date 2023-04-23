// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnose

import (
	"fmt"
	"io"
	"sort"

	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/cihub/seelog"
	"github.com/fatih/color"
)

func init() {
	diagnosis.Register("connectivity-datadog-autodiscovery", diagnoseMetadataAutodiscoveryConnectivity)
}

// Overall running statistics
type diangosisCounters struct {
	totalCnt   int
	successCnt int
	failCnt    int
	warningCnt int
	errorCnt   int
	skippedCnt int
}

// Output summary
func outputSummary(w io.Writer, c diangosisCounters) {
	fmt.Fprintf(w, "-------------------------\n  Total:%d", c.totalCnt)
	if c.successCnt > 0 {
		fmt.Fprintf(w, ", Success:%d", c.successCnt)
	}
	if c.failCnt > 0 {
		fmt.Fprintf(w, ", Fail:%d", c.failCnt)
	}
	if c.warningCnt > 0 {
		fmt.Fprintf(w, ", Warning:%d", c.warningCnt)
	}
	if c.errorCnt > 0 {
		fmt.Fprintf(w, ", Error:%d", c.errorCnt)
	}
	fmt.Fprint(w, "\n")
}

func incrementCounters(r diagnosis.DiagnosisResult, c *diangosisCounters) {
	c.totalCnt++

	if r == diagnosis.DiagnosisNotEnable {
		c.skippedCnt++
	} else if r == diagnosis.DiagnosisSuccess {
		c.successCnt++
	} else if r == diagnosis.DiagnosisFail {
		c.failCnt++
	} else if r == diagnosis.DiagnosisWarning {
		c.warningCnt++
	} else { //if d.Result == diagnosis.DiagnosisUnexpectedError
		c.errorCnt++
	}
}

func getDiagnosisResultForOutput(r diagnosis.DiagnosisResult) string {
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

func outputDiagnosis(w io.Writer, cfg diagnosis.DiagnoseConfig, result string, diagnosisIdx int, d diagnosis.Diagnosis) {
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
	if d.RawError != nil {
		// Do not output error for diagnosis.DiagnosisSuccess unless verbose
		if d.Result != diagnosis.DiagnosisSuccess || cfg.Verbose {
			fmt.Fprintf(w, "  Error: %s\n", d.RawError)
		}
	}

	fmt.Fprint(w, "\n")
}

func outputSkippedDiangosis(w io.Writer, cfg diagnosis.DiagnoseConfig, suitName string, c diangosisCounters, suiteAlreadyReported *bool, lastDot *bool) {

	if cfg.Verbose {
		outputSuiteIfNeeded(w, suitName, suiteAlreadyReported)
		fmt.Fprintf(w, "  [%d] SKIPPED\n", c.totalCnt)
	} else {
		outputDot(w, lastDot)
	}
}

func outputNewLineIfNeeded(w io.Writer, lastDot *bool) {
	if *lastDot {
		fmt.Fprintf(w, "\n")
		*lastDot = false
	}
}

func outputSuiteIfNeeded(w io.Writer, suiteName string, suiteAlreadyReported *bool) {
	if *suiteAlreadyReported == false {
		fmt.Fprintf(w, "==============\nSuite: %s\n", suiteName)
		*suiteAlreadyReported = true
	}
}

func outputDot(w io.Writer, lastDot *bool) {
	fmt.Fprint(w, ".")
	*lastDot = true
}

// Currently used only to match Diagnose Suite name. In future will be
// extended to diagnose name or category
func matchConfigFilters(cfg diagnosis.DiagnoseConfig, s string) bool {
	if len(cfg.Include) > 0 {
		for _, re := range cfg.Include {
			if re.MatchString(s) {
				return true
			}
		}

		return false
	}

	if len(cfg.Exclude) > 0 {
		for _, re := range cfg.Exclude {
			if re.MatchString(s) {
				return false
			}
		}
	}

	return true
}

// Enumerate registered Diagnose suites and get their diagnoses
// for human consumption
func ListAllStdOut(w io.Writer, diagCfg diagnosis.DiagnoseConfig) {
	if w != color.Output {
		color.NoColor = true
	}

	// Sort Diagnose by their suite names
	sortedSuites := make([]diagnosis.DiagnoseSuite, len(diagnosis.DiagnoseCatalog))
	copy(sortedSuites, diagnosis.DiagnoseCatalog)
	sort.Slice(sortedSuites, func(i, j int) bool {
		return sortedSuites[i].SuitName < sortedSuites[j].SuitName
	})

	fmt.Fprintf(w, "Diagnose suites ...\n")

	count := 0
	for _, ds := range sortedSuites {
		// Is it filtered?
		if matchConfigFilters(diagCfg, ds.SuitName) {
			count++
			fmt.Fprintf(w, "  %d. %s\n", count, ds.SuitName)
		}
	}
}

// Enumerate registered Diagnose suites and get their diagnoses
// for structural output
func RunAll(diagCfg diagnosis.DiagnoseConfig) []diagnosis.Diagnoses {
	// Filter Diagnose suite
	suites := make([]diagnosis.DiagnoseSuite, 0)
	for _, ds := range diagnosis.DiagnoseCatalog {
		if matchConfigFilters(diagCfg, ds.SuitName) {
			suites = append(suites, ds)
		}
	}

	suiteDiagnoses := make([]diagnosis.Diagnoses, 0)

	for _, ds := range suites {
		// Run particular diagnose
		diagnoses := ds.Diagnose(diagCfg)
		if len(diagnoses) > 0 {
			suiteDiagnoses = append(suiteDiagnoses, diagnosis.Diagnoses{
				SuiteName:      ds.SuitName,
				SuiteDiagnoses: diagnoses,
			})
		}
	}

	return suiteDiagnoses
}

// Enumerate registered Diagnose suites and get their diagnoses
// for human consumption
func RunAllStdOut(w io.Writer, diagCfg diagnosis.DiagnoseConfig) {
	if w != color.Output {
		color.NoColor = true
	}

	// Sort Diagnose by their suite names
	sortedSuites := make([]diagnosis.DiagnoseSuite, len(diagnosis.DiagnoseCatalog))
	copy(sortedSuites, diagnosis.DiagnoseCatalog)
	sort.Slice(sortedSuites, func(i, j int) bool {
		return sortedSuites[i].SuitName < sortedSuites[j].SuitName
	})

	fmt.Fprintf(w, "=== Starting diagnose ===\n")

	var c diangosisCounters

	lastDot := false
	for _, ds := range sortedSuites {
		// Is it filtered?
		if !matchConfigFilters(diagCfg, ds.SuitName) {
			continue
		}

		// Run partiocular diagnose
		diagnoses := ds.Diagnose(diagCfg)
		if diagnoses == nil {
			// No diagnoses are reported, move on to next Diagnose
			continue
		}

		suiteAlreadyReported := false
		for _, d := range diagnoses {
			incrementCounters(d.Result, &c)

			// Skipping not enabled diagnosis
			if d.Result == diagnosis.DiagnosisNotEnable {
				outputSkippedDiangosis(w, diagCfg, ds.SuitName, c, &suiteAlreadyReported, &lastDot)
				continue
			}

			if d.Result == diagnosis.DiagnosisSuccess && !diagCfg.Verbose {
				outputDot(w, &lastDot)
				continue
			}

			outputSuiteIfNeeded(w, ds.SuitName, &suiteAlreadyReported)

			outputNewLineIfNeeded(w, &lastDot)
			outputDiagnosis(w, diagCfg, getDiagnosisResultForOutput(d.Result), c.totalCnt, d)
		}
	}

	outputNewLineIfNeeded(w, &lastDot)
	outputSummary(w, c)
}

func diagnoseMetadataAutodiscoveryConnectivity(cfg diagnosis.DiagnoseConfig) []diagnosis.Diagnosis {
	if len(diagnosis.MetadataAvailCatalog) == 0 {
		return nil
	}

	var sortedDiagnosis []string
	for name := range diagnosis.MetadataAvailCatalog {
		sortedDiagnosis = append(sortedDiagnosis, name)
	}
	sort.Strings(sortedDiagnosis)

	diagnoses := make([]diagnosis.Diagnosis, 0)
	for _, name := range sortedDiagnosis {
		err := diagnosis.MetadataAvailCatalog[name]()

		// Will always add successful diagnosis because particular environment is auto-discovered
		// and may not exist and or configured but knowing if we can or cannot connect to it
		// could be still beneficial
		var diagnosisString string
		if err == nil {
			diagnosisString = fmt.Sprintf("Successfully connected to %s environment", name)
		} else {
			diagnosisString = fmt.Sprintf("[Ignore if not applied] %s", err.Error())
		}

		diagnoses = append(diagnoses, diagnosis.Diagnosis{
			Result:    diagnosis.DiagnosisSuccess,
			Name:      name,
			Diagnosis: diagnosisString,
		})
	}

	return diagnoses
}

// Runs all registered metadata availability checks, output it in writer
func RunMetadataAvail(w io.Writer) error {
	if w != color.Output {
		color.NoColor = true
	}

	// Use temporarily a custom logger to our Writer
	customLogger, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg - %Ns%n")
	if err != nil {
		return err
	}
	log.RegisterAdditionalLogger("diagnose", customLogger)
	defer log.UnregisterAdditionalLogger("diagnose")

	var sortedDiagnosis []string
	for name := range diagnosis.MetadataAvailCatalog {
		sortedDiagnosis = append(sortedDiagnosis, name)
	}
	sort.Strings(sortedDiagnosis)

	for _, name := range sortedDiagnosis {
		fmt.Fprintf(w, "=== Running %s diagnosis ===\n", color.BlueString(name))
		err := diagnosis.MetadataAvailCatalog[name]()
		statusString := color.GreenString("PASS")
		if err != nil {
			statusString = color.RedString("FAIL")
			log.Infof("diagnosis error for %s: %v", name, err)
		}
		log.Flush()
		fmt.Fprintf(w, "===> %s\n\n", statusString)
	}

	return nil
}
