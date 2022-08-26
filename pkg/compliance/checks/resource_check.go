// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checks

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// ErrResourceKindNotSupported is returned in case resource kind is not supported by evaluator
	ErrResourceKindNotSupported = errors.New("resource kind not supported")

	// ErrResourceFailedToResolve is returned when a resource failed to resolve to any instances for evaluation
	ErrResourceFailedToResolve = errors.New("failed to resolve resource")
)

type Resolved interface {
	Evaluate(conditionExpression *eval.IterableExpression, c *resourceCheck, env env.Env) []*compliance.Report
}
type ResolvedInstance interface {
	eval.Instance
	ID() string
	Type() string
}

type resolvedInstance struct {
	eval.Instance
	id   string
	kind string
}

func (ri *resolvedInstance) ID() string {
	return ri.id
}

func (ri *resolvedInstance) Type() string {
	return ri.kind
}

func (ri *resolvedInstance) Evaluate(conditionExpression *eval.IterableExpression, c *resourceCheck, env env.Env) []*compliance.Report {
	passed, err := conditionExpression.Evaluate(ri.Instance)
	if err != nil {
		return []*compliance.Report{compliance.BuildReportForError(err)}
	}

	report := instanceToReport(ri, passed, c.reportedFields)
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

func (ri *resolvedIterator) Evaluate(conditionExpression *eval.IterableExpression, c *resourceCheck, env env.Env) []*compliance.Report {
	results, err := conditionExpression.EvaluateIterator(ri.Iterator, globalInstance)
	if err != nil {
		return []*compliance.Report{compliance.BuildReportForError(err)}
	}

	var reports []*compliance.Report
	for _, result := range results {
		report := instanceResultToReport(result, c.reportedFields)
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
	return NewResolvedIterator(newInstanceIterator(instances))
}

type Resolver func(ctx context.Context, e env.Env, ruleID string, resource compliance.ResourceCommon, rego bool) (Resolved, error)

type resourceCheck struct {
	ruleID   string
	resource compliance.Resource

	resolve Resolver

	reportedFields []string
}

func (c *resourceCheck) check(env env.Env) []*compliance.Report {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	resolved, err := c.resolve(ctx, env, c.ruleID, c.resource.ResourceCommon, false)
	if err != nil {
		return []*compliance.Report{compliance.BuildReportForError(err)}
	}

	conditionExpression, err := eval.Cache.ParseIterable(c.resource.Condition)
	if err != nil {
		return []*compliance.Report{compliance.BuildReportForError(err)}
	}

	return resolved.Evaluate(conditionExpression, c, env)
}

func newResourceCheck(env env.Env, ruleID string, resource compliance.Resource) (checkable, error) {
	// TODO: validate resource here
	kind := resource.Kind()

	switch kind {
	case compliance.KindCustom:
		return newCustomCheck(ruleID, resource)
	}

	resolve, reportedFields, err := resourceKindToResolverAndFields(env, ruleID, kind)
	if err != nil {
		return nil, log.Errorf("%s: failed to find resource resolver for resource kind: %s", ruleID, kind)
	}

	return &resourceCheck{
		ruleID:         ruleID,
		resource:       resource,
		resolve:        resolve,
		reportedFields: reportedFields,
	}, nil
}

func resourceKindToResolverAndFields(env env.Env, ruleID string, kind compliance.ResourceKind) (Resolver, []string, error) {
	switch kind {
	case compliance.KindFile:
		return resolveFile, fileReportedFields, nil
	case compliance.KindAudit:
		return resolveAudit, auditReportedFields, nil
	case compliance.KindGroup:
		return resolveGroup, groupReportedFields, nil
	case compliance.KindCommand:
		return resolveCommand, commandReportedFields, nil
	case compliance.KindProcess:
		return resolveProcess, processReportedFields, nil
	case compliance.KindDocker:
		if env.DockerClient() == nil {
			return nil, nil, log.Errorf("%s: docker client not initialized", ruleID)
		}
		return resolveDocker, dockerReportedFields, nil
	case compliance.KindKubernetes:
		if env.KubeClient() == nil {
			return nil, nil, log.Errorf("%s: kube client not initialized", ruleID)
		}
		return resolveKubeapiserver, kubeResourceReportedFields, nil
	case compliance.KindConstants:
		return resolveConstants, nil, nil
	default:
		return nil, nil, ErrResourceKindNotSupported
	}
}
