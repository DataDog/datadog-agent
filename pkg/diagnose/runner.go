// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diagnose

import (
	"fmt"
	"io"
	"regexp"
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
type counters struct {
	total         int
	success       int
	fail          int
	warnings      int
	unexpectedErr int
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
	if d.RawError != nil {
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
	if *suiteAlreadyReported == false {
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

// Currently used only to match Diagnose Suite name. In future will be
// extended to diagnose name or category
func matchConfigFilters(cfg diagnosis.Config, s string) bool {
	if len(cfg.Include) > 0 && len(cfg.Exclude) > 0 {
		return matchRegExList(cfg.Include, s) && !matchRegExList(cfg.Exclude, s)
	} else if len(cfg.Include) > 0 {
		return matchRegExList(cfg.Include, s)
	} else if len(cfg.Exclude) > 0 {
		return !matchRegExList(cfg.Exclude, s)
	}
	return true
}

func getSortedDiagnoseSuites() []diagnosis.Suite {
	sortedSuites := make([]diagnosis.Suite, len(diagnosis.Catalog))
	copy(sortedSuites, diagnosis.Catalog)
	sort.Slice(sortedSuites, func(i, j int) bool {
		return sortedSuites[i].SuitName < sortedSuites[j].SuitName
	})
	return sortedSuites
}

// Enumerate registered Diagnose suites and get their diagnoses
// for human consumption
func ListAllStdOut(w io.Writer, diagCfg diagnosis.Config) {
	if w != color.Output {
		color.NoColor = true
	}

	sortedSuites := getSortedDiagnoseSuites()

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
func RunAll(diagCfg diagnosis.Config) []diagnosis.Diagnoses {
	// Filter Diagnose suite
	var suites []diagnosis.Suite
	for _, ds := range diagnosis.Catalog {
		if matchConfigFilters(diagCfg, ds.SuitName) {
			suites = append(suites, ds)
		}
	}

	var suiteDiagnoses []diagnosis.Diagnoses
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
func RunAllStdOut(w io.Writer, diagCfg diagnosis.Config) {
	if w != color.Output {
		color.NoColor = true
	}

	sortedSuites := getSortedDiagnoseSuites()

	fmt.Fprintf(w, "=== Starting diagnose ===\n")

	var c counters

	lastDot := false
	for _, ds := range sortedSuites {
		// Is it filtered?
		if !matchConfigFilters(diagCfg, ds.SuitName) {
			continue
		}

		// Run particular diagnose
		diagnoses := ds.Diagnose(diagCfg)
		if diagnoses == nil {
			// No diagnoses are reported, move on to next Diagnose
			continue
		}

		suiteAlreadyReported := false
		for _, d := range diagnoses {
			c.increment(d.Result)

			if d.Result == diagnosis.DiagnosisSuccess && !diagCfg.Verbose {
				outputDot(w, &lastDot)
				continue
			}

			outputSuiteIfNeeded(w, ds.SuitName, &suiteAlreadyReported)

			outputNewLineIfNeeded(w, &lastDot)
			outputDiagnosis(w, diagCfg, getDiagnosisResultForOutput(d.Result), c.total, d)
		}
	}

	outputNewLineIfNeeded(w, &lastDot)
	c.summary(w)
}

func diagnoseMetadataAutodiscoveryConnectivity(cfg diagnosis.Config) []diagnosis.Diagnosis {
	if len(diagnosis.MetadataAvailCatalog) == 0 {
		return nil
	}

	var sortedDiagnosis []string
	for name := range diagnosis.MetadataAvailCatalog {
		sortedDiagnosis = append(sortedDiagnosis, name)
	}
	sort.Strings(sortedDiagnosis)

	var diagnoses []diagnosis.Diagnosis
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
