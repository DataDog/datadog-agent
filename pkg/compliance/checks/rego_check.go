// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

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

	return nil
}

func (r *regoCheck) normalizeInputMap(vars map[string]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{})
	for k, v := range vars {
		ps := strings.Split(k, ".")
		name := ps[len(ps)-1]
		normalized[name] = v
	}

	return normalized
}

func (r *regoCheck) buildNormalInput(env env.Env) (map[string]interface{}, error) {
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

func (r *regoCheck) buildContextInput(env env.Env) map[string]interface{} {
	context := make(map[string]interface{})
	context["hostname"] = env.Hostname()

	if r.ruleScope == compliance.KubernetesNodeScope {
		context["kubernetes_node_labels"] = env.NodeLabels()
	}

	return context
}

func (r *regoCheck) check(env env.Env) []*compliance.Report {
	log.Debugf("%s: rego check starting", r.ruleID)

	var resultFinalizer func([]regoFinding) []*compliance.Report

	var input map[string]interface{}
	providedInput := env.ProvidedInput(r.ruleID)

	if providedInput != nil {
		input = providedInput

		resultFinalizer = func(findings []regoFinding) []*compliance.Report {
			for _, finding := range findings {
				jsonData, err := prettyPrintJSON(finding.asMap())
				if err != nil {
					log.Warnf("failed to pretty-print finding %v", finding)
					continue
				}
				log.Infof("finding: %v", string(jsonData))
			}
			return nil
		}
	} else {
		normalInput, err := r.buildNormalInput(env)
		if err != nil {
			return []*compliance.Report{compliance.BuildReportForError(err)}
		}

		input = normalInput

		resultFinalizer = func(findings []regoFinding) []*compliance.Report {
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

	reports := resultFinalizer(findings)

	log.Debugf("reports: %v", reports)
	return reports
}

func prettyPrintJSON(data interface{}) ([]byte, error) {
	unformatted, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	if err := json.Indent(&buffer, unformatted, "", "\t"); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
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

	jsonData, err := prettyPrintJSON(currentData)
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
	normalized := r.normalizeInputMap(instance.Vars().GoMap())
	input[key] = append(vars, normalized)
}

type regoFinding struct {
	Status       bool       `mapstructure:"status"`
	ResourceType string     `mapstructure:"resource_type"`
	ResourceID   string     `mapstructure:"resource_id"`
	Data         event.Data `mapstructure:"data"`
}

func (f *regoFinding) asMap() map[string]interface{} {
	res := map[string]interface{}{
		"resource_type": f.ResourceType,
		"resource_id":   f.ResourceID,
		"data":          f.Data,
	}
	if f.Status {
		res["status"] = "passed"
	} else {
		res["status"] = "failing"
	}

	return res
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
		if err := mapstructure.Decode(m, &finding); err != nil {
			return nil, err
		}
		res = append(res, finding)
	}

	return res, nil
}
