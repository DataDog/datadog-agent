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
	"runtime"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	integrations "github.com/DataDog/datadog-agent/comp/logs/integrations/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/util/optional"

	"github.com/fatih/color"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/diagnose/connectivity"
	"github.com/DataDog/datadog-agent/pkg/diagnose/diagnosis"
	"github.com/DataDog/datadog-agent/pkg/diagnose/ports"
)

// diagnose suite filter
type diagSuiteFilter struct {
	include []*regexp.Regexp
	exclude []*regexp.Regexp
}

// Output summary
func summary(w io.Writer, c diagnosis.Counters) {
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

func getSortedAndFilteredDiagnoseSuites[T any](diagCfg diagnosis.Config, values []T, getName func(T) string) ([]T, error) {
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

	sortedValues := make([]T, len(values))
	copy(sortedValues, values)
	sort.Slice(sortedValues, func(i, j int) bool {
		return getName(sortedValues[i]) < getName(sortedValues[j])
	})

	var sortedFilteredValues []T
	for _, ds := range sortedValues {
		if matchConfigFilters(filter, getName(ds)) {
			sortedFilteredValues = append(sortedFilteredValues, ds)
		}
	}

	return sortedFilteredValues, nil
}

func getSuiteDiagnoses(ds diagnosis.Suite) []diagnosis.Diagnosis {
	diagnoses := ds.Diagnose()

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

// ListStdOut enumerates registered Diagnose suites and get their diagnoses
// for human consumption
//
//nolint:revive // TODO(CINT) Fix revive linter
func ListStdOut(w io.Writer, diagCfg diagnosis.Config) {
	if w != color.Output {
		color.NoColor = true
	}

	fmt.Fprintf(w, "Diagnose suites ...\n")

	sortedSuitesName, err := getSortedAndFilteredDiagnoseSuites(diagCfg, getCheckNames(diagCfg), func(name string) string { return name })
	if err != nil {
		fmt.Fprintf(w, "Failed to get list of diagnose suites. Validate your command line options. Error: %s\n", err.Error())
		return
	}

	count := 0
	for _, suiteName := range sortedSuitesName {
		count++
		fmt.Fprintf(w, "  %d. %s\n", count, suiteName)
	}
}

// Enumerate registered Diagnose suites and get their diagnoses
// for structural output
func getDiagnosesFromCurrentProcess(diagCfg diagnosis.Config, suites []diagnosis.Suite) (*diagnosis.DiagnoseResult, error) {
	suites, err := getSortedAndFilteredDiagnoseSuites(diagCfg, suites, func(suite diagnosis.Suite) string { return suite.SuitName })
	if err != nil {
		return nil, err
	}

	var suitesDiagnoses []diagnosis.Diagnoses
	var count diagnosis.Counters
	for _, ds := range suites {
		// Run particular diagnose
		diagnoses := getSuiteDiagnoses(ds)
		if len(diagnoses) > 0 {
			for _, d := range diagnoses {
				count.Increment(d.Result)
			}
			suitesDiagnoses = append(suitesDiagnoses, diagnosis.Diagnoses{
				SuiteName:      ds.SuitName,
				SuiteDiagnoses: diagnoses,
			})
		}
	}
	diagnoseResult := &diagnosis.DiagnoseResult{
		Diagnoses: suitesDiagnoses,
		Summary:   count,
	}

	return diagnoseResult, nil
}

func requestDiagnosesFromAgentProcess(diagCfg diagnosis.Config) (*diagnosis.DiagnoseResult, error) {
	// Get client to Agent's RPC call
	c := util.GetClient(false)
	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, fmt.Errorf("error getting IPC address for the agent: %w", err)
	}

	// Make sure we have a session token (for privileged information)
	if err = util.SetAuthToken(pkgconfigsetup.Datadog()); err != nil {
		return nil, fmt.Errorf("auth error: %w", err)
	}

	// Form call end-point
	//nolint:revive // TODO(CINT) Fix revive linter
	diagnoseURL := fmt.Sprintf("https://%v:%v/agent/diagnose", ipcAddress, pkgconfigsetup.Datadog().GetInt("cmd_port"))

	// Serialized diag config to pass it to Agent execution context
	var cfgSer []byte
	if cfgSer, err = json.Marshal(diagCfg); err != nil {
		return nil, fmt.Errorf("error while encoding diagnose configuration: %s", err)
	}

	// Run diagnose code inside Agent process
	var response []byte
	response, err = util.DoPost(c, diagnoseURL, "application/json", bytes.NewBuffer(cfgSer))
	if err != nil {
		if response != nil && string(response) != "" {
			return nil, fmt.Errorf("error getting diagnoses from running agent: %s", strings.TrimSpace(string(response)))
		}
		return nil, fmt.Errorf("the agent was unable to get diagnoses from running agent: %w", err)
	}

	// Deserialize results
	var diagnoses diagnosis.DiagnoseResult
	err = json.Unmarshal(response, &diagnoses)
	if err != nil {
		return nil, fmt.Errorf("error while decoding diagnose results returned from Agent: %w", err)
	}

	return &diagnoses, nil
}

// RunInCLIProcess run diagnoses in the CLI process.
func RunInCLIProcess(diagCfg diagnosis.Config, deps SuitesDepsInCLIProcess) (*diagnosis.DiagnoseResult, error) {
	return RunDiagnose(diagCfg, func() []diagnosis.Suite {
		return buildSuites(diagCfg, func() []diagnosis.Diagnosis {
			return diagnoseChecksInCLIProcess(diagCfg, deps.senderManager, deps.logReceiver, deps.secretResolver, deps.wmeta, deps.AC, deps.tagger)
		})
	})
}

// RunDiagnose runs diagnoses.
func RunDiagnose(diagCfg diagnosis.Config, getSuites func() []diagnosis.Suite) (*diagnosis.DiagnoseResult, error) {
	// Make remote call to get diagnoses
	if !diagCfg.RunLocal {
		return requestDiagnosesFromAgentProcess(diagCfg)
	}

	// Collect local diagnoses
	diagnoseResult, err := getDiagnosesFromCurrentProcess(diagCfg, getSuites())
	if err != nil {
		return nil, err
	}

	return diagnoseResult, nil
}

// RunInAgentProcess runs diagnoses in the Agent process.
func RunInAgentProcess(diagCfg diagnosis.Config, deps SuitesDepsInAgentProcess) (*diagnosis.DiagnoseResult, error) {
	// Collect local diagnoses
	suites := buildSuites(diagCfg, func() []diagnosis.Diagnosis {
		return diagnoseChecksInAgentProcess(deps.collector)
	})

	return getDiagnosesFromCurrentProcess(diagCfg, suites)
}

// RunLocalCheck runs locally the checks created by the registries.
func RunLocalCheck(diagCfg diagnosis.Config, registries ...func(*diagnosis.Catalog)) (*diagnosis.DiagnoseResult, error) {
	suites := buildCustomSuites(registries...)
	return getDiagnosesFromCurrentProcess(diagCfg, suites)
}

// RunDiagnoseStdOut runs the diagnose and outputs the results to the writer.
func RunDiagnoseStdOut(w io.Writer, diagCfg diagnosis.Config, diagnoses *diagnosis.DiagnoseResult) error {
	if diagCfg.JSONOutput {
		return runStdOutJSON(w, diagnoses)
	}
	return runStdOut(w, diagCfg, diagnoses)
}

func runStdOutJSON(w io.Writer, diagnoseResult *diagnosis.DiagnoseResult) error {
	diagJSON, err := json.MarshalIndent(diagnoseResult, "", "  ")
	if err != nil {
		body, _ := json.Marshal(map[string]string{"error": fmt.Sprintf("marshalling diagnose results to JSON: %s", err)})
		fmt.Fprintln(w, string(body))
		return err
	}
	fmt.Fprintln(w, string(diagJSON))

	return nil
}

func runStdOut(w io.Writer, diagCfg diagnosis.Config, diagnoseResult *diagnosis.DiagnoseResult) error {
	if w != color.Output {
		color.NoColor = true
	}

	fmt.Fprintf(w, "=== Starting diagnose ===\n")

	lastDot := false
	for _, ds := range diagnoseResult.Diagnoses {
		suiteAlreadyReported := false
		for _, d := range ds.SuiteDiagnoses {

			if d.Result == diagnosis.DiagnosisSuccess && !diagCfg.Verbose {
				outputDot(w, &lastDot)
				continue
			}

			outputSuiteIfNeeded(w, ds.SuiteName, &suiteAlreadyReported)

			outputNewLineIfNeeded(w, &lastDot)
			outputDiagnosis(w, diagCfg, d.Result.ToString(true), diagnoseResult.Summary.Total, d)
		}
	}

	outputNewLineIfNeeded(w, &lastDot)
	summary(w, diagnoseResult.Summary)

	return nil
}

// SuitesDeps stores the dependencies for the diagnose suites.
type SuitesDeps struct {
	SenderManager  sender.DiagnoseSenderManager
	Collector      optional.Option[collector.Component]
	SecretResolver secrets.Component
	WMeta          optional.Option[workloadmeta.Component]
	AC             autodiscovery.Component
	Tagger         tagger.Component
}

// SuitesDepsInCLIProcess stores the dependencies for the diagnose suites when running the CLI Process.
type SuitesDepsInCLIProcess struct {
	senderManager  sender.DiagnoseSenderManager
	secretResolver secrets.Component
	wmeta          optional.Option[workloadmeta.Component]
	AC             autodiscovery.Component
	logReceiver    integrations.Component
	tagger         tagger.Component
}

// NewSuitesDepsInCLIProcess returns a new instance of SuitesDepsInCLIProcess.
func NewSuitesDepsInCLIProcess(
	senderManager sender.DiagnoseSenderManager,
	secretResolver secrets.Component,
	wmeta optional.Option[workloadmeta.Component],
	ac autodiscovery.Component,
	tagger tagger.Component,
) SuitesDepsInCLIProcess {
	return SuitesDepsInCLIProcess{
		senderManager:  senderManager,
		secretResolver: secretResolver,
		wmeta:          wmeta,
		AC:             ac,
		tagger:         tagger,
	}
}

// SuitesDepsInAgentProcess stores the dependencies for the diagnose suites when running the Agent Process.
type SuitesDepsInAgentProcess struct {
	collector collector.Component
}

// NewSuitesDepsInAgentProcess returns a new instance of SuitesDepsInAgentProcess.
func NewSuitesDepsInAgentProcess(collector collector.Component) SuitesDepsInAgentProcess {
	return SuitesDepsInAgentProcess{
		collector: collector,
	}
}

// GetWMeta returns the workload metadata instance
func (s *SuitesDeps) GetWMeta() optional.Option[workloadmeta.Component] {
	return s.WMeta
}

// NewSuitesDeps returns a new SuitesDeps.
func NewSuitesDeps(
	senderManager sender.DiagnoseSenderManager,
	collector optional.Option[collector.Component],
	secretResolver secrets.Component,
	wmeta optional.Option[workloadmeta.Component], ac autodiscovery.Component,
	tagger tagger.Component,
) SuitesDeps {
	return SuitesDeps{
		SenderManager:  senderManager,
		Collector:      collector,
		SecretResolver: secretResolver,
		WMeta:          wmeta,
		AC:             ac,
		Tagger:         tagger,
	}
}

func getCheckNames(diagCfg diagnosis.Config) []string {
	suites := buildSuites(diagCfg, func() []diagnosis.Diagnosis { return nil })
	names := make([]string, len(suites))
	for i, s := range suites {
		names[i] = s.SuitName
	}
	return names
}

func buildSuites(diagCfg diagnosis.Config, checkDatadog func() []diagnosis.Diagnosis) []diagnosis.Suite {
	return buildCustomSuites(
		RegisterCheckDatadog(checkDatadog),
		RegisterConnectivityDatadogCoreEndpoints(diagCfg),
		RegisterConnectivityAutodiscovery,
		RegisterConnectivityDatadogEventPlatform,
		RegisterPortConflict,
	)
}

func buildCustomSuites(registries ...func(*diagnosis.Catalog)) []diagnosis.Suite {
	catalog := diagnosis.NewCatalog()
	for _, registry := range registries {
		registry(catalog)
	}
	return catalog.GetSuites()
}

// RegisterCheckDatadog registers the check-datadog diagnose suite.
func RegisterCheckDatadog(checkDatadog func() []diagnosis.Diagnosis) func(catalog *diagnosis.Catalog) {
	return func(catalog *diagnosis.Catalog) {
		catalog.Register("check-datadog", checkDatadog)
	}
}

// RegisterConnectivityDatadogCoreEndpoints registers the connectivity-datadog-core-endpoints diagnose suite.
func RegisterConnectivityDatadogCoreEndpoints(diagCfg diagnosis.Config) func(catalog *diagnosis.Catalog) {
	return func(catalog *diagnosis.Catalog) {
		catalog.Register("connectivity-datadog-core-endpoints", func() []diagnosis.Diagnosis { return connectivity.Diagnose(diagCfg) })
	}
}

// RegisterConnectivityAutodiscovery registers the connectivity-datadog-autodiscovery diagnose suite.
func RegisterConnectivityAutodiscovery(catalog *diagnosis.Catalog) {
	catalog.Register("connectivity-datadog-autodiscovery", connectivity.DiagnoseMetadataAutodiscoveryConnectivity)
}

// RegisterConnectivityDatadogEventPlatform registers the connectivity-datadog-event-platform diagnose suite.
func RegisterConnectivityDatadogEventPlatform(catalog *diagnosis.Catalog) {
	catalog.Register("connectivity-datadog-event-platform", eventplatformimpl.Diagnose)
}

// RegisterPortConflict registers the port-conflict diagnose suite.
func RegisterPortConflict(catalog *diagnosis.Catalog) {
	// port-conflict suite available in darwin and linux only for now
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		catalog.Register("port-conflict", func() []diagnosis.Diagnosis { return ports.DiagnosePortSuite() })
	}
}
