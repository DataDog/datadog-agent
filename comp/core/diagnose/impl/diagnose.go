// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package diagnoseimpl implements the diagnose component interface
package diagnoseimpl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/api/api/utils"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/comp/core/diagnose/format"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type suite struct {
	name     string
	diagnose func(diagnose.Config) []diagnose.Diagnosis
}

// Requires defines the dependencies for the diagnose component
type Requires struct {
}

// Provides defines the output of the diagnose component
type Provides struct {
	Comp          diagnose.Component
	APIDiagnose   api.AgentEndpointProvider
	FlareProvider flaretypes.Provider
}

type diagnoseRegistry struct {
}

// NewComponent creates a new diagnose component
func NewComponent(_ Requires) (Provides, error) {
	comp := &diagnoseRegistry{}
	provides := Provides{
		Comp: comp,
		APIDiagnose: api.NewAgentEndpointProvider(func(w http.ResponseWriter, r *http.Request) { comp.getDiagnose(w, r) },
			"/diagnose",
			"POST",
		),
		FlareProvider: flaretypes.NewProvider(comp.fillFlare),
	}
	return provides, nil
}

func (d *diagnoseRegistry) RunSuite(suiteName string, formatOutput string, verbose bool) ([]byte, error) {
	catalog := diagnose.GetCatalog()
	diag, ok := catalog.Suites[suiteName]
	if !ok {
		return []byte{}, fmt.Errorf("diagnose suite %s not found", suiteName)
	}

	diagnoseConfig := diagnose.Config{
		Verbose: verbose,
	}

	diagnoseResult, err := getDiagnoses(diagnoseConfig, []suite{{name: suiteName, diagnose: diag}})

	if err != nil {
		return nil, err
	}
	return formatResult(diagnoseResult, diagnoseConfig, formatOutput)
}

func (d *diagnoseRegistry) RunSuites(formatOutput string, verbose bool) ([]byte, error) {
	diagnoseConfig := diagnose.Config{
		Verbose: verbose,
	}
	diagnoseResult, err := d.run(diagnoseConfig)
	if err != nil {
		return nil, err
	}

	return formatResult(diagnoseResult, diagnoseConfig, formatOutput)
}

func (d *diagnoseRegistry) RunLocalSuite(suites diagnose.Suites, config diagnose.Config) (*diagnose.Result, error) {
	internalSuites := []suite{}
	for name, diag := range suites {
		internalSuites = append(internalSuites, suite{
			name:     name,
			diagnose: diag,
		})
	}
	return getDiagnoses(config, internalSuites)
}

func (d *diagnoseRegistry) getDiagnose(w http.ResponseWriter, r *http.Request) {
	diagCfg := diagnose.Config{
		Verbose: true,
	}

	// Read parameters
	if r.Body != http.NoBody {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, log.Errorf("Error while reading HTTP request body: %s", err).Error(), 500)
			return
		}

		if err := json.Unmarshal(body, &diagCfg); err != nil {
			http.Error(w, log.Errorf("Error while unmarshaling JSON from request body: %s", err).Error(), 500)
			return
		}
	}

	// Reset the `server_timeout` deadline for this connection as running diagnose code in Agent process can take some time
	conn, ok := utils.GetConnection(r)
	if ok {
		_ = conn.SetDeadline(time.Time{})
	}

	diagnoseResult, err := d.run(diagCfg)
	if err != nil {
		httputils.SetJSONError(w, log.Errorf("Running diagnose in Agent process failed: %s", err), 500)
		return
	}

	// Serizalize diagnoses (and implicitly write result to the response)
	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(diagnoseResult)
	if err != nil {
		httputils.SetJSONError(w, log.Errorf("Unable to marshal response: %s", err), 500)
	}
}

func (d *diagnoseRegistry) run(diagCfg diagnose.Config) (*diagnose.Result, error) {
	catalog := diagnose.GetCatalog()
	suites := []suite{}
	for name, diag := range catalog.Suites {
		suites = append(suites, suite{
			name:     name,
			diagnose: diag,
		})
	}
	return getDiagnoses(diagCfg, suites)
}

