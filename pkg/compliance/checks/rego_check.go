// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/metrics"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/topdown/print"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const regoEvaluator = "rego"
const regoEvalTimeout = 20 * time.Second

type regoCheck struct {
	evalLock sync.Mutex

	metrics        metrics.Metrics
	ruleID         string
	ruleScope      compliance.RuleScope
	inputs         []compliance.RegoInput
	regoModuleArgs []func(*rego.Rego)
}

func importModule(importPath, parentDir string, required bool) (string, error) {
	// look for relative file if we have a source
	if parentDir != "" {
		importPath = filepath.Join(parentDir, importPath)
	}

	mod, err := os.ReadFile(importPath)
	if err != nil {
		if required {
			return "", err
		}
		return "", nil
	}
	return string(mod), nil
}

func computeRuleModulesAndQuery(rule *compliance.RegoRule, meta *compliance.SuiteMeta) ([]func(*rego.Rego), string, error) {
	options := make([]func(*rego.Rego), 0)

	options = append(options, rego.Module("datadog_helpers.rego", helpers))

	query := rule.Findings

	if rule.Module != "" {
		mod, err := ast.ParseModule(fmt.Sprintf("__gen__rule_%s.rego", rule.ID), rule.Module)
		if err != nil {
			return nil, "", err
		}

		options = append(options, rego.ParsedModule(mod))

		if query == "" {
			query = fmt.Sprintf("%v.findings", mod.Package.Path)
		}
	}

	if query == "" {
		query = "data.datadog.findings"
		log.Infof("defaulting rego query to `%s`", query)
	}

	var parentDir string
	if meta.Source != "" {
		parentDir = filepath.Dir(meta.Source)
	}

	alreadyImported := make(map[string]bool)

	// import rego file with the same name as the rule id
	imp := fmt.Sprintf("%s.rego", rule.ID)
	mod, err := importModule(imp, parentDir, false)
	if err != nil {
		return nil, "", err
	}
	if mod != "" {
		options = append(options, rego.Module(imp, mod))
	}
	alreadyImported[imp] = true

	// import explicitly required imports
	for _, imp := range rule.Imports {
		if imp == "" || alreadyImported[imp] {
			continue
		}

		mod, err := importModule(imp, parentDir, true)
		if err != nil {
			return nil, "", err
		}

		if mod != "" {
			options = append(options, rego.Module(imp, mod))
		}
		alreadyImported[imp] = true
	}

	return options, query, nil
}

func (r *regoCheck) compileRule(rule *compliance.RegoRule, regoOptions []func(r *rego.Rego), meta *compliance.SuiteMeta) error {
	moduleArgs := make([]func(*rego.Rego), 0, 2+len(regoBuiltins))

	// rego modules and query
	ruleModules, query, err := computeRuleModulesAndQuery(rule, meta)
	if err != nil {
		return err
	}
	moduleArgs = append(moduleArgs, regoOptions...)
	moduleArgs = append(moduleArgs, ruleModules...)
	moduleArgs = append(moduleArgs, rego.Query(query))

	log.Debugf("rego query: %v", query)

	r.regoModuleArgs = moduleArgs

	return nil
}

