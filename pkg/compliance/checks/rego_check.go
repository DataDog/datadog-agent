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
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mitchellh/mapstructure"
	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type regoCheck struct {
	ruleID            string
	constants         map[string]interface{}
	ruleScope         compliance.RuleScope
	resources         []compliance.RegoResource
	preparedEvalQuery rego.PreparedEvalQuery
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

	for _, imp := range rule.Imports {
		if imp == "" {
			continue
		}

		// look for relative file if we have a source
		if parentDir != "" {
			imp = filepath.Join(parentDir, imp)
		}

		mod, err := os.ReadFile(imp)
		if err != nil {
			return nil, "", err
		}
		options = append(options, rego.Module(imp, string(mod)))
	}

	return options, query, nil
}

func (r *regoCheck) compileRule(rule *compliance.RegoRule, ruleScope compliance.RuleScope, meta *compliance.SuiteMeta) error {
	ctx := context.TODO()

	moduleArgs := make([]func(*rego.Rego), 0, 2+len(regoBuiltins))

	// rego modules and query
	ruleModules, query, err := computeRuleModulesAndQuery(rule, meta)
	if err != nil {
		return err
	}
	moduleArgs = append(moduleArgs, ruleModules...)
	moduleArgs = append(moduleArgs, rego.Query(query))

	log.Debugf("rego query: %v", query)

	// rego builtins
	moduleArgs = append(moduleArgs, regoBuiltins...)

	preparedEvalQuery, err := rego.New(
		moduleArgs...,
	).PrepareForEval(ctx)

	if err != nil {
		return err
	}

	r.preparedEvalQuery = preparedEvalQuery
	r.ruleScope = ruleScope
	r.constants = rule.Constants

	return nil
}

func (r *regoCheck) buildNormalInput(env env.Env) (eval.RegoInputMap, error) {
	inputPerTags := make(map[string][]interface{})

	for _, resource := range r.resources {
		resolve, _, err := resourceKindToResolverAndFields(env, r.ruleID, resource.Kind())
		if err != nil {
			return nil, err
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()

		resolved, err := resolve(ctx, env, r.ruleID, resource.ResourceCommon)
		if err != nil {
			continue
		}

		if resource.TagName == "" {
			return nil, errors.New("no tag name found for resource")
		}

		switch res := resolved.(type) {
		case resolvedInstance:
			r.appendInstance(inputPerTags, resource.TagName, res)
		case eval.Iterator:
			it := res
			for !it.Done() {
				instance, err := it.Next()
				if err != nil {
					return nil, err
				}

				r.appendInstance(inputPerTags, resource.TagName, instance)
			}
		}
	}

	context := r.buildContextInput(env)

	input := make(map[string]interface{})
	for k, v := range inputPerTags {
		input[k] = v
	}
	input["context"] = context

	return input, nil
}

func (r *regoCheck) buildContextInput(env env.Env) eval.RegoInputMap {
	context := make(map[string]interface{})
	context["hostname"] = env.Hostname()
	context["constants"] = r.constants

	if r.ruleScope == compliance.KubernetesNodeScope {
		context["kubernetes_node_labels"] = env.NodeLabels()
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

		report := &compliance.Report{
			Resource: reportResource,
			Passed:   finding.Status,
			Data:     finding.Data,
		}

		reports = append(reports, report)
	}
	return reports
}

func (r *regoCheck) check(env env.Env) []*compliance.Report {
	log.Debugf("%s: rego check starting", r.ruleID)

	var input eval.RegoInputMap
	providedInput := env.ProvidedInput(r.ruleID)

	if providedInput != nil {
		input = providedInput
	} else {
		normalInput, err := r.buildNormalInput(env)
		if err != nil {
			return []*compliance.Report{compliance.BuildReportForError(err)}
		}

		input = normalInput
	}

	log.Debugf("rego eval input: %+v", input)

	if path := env.DumpInputPath(); path != "" {
		// if the dump failed we pass
		_ = dumpInputToFile(r.ruleID, path, input)
	}

	ctx := context.TODO()
	results, err := r.preparedEvalQuery.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return []*compliance.Report{compliance.BuildReportForError(err)}
	} else if len(results) == 0 {
		return nil
	}

	log.Debugf("%s: rego evaluation done => %+v\n", r.ruleID, results)

	if len(results) == 0 || len(results[0].Expressions) == 0 {
		return []*compliance.Report{
			compliance.BuildReportForError(
				errors.New("failed to collect result expression"),
			),
		}
	}

	findings, err := parseFindings(results[0].Expressions[0].Value)
	if err != nil {
		return []*compliance.Report{compliance.BuildReportForError(err)}
	}

	reports := findingsToReports(findings)

	log.Debugf("reports: %v", reports)
	return reports
}

func dumpInputToFile(ruleID, path string, input interface{}) error {
	currentData := make(map[string]interface{})
	currentContent, err := ioutil.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(currentContent, &currentData); err != nil {
			return err
		}
	}

	currentData[ruleID] = input

	jsonData, err := PrettyPrintJSON(currentData, "\t")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(path, jsonData, 0644)
}

func (r *regoCheck) appendInstance(input map[string][]interface{}, key string, instance eval.Instance) {
	vars, exists := input[key]
	if !exists {
		vars = []interface{}{}
	}
	input[key] = append(vars, instance.RegoInput())
}

type regoFinding struct {
	Status       bool       `mapstructure:"status"`
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

		if err := checkFindingsRequiredFields(&decodeMetadata); err != nil {
			return nil, err
		}

		res = append(res, finding)
	}

	return res, nil
}

func checkFindingsRequiredFields(metadata *mapstructure.Metadata) error {
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
