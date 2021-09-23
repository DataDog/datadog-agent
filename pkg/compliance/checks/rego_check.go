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
	"strings"

	"github.com/open-policy-agent/opa/rego"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type regoCheck struct {
	ruleID            string
	resources         []compliance.RegoResource
	preparedEvalQuery rego.PreparedEvalQuery
}

func (r *regoCheck) compileRule(rule *compliance.RegoRule) error {
	ctx := context.TODO()

	log.Debugf("rego query: %v", rule.Findings)

	moduleArgs := make([]func(*rego.Rego), 0, 2+len(regoBuiltins))
	moduleArgs = append(moduleArgs, rego.Query(rule.Findings),
		rego.Module(fmt.Sprintf("rule_%s.rego", rule.ID), rule.Module),
		rego.Module("helpers.rego", helpers))
	moduleArgs = append(moduleArgs, regoBuiltins...)

	preparedEvalQuery, err := rego.New(
		moduleArgs...,
	).PrepareForEval(ctx)

	if err != nil {
		return err
	}

	r.preparedEvalQuery = preparedEvalQuery

	return nil
}

func (r *regoCheck) normalizeInputMap(vars map[string]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{})
	for k, v := range vars {
		ps := strings.SplitN(k, ".", 2)
		normalized[ps[1]] = v
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

	context := make(map[string]interface{})
	context["hostname"] = env.Hostname()

	input := make(map[string]interface{})
	for k, v := range inputPerTags {
		input[k] = v
	}
	input["context"] = context

	return input, nil
}

func (r *regoCheck) check(env env.Env) []*compliance.Report {
	log.Debugf("%s: rego check starting", r.ruleID)

	var resultFinalizer func([]regoFinding) []*compliance.Report

	var input map[string]interface{}
	providedInput := env.ProvidedInput(r.ruleID)

	if providedInput != nil {
		input = providedInput

		resultFinalizer = func(findings []regoFinding) []*compliance.Report {
			log.Infof("findings: %v", findings)
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
					ID:   finding.resourceID,
					Type: finding.resourceType,
				}

				report := &compliance.Report{
					Resource: reportResource,
					Passed:   finding.status,
					Data:     finding.data,
				}

				reports = append(reports, report)
			}
			return reports
		}
	}

	log.Debugf("rego eval input: %+v", input)

	if path := env.DumpInputPath(); path != "" {
		dumpInputToFile(r.ruleID, path, input)
	}

	ctx := context.TODO()
	results, err := r.preparedEvalQuery.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return []*compliance.Report{compliance.BuildReportForError(err)}
	} else if len(results) == 0 {
		return nil
	}

	log.Debugf("%s: rego evaluation done => %+v\n", r.ruleID, results)

	findings, err := parseFindings(results[0].Expressions[0].Value)
	if err != nil {
		return []*compliance.Report{compliance.BuildReportForError(err)}
	}

	reports := resultFinalizer(findings)

	log.Debugf("reports: %v", reports)
	return reports
}

func dumpInputToFile(ruleID, path string, input interface{}) error {
	currentData := make(map[string]interface{})
	currentContent, err := ioutil.ReadFile(path)
	if err == nil {
		json.Unmarshal(currentContent, &currentData)
	}

	currentData[ruleID] = input

	jsonInputDump, err := json.Marshal(currentData)
	if err != nil {
		return err
	}

	var buffer bytes.Buffer
	json.Indent(&buffer, jsonInputDump, "", "\t")

	return ioutil.WriteFile(path, buffer.Bytes(), 0644)
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
	status       bool
	resourceType string
	resourceID   string
	data         event.Data
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

		status, ok := m[ResourceStatusFindingField].(bool)
		if !ok {
			return nil, errors.New("failed to parse resource status")
		}

		id, ok := m[ResourceIDFindingField].(string)
		if !ok {
			return nil, errors.New("failed to parse resource_id")
		}

		rty, ok := m[ResourceTypeFindingField].(string)
		if !ok {
			return nil, errors.New("failed to parse resource_type")
		}

		data, ok := m[ResourceDataFindingField].(map[string]interface{})
		if !ok {
			return nil, errors.New("failed to parse resource data")
		}

		finding := regoFinding{
			status:       status,
			resourceID:   id,
			resourceType: rty,
			data:         data,
		}

		res = append(res, finding)
	}

	return res, nil
}