func (r *regoCheck) buildNormalInput(env env.Env) (eval.RegoInputMap, error) {
	objectsPerTags := make(map[string]interface{})
	arraysPerTags := make(map[string][]interface{})

	contextInput := r.buildContextInput(env)
	objectsPerTags["context"] = contextInput

	for _, input := range r.inputs {
		resolve, _, err := resourceKindToResolverAndFields(env, r.ruleID, input.Kind())
		if err != nil {
			return nil, err
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		resolved, err := resolve(ctx, env, r.ruleID, input.ResourceCommon, true)
		if err != nil {
			log.Warnf("failed to resolve input: %v", err)
			continue
		}

		tagName := extractTagName(&input)

		inputType, err := input.ValidateInputType()
		if err != nil {
			return nil, err
		}

		if _, present := objectsPerTags[tagName]; present {
			return nil, fmt.Errorf("already defined tag: `%s`", tagName)
		}

		switch res := resolved.(type) {
		case nil:
			switch inputType {
			case "array":
				r.appendInstance(arraysPerTags, tagName, nil)
			case "object":
				objectsPerTags[tagName] = &struct{}{}
			default:
				return nil, fmt.Errorf("internal error, wrong input type `%s`", inputType)
			}
		case resolvedInstance:
			switch inputType {
			case "array":
				r.appendInstance(arraysPerTags, tagName, res)
			case "object":
				objectsPerTags[tagName] = res.RegoInput()
			default:
				return nil, fmt.Errorf("internal error, wrong input type `%s`", inputType)
			}
		case eval.Iterator:
			if inputType != "array" {
				return nil, fmt.Errorf("the input kind `%s` does not support the `%s` type", string(input.Kind()), inputType)
			}

			// create an empty array as a base
			// this is useful if the iterator is empty for example, as it will ensure we at least
			// export an empty array to the rego input
			if _, present := arraysPerTags[tagName]; !present {
				arraysPerTags[tagName] = []interface{}{}
			}

			it := res
			for !it.Done() {
				instance, err := it.Next()
				if err != nil {
					return nil, err
				}

				r.appendInstance(arraysPerTags, tagName, instance)
			}
		}
	}

	input := make(map[string]interface{})
	for k, v := range objectsPerTags {
		input[k] = v
	}
	for k, v := range arraysPerTags {
		if _, present := input[k]; present {
			return nil, fmt.Errorf("multiple definitions of tag: `%s`", k)
		}
		input[k] = v
	}

	return input, nil
}

func extractTagName(input *compliance.RegoInput) string {
	tagName := input.TagName
	if tagName == "" {
		return string(input.Kind())
	}
	return tagName
}

func (r *regoCheck) appendInstance(input map[string][]interface{}, key string, instance eval.Instance) {
	vars, exists := input[key]
	if !exists {
		vars = []interface{}{}
	}

	if instance != nil {
		input[key] = append(vars, instance.RegoInput())
	}
}

func buildMappedInputs(inputs []compliance.RegoInput) map[string]compliance.RegoInput {
	res := make(map[string]compliance.RegoInput)
	for _, input := range inputs {
		tagName := extractTagName(&input)

		if _, present := res[tagName]; present {
			log.Warnf("error building mapped input context: duplicated tag")
			return nil
		}

		res[tagName] = input
	}
	return res
}

func roundTrip(inputs interface{}) (interface{}, error) {
	output, err := yaml.Marshal(inputs)
	if err != nil {
		return nil, err
	}
	var res interface{}
	if err := yaml.Unmarshal(output, &res); err != nil {
		return nil, err
	}
	return res, nil
}

func (r *regoCheck) buildContextInput(env env.Env) eval.RegoInputMap {
	context := make(map[string]interface{})
	context["ruleID"] = r.ruleID
	context["hostname"] = env.Hostname()

	if r.ruleScope == compliance.KubernetesClusterScope {
		context["kubernetes_cluster"], _ = env.KubeClient().ClusterID()
	}
	if r.ruleScope == compliance.KubernetesNodeScope {
		context["kubernetes_node_labels"] = env.NodeLabels()
	}

	mappedInputs := buildMappedInputs(r.inputs)
	if mappedInputs != nil {
		preparedInputs, err := roundTrip(mappedInputs)
		if err != nil {
			log.Warnf("failed to build mapped inputs in context")
		} else {
			context["input"] = preparedInputs
		}
	}

	return context
}

func findingsToReports(findings []regoFinding) []*compliance.Report {
	var reports []*compliance.Report
	for _, finding := range findings {
		reportResource := compliance.ReportResource{
			ID:   finding.ResourceID,
			Type: finding.ResourceType,
		}

		var report *compliance.Report
		switch finding.Status {
		case "error":
			errMsg := finding.Data["error"]
			if errMsg == nil {
				errMsg = ""
			}
			err := fmt.Errorf("%v", errMsg)
			report = &compliance.Report{
				Resource:          reportResource,
				Passed:            false,
				Error:             err,
				UserProvidedError: true,
				Evaluator:         regoEvaluator,
			}
		case "passed":
			report = &compliance.Report{
				Resource:  reportResource,
				Passed:    true,
				Data:      finding.Data,
				Evaluator: regoEvaluator,
			}
		case "failing":
			report = &compliance.Report{
				Resource:  reportResource,
				Passed:    false,
				Data:      finding.Data,
				Evaluator: regoEvaluator,
			}
		default:
			return buildRegoErrorReports(fmt.Errorf("unknown finding status: %s", finding.Status))
		}

		reports = append(reports, report)
	}
	return reports
}

func (r *regoCheck) check(env env.Env) []*compliance.Report {
	r.evalLock.Lock()
	defer r.evalLock.Unlock()

	log.Debugf("%s: rego check starting", r.ruleID)

	var input eval.RegoInputMap
	providedInput := env.ProvidedInput(r.ruleID)

	if providedInput != nil {
		input = providedInput
	} else {
		normalInput, err := r.buildNormalInput(env)
		if err != nil {
			return buildRegoErrorReports(err)
		}

		input = normalInput
	}

	log.Debugf("rego eval input: %+v", input)

	if path := env.DumpInputPath(); path != "" {
		// if the dump failed we pass
		_ = dumpInputToFile(r.ruleID, path, input)
	}

	if env.ShouldSkipRegoEval() {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), regoEvalTimeout)
	defer cancel()

	args := make([]func(*rego.Rego), len(r.regoModuleArgs), len(r.regoModuleArgs)+2)
	copy(args, r.regoModuleArgs)
	args = append(args, rego.Input(input), rego.Metrics(r.metrics))

	regoMod := rego.New(args...)
	results, err := regoMod.Eval(ctx)
	if err != nil {
		return buildRegoErrorReports(err)
	}

	log.Debugf("%s: rego evaluation done => %+v", r.ruleID, results)

	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return buildRegoErrorReports(errors.New("failed to collect result expression"))
	}

	findings, err := parseFindings(results[0].Expressions[0].Value)
	if err != nil {
		return buildRegoErrorReports(err)
	}

	reports := findingsToReports(findings)

	log.Debugf("reports: %v", reports)
	return reports
}