func (d *diagnoseRegistry) fillFlare(fb flaretypes.FlareBuilder) error {
	fb.AddFileFromFunc("diagnose.log", func() ([]byte, error) {
		diagnoseConfig := diagnose.Config{Verbose: true}
		result, err := d.run(diagnoseConfig)
		if err != nil {
			return nil, err
		}

		return formatResult(result, diagnoseConfig, "text")
	})
	return nil
}

func formatResult(diagnoseResult *diagnose.Result, diagCfg diagnose.Config, formatOutput string) ([]byte, error) {
	var buffer bytes.Buffer
	var err error
	writer := bufio.NewWriter(&buffer)

	switch formatOutput {
	case "json":
		err = format.JSON(writer, diagnoseResult)
	case "text":
		err = format.Text(writer, diagCfg, diagnoseResult)
	}

	if err != nil {
		return nil, err
	}
	writer.Flush()

	return buffer.Bytes(), nil
}

// Enumerate registered Diagnose suites and get their diagnoses
// for structural output
func getDiagnoses(diagCfg diagnose.Config, suites []suite) (*diagnose.Result, error) {
	suites, err := getSortedAndFilteredDiagnoseSuites(diagCfg, suites)
	if err != nil {
		return nil, err
	}

	var suitesDiagnoses []diagnose.Diagnoses
	var count diagnose.Counters
	for _, ds := range suites {
		// Run particular diagnose
		diagnoses := getSuiteDiagnoses(ds, diagCfg)
		if len(diagnoses) > 0 {
			for _, d := range diagnoses {
				count.Increment(d.Status)
			}
			suitesDiagnoses = append(suitesDiagnoses, diagnose.Diagnoses{
				Name:      ds.name,
				Diagnoses: diagnoses,
			})
		}
	}
	diagnoseResult := &diagnose.Result{
		Runs:    suitesDiagnoses,
		Summary: count,
	}

	return diagnoseResult, nil
}

// diagnose suite filter
type diagSuiteFilter struct {
	include []*regexp.Regexp
	exclude []*regexp.Regexp
}

func getSortedAndFilteredDiagnoseSuites(diagCfg diagnose.Config, suites []suite) ([]suite, error) {
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

	sortedValues := make([]suite, len(suites))
	copy(sortedValues, suites)
	sort.Slice(sortedValues, func(i, j int) bool {
		return sortedValues[i].name < sortedValues[j].name
	})

	var sortedFilteredValues []suite
	for _, ds := range sortedValues {
		if matchConfigFilters(filter, ds.name) {
			sortedFilteredValues = append(sortedFilteredValues, ds)
		}
	}

	return sortedFilteredValues, nil
}

func getSuiteDiagnoses(ds suite, diagConfig diagnose.Config) []diagnose.Diagnosis {
	diagnoses := ds.diagnose(diagConfig)

	// validate each diagnoses
	for i, d := range diagnoses {
		if d.Status < diagnose.DiagnosisResultMIN ||
			d.Status > diagnose.DiagnosisResultMAX ||
			len(d.Name) == 0 ||
			len(d.Diagnosis) == 0 {

			if len(d.RawError) > 0 {
				// If error already reported, append to it
				diagnoses[i].RawError = fmt.Sprintf("required diagnosis fields are invalid. Result:%d, Name:%s, Diagnosis:%s. Reported Error: %s",
					d.Status, d.Name, d.Diagnosis, d.RawError)
			} else {
				diagnoses[i].RawError = fmt.Sprintf("required diagnosis fields are invalid. Result:%d, Name:%s, Diagnosis:%s", d.Status, d.Name, d.Diagnosis)
			}

			diagnoses[i].Status = diagnose.DiagnosisUnexpectedError
			if len(d.Name) == 0 {
				diagnoses[i].Name = ds.name
			}
		}
	}

	return diagnoses
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
