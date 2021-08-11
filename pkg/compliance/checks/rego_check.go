// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"errors"
	"fmt"

	"github.com/open-policy-agent/opa/rego"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type regoCheck struct {
	ruleID            string
	resources         []compliance.RegoResource
	preparedEvalQuery rego.PreparedEvalQuery
}

func (r *regoCheck) compileRule(rule *compliance.RegoRule) error {
	ctx := context.TODO()

	preparedEvalQuery, err := rego.New(
		rego.Query("result = "+rule.Query),
		rego.Module(fmt.Sprintf("rule_%s.rego", rule.ID), rule.Module),
	).PrepareForEval(ctx)

	if err != nil {
		return err
	}

	r.preparedEvalQuery = preparedEvalQuery

	return nil
}

func (r *regoCheck) check(env env.Env) []*compliance.Report {
	log.Debugf("%s: rego check starting", r.ruleID)

	input := make(map[string][]interface{})

	instances := make(map[resolvedInstance][]string)

	for _, resource := range r.resources {
		resolve, reportedFields, err := resourceKindToResolverAndFields(env, r.ruleID, resource.Kind())
		if err != nil {
			return []*compliance.Report{compliance.BuildReportForError(err)}
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)

		resolved, err := resolve(ctx, env, r.ruleID, resource.BaseResource)
		if err != nil {
			cancel()
			return []*compliance.Report{compliance.BuildReportForError(err)}
		}
		cancel()

		switch instance := resolved.(type) {
		case resolvedInstance:
			vars, exists := input[string(resource.Kind())]
			if !exists {
				vars = []interface{}{}
			}
			input[string(resource.Kind())+"s"] = append(vars, instance.Vars().GoMap())

			instances[instance] = reportedFields
		}
	}

	ctx := context.TODO()
	results, err := r.preparedEvalQuery.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return []*compliance.Report{compliance.BuildReportForError(err)}
	} else if len(results) == 0 {
		return nil
	}

	log.Debugf("%s: rego evaluation done => %+v", results)

	passed, ok := results[0].Bindings["result"].(bool)
	if !ok {
		return []*compliance.Report{compliance.BuildReportForError(errors.New("wrong result type"))}
	}

	var reports []*compliance.Report
	for instance, reportedFields := range instances {
		report := instanceToReport(instance, passed, reportedFields)
		reports = append(reports, report)
	}

	return reports
}