func dumpInputToFile(ruleID, path string, input interface{}) error {
	currentData := make(map[string]interface{})
	currentContent, err := os.ReadFile(path)
	if err == nil {
		if len(currentContent) != 0 {
			if err := json.Unmarshal(currentContent, &currentData); err != nil {
				return err
			}
		}
	}

	currentData[ruleID] = input

	jsonData, err := PrettyPrintJSON(currentData, "\t")
	if err != nil {
		return err
	}

	return os.WriteFile(path, jsonData, 0644)
}

type regoFinding struct {
	Status       string     `mapstructure:"status"`
	ResourceType string     `mapstructure:"resource_type"`
	ResourceID   string     `mapstructure:"resource_id"`
	Data         event.Data `mapstructure:"data"`
}

func parseFindings(regoData interface{}) ([]regoFinding, error) {
	arrayData, ok := regoData.([]interface{})
	if !ok {
		return nil, errors.New("failed to parse array of findings")
	}

	res := make([]regoFinding, 0)

	for _, data := range arrayData {
		m, ok := data.(map[string]interface{})
		if !ok {
			return nil, errors.New("failed to parse finding")
		}

		var finding regoFinding
		var decodeMetadata mapstructure.Metadata
		if err := mapstructure.DecodeMetadata(m, &finding, &decodeMetadata); err != nil {
			return nil, err
		}

		if err := checkFindingRequiredFields(&decodeMetadata); err != nil {
			return nil, err
		}

		if err := finding.normalizeStatus(); err != nil {
			return nil, err
		}

		res = append(res, finding)
	}

	return res, nil
}

var statusMapping = map[string]string{
	"passed":  "passed",
	"pass":    "passed",
	"failing": "failing",
	"fail":    "failing",
	"error":   "error",
	"err":     "error",
}

func (finding *regoFinding) normalizeStatus() error {
	mapped, ok := statusMapping[finding.Status]
	if !ok {
		return fmt.Errorf("unknown finding status: %s", finding.Status)
	}

	finding.Status = mapped
	return nil
}

func checkFindingRequiredFields(metadata *mapstructure.Metadata) error {
	requiredFields := make(map[string]bool)
	requiredFields["status"] = false

	for _, decodedField := range metadata.Keys {
		if _, present := requiredFields[decodedField]; present {
			requiredFields[decodedField] = true
		}
	}

	for field, present := range requiredFields {
		if !present {
			return fmt.Errorf("missing field `%s` when decoding rego finding", field)
		}
	}

	return nil
}

func buildRegoErrorReports(err error) []*compliance.Report {
	log.Debugf("building rego error report: %v", err)
	report := compliance.BuildReportForError(err)
	report.Evaluator = regoEvaluator
	return []*compliance.Report{report}
}

type regoPrintHook struct{}

func (h *regoPrintHook) Print(_ print.Context, value string) error {
	log.Infof("Rego print output: %s", value)
	return nil
}
