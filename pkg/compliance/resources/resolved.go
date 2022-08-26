// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
)

type Resolved interface {
	Evaluate(conditionExpression *eval.IterableExpression, env env.Env) []*compliance.Report
}

type ResolvedInstance interface {
	eval.Instance
	ID() string
	Type() string
	// Report(bool) *compliance.Report
}

type resolvedInstance struct {
	eval.Instance
	id            string
	kind          string
	allowedFields []string
}

func (ri *resolvedInstance) ID() string {
	return ri.id
}

func (ri *resolvedInstance) Type() string {
	return ri.kind
}

// Report converts an instance and passed status to report
// filtering out fields not on the allowedFields list
func (ri *resolvedInstance) Report(passed bool) *compliance.Report {
	var data event.Data
	var resourceReport compliance.ReportResource

	if ri != nil {
		data = instanceToEventData(ri, ri.allowedFields)
		resourceReport = compliance.ReportResource{
			ID:   ri.ID(),
			Type: ri.Type(),
		}
	}

	return &compliance.Report{
		Resource: resourceReport,
		Passed:   passed,
		Data:     data,
	}
}

// instanceToEventData converts an instance to event data filtering out fields not on the allowedFields list
func instanceToEventData(instance eval.Instance, allowedFields []string) event.Data {
	data := event.Data{}

	for k, v := range instance.Vars() {
		allow := false
		for _, a := range allowedFields {
			if k == a {
				allow = true
				break
			}
		}
		if !allow {
			continue
		}
		data[k] = v
	}
	return data
}

func (ri *resolvedInstance) Evaluate(conditionExpression *eval.IterableExpression, env env.Env) []*compliance.Report {
	passed, err := conditionExpression.Evaluate(ri.Instance)
	if err != nil {
		return []*compliance.Report{compliance.BuildReportForError(err)}
	}

	report := ri.Report(passed)
	return []*compliance.Report{report}
}

func NewResolvedInstance(instance eval.Instance, resourceID, resourceType string) *resolvedInstance {
	return &resolvedInstance{
		Instance: instance,
		id:       resourceID,
		kind:     resourceType,
	}
}

type resolvedIterator struct {
	eval.Iterator
}

func instanceResultToReport(result *eval.InstanceResult) *compliance.Report {
	resolvedInstance, _ := result.Instance.(*resolvedInstance)
	return resolvedInstance.Report(result.Passed)
}

func (ri *resolvedIterator) Evaluate(conditionExpression *eval.IterableExpression, env env.Env) []*compliance.Report {
	results, err := conditionExpression.EvaluateIterator(ri.Iterator, globalInstance)
	if err != nil {
		return []*compliance.Report{compliance.BuildReportForError(err)}
	}

	var reports []*compliance.Report
	for _, result := range results {
		report := instanceResultToReport(result)
		reports = append(reports, report)
	}

	return reports
}

func NewResolvedIterator(iterator eval.Iterator) *resolvedIterator {
	return &resolvedIterator{
		Iterator: iterator,
	}
}

func NewResolvedInstances(resolvedInstances []ResolvedInstance) *resolvedIterator {
	instances := make([]eval.Instance, len(resolvedInstances))
	for i, ri := range resolvedInstances {
		instances[i] = ri
	}
	return NewResolvedIterator(NewInstanceIterator(instances))
}

type Resolver func(ctx context.Context, e env.Env, ruleID string, resource compliance.ResourceCommon, rego bool) (Resolved, error)
