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
	"strconv"
	"strings"

	"github.com/open-policy-agent/opa/ast"
	"github.com/open-policy-agent/opa/rego"
	"github.com/open-policy-agent/opa/types"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type regoCheck struct {
	ruleID            string
	resources         []compliance.RegoResource
	preparedEvalQuery rego.PreparedEvalQuery
}

func (r *regoCheck) compileRule(rule *compliance.RegoRule) error {
	ctx := context.TODO()

	var query string
	if rule.Denies != "" {
		query = fmt.Sprintf(`
			{
				"result": %s,
				"denies": %s
			}
		`, rule.Query, rule.Denies)
	} else {
		query = fmt.Sprintf(`
			{
				"result": %s,
				"denies": []
			}
		`, rule.Query)
	}
	log.Debugf("rego query: %v", query)

	moduleArgs := make([]func(*rego.Rego), 0, 2+len(regoBuiltins))
	moduleArgs = append(moduleArgs, rego.Query(query),
		rego.Module(fmt.Sprintf("rule_%s.rego", rule.ID), rule.Module))
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

type instanceFields struct {
	instance eval.Instance
	fields   []string
}

func resourcePluralizer(resource compliance.ResourceCommon) string {
	str := string(resource.Kind())

	if strings.HasSuffix(str, "s") {
		return str + "es"
	}
	return str + "s"
}

type envInput struct {
	input             map[string][]interface{}
	resourceInstances map[int]instanceFields
}

func (r *regoCheck) buildNormalInput(env env.Env) (envInput, error) {
	input := make(map[string][]interface{})

	currentIDCounter := 0
	resourceInstances := make(map[int]instanceFields)
	for _, resource := range r.resources {
		resolve, reportedFields, err := resourceKindToResolverAndFields(env, r.ruleID, resource.Kind())
		if err != nil {
			return envInput{}, err
			// return []*compliance.Report{compliance.BuildReportForError(err)}
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)

		resolved, err := resolve(ctx, env, r.ruleID, resource.ResourceCommon)
		if err != nil {
			cancel()
			continue
		}
		cancel()

		key := resourcePluralizer(resource.ResourceCommon)

		switch res := resolved.(type) {
		case resolvedInstance:
			r.appendInstance(resourceInstances, input, key, res, &currentIDCounter, reportedFields)
		case eval.Iterator:
			it := res
			for !it.Done() {
				instance, err := it.Next()
				if err != nil {
					return envInput{}, err
				}

				r.appendInstance(resourceInstances, input, key, instance, &currentIDCounter, reportedFields)
			}
		}
	}

	return envInput{
		input:             input,
		resourceInstances: resourceInstances,
	}, nil
}

func (r *regoCheck) check(env env.Env) []*compliance.Report {
	log.Debugf("%s: rego check starting", r.ruleID)

	var resultFinalizer func(bool, []int) []*compliance.Report

	var input map[string][]interface{}
	providedInput := env.ProvidedInput()

	if providedInput != nil {
		input = *providedInput

		resultFinalizer = func(passed bool, denies []int) []*compliance.Report {
			log.Infof("denies: %v", denies)
			return nil
		}
	} else {
		envInput, err := r.buildNormalInput(env)
		if err != nil {
			return []*compliance.Report{compliance.BuildReportForError(err)}
		}

		input = envInput.input

		resultFinalizer = func(passed bool, denies []int) []*compliance.Report {
			var reports []*compliance.Report
			for resID, instanceFields := range envInput.resourceInstances {
				specificPassed := !containsInt(denies, resID)
				// if the global checked passed, the specific doesn't matter
				resultPassed := passed || specificPassed

				ri, ok := instanceFields.instance.(resolvedInstance)
				if ok {
					report := instanceToReport(ri, resultPassed, instanceFields.fields)
					reports = append(reports, report)
				} else {
					report := evalInstanceToReport(instanceFields.instance, resultPassed, instanceFields.fields)
					reports = append(reports, report)
				}
			}
			return reports
		}
	}

	log.Debugf("rego eval input: %+v", input)

	if path := env.DumpInputPath(); path != "" {
		dumpInputToFile(path, input)
	}

	ctx := context.TODO()
	results, err := r.preparedEvalQuery.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return []*compliance.Report{compliance.BuildReportForError(err)}
	} else if len(results) == 0 {
		return nil
	}

	log.Debugf("%s: rego evaluation done => %+v\n", r.ruleID, results)

	res, ok := results[0].Expressions[0].Value.(map[string]interface{})
	if !ok {
		return []*compliance.Report{compliance.BuildReportForError(errors.New("wrong result type"))}
	}

	passed, ok := res["result"].(bool)
	if !ok {
		return []*compliance.Report{compliance.BuildReportForError(errors.New("wrong result type"))}
	}

	denies, err := extractDeniesRIDs(res["denies"])
	if err != nil {
		return []*compliance.Report{compliance.BuildReportForError(err)}
	}

	reports := resultFinalizer(passed, denies)

	log.Debugf("reports: %v", reports)
	return reports
}

func dumpInputToFile(path string, input interface{}) error {
	jsonInputDump, err := json.Marshal(input)
	if err != nil {
		return err
	}

	var buffer bytes.Buffer
	json.Indent(&buffer, jsonInputDump, "", "\t")

	return ioutil.WriteFile(path, buffer.Bytes(), 0644)
}

func (r *regoCheck) appendInstance(resourceInstances map[int]instanceFields, input map[string][]interface{}, key string, instance eval.Instance, idCounter *int, reportedFields []string) {
	vars, exists := input[key]
	if !exists {
		vars = []interface{}{}
	}
	normalized := r.normalizeInputMap(instance.Vars().GoMap())
	normalized["rid"] = *idCounter
	resourceInstances[*idCounter] = instanceFields{
		instance: instance,
		fields:   reportedFields,
	}
	*idCounter++

	input[key] = append(vars, normalized)
}

var regoBuiltins = []func(*rego.Rego){
	octalLiteralFunc,
	denyInput,
}

var octalLiteralFunc = rego.Function1(
	&rego.Function{
		Name: "parseOctal",
		Decl: types.NewFunction(types.Args(types.S), types.N),
	},
	func(_ rego.BuiltinContext, a *ast.Term) (*ast.Term, error) {
		str, ok := a.Value.(ast.String)
		if !ok {
			return nil, errors.New("failed to parse octal literal")
		}

		value, err := strconv.ParseInt(string(str), 8, 0)
		if err != nil {
			return nil, err
		}

		return ast.IntNumberTerm(int(value)), err
	},
)

var denyInput = rego.Function1(
	&rego.Function{
		Name: "deny_input",
		Decl: types.NewFunction(types.Args(types.A), types.N),
	},
	func(_ rego.BuiltinContext, a *ast.Term) (*ast.Term, error) {
		rid, err := a.Value.Find([]*ast.Term{ast.StringTerm("rid")})
		if err != nil {
			return nil, err
		}
		res := ast.NumberTerm(json.Number(rid.String()))
		return res, nil
	},
)

func containsInt(data []int, value int) bool {
	for _, v := range data {
		if v == value {
			return true
		}
	}
	return false
}

func extractDeniesRIDs(denies interface{}) ([]int, error) {
	rids, ok := denies.([]interface{})
	if !ok {
		return nil, errors.New("wrong denies type")
	}

	res := make([]int, 0)
	for _, rid := range rids {
		id, ok := rid.(json.Number)
		if !ok {
			return nil, errors.New("wrond rid type")
		}
		finalRid, err := id.Int64()
		if err != nil {
			return nil, err
		}
		res = append(res, int(finalRid))
	}
	return res, nil
}
