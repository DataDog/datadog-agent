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

	// ErrResourceFallbackMissing is returned when a resource relies on fallback but no fallback is provided
	ErrResourceFallbackMissing = errors.New("resource fallback missing")

	// ErrResourceCannotUseFallback is returned when a resource cannot use fallback
	ErrResourceCannotUseFallback = errors.New("resource cannot use fallback")

	// ErrResourceFailedToResolve is returned when a resource failed to resolve to any instances for evaluation
	ErrResourceFailedToResolve = errors.New("failed to resolve resource")
)

type resolved interface {
	Evaluate(conditionExpression *eval.IterableExpression, c *resourceCheck, env env.Env) []*compliance.Report
}
type resolvedInstance interface {
	eval.Instance
	ID() string
	Type() string
}

type _resolvedInstance struct {
	eval.Instance
	id   string
	kind string
}

func (ri *_resolvedInstance) ID() string {
	return ri.id
}

func (ri *_resolvedInstance) Type() string {
	return ri.kind
}

func (ri *_resolvedInstance) Evaluate(conditionExpression *eval.IterableExpression, c *resourceCheck, env env.Env) []*compliance.Report {
	if c.resource.Fallback != nil {
		if c.fallback == nil {
			return []*compliance.Report{compliance.BuildReportForError(ErrResourceFallbackMissing)}
		}

		fallbackExpression, err := eval.Cache.ParseExpression(c.resource.Fallback.Condition)
		if err != nil {
			return []*compliance.Report{compliance.BuildReportForError(err)}
		}

		useFallback, err := fallbackExpression.BoolEvaluate(ri.Instance)
		if err != nil {
			return []*compliance.Report{compliance.BuildReportForError(err)}
		}
		if useFallback {
			return c.fallback.check(env)
		}
	}

	passed, err := conditionExpression.Evaluate(ri.Instance)
	if err != nil {
		return []*compliance.Report{compliance.BuildReportForError(err)}
	}

	report := instanceToReport(ri, passed, c.reportedFields)
	return []*compliance.Report{report}
}

func newResolvedInstance(instance eval.Instance, resourceID, resourceType string) *_resolvedInstance {
	return &_resolvedInstance{
		Instance: instance,
		id:       resourceID,
		kind:     resourceType,
	}
}

type resolvedIterator struct {
	eval.Iterator
}

func (ri *resolvedIterator) Evaluate(conditionExpression *eval.IterableExpression, c *resourceCheck, env env.Env) []*compliance.Report {
	if c.resource.Fallback != nil {
		return []*compliance.Report{compliance.BuildReportForError(ErrResourceCannotUseFallback)}
	}

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

func newResolvedIterator(iterator eval.Iterator) *resolvedIterator {
	return &resolvedIterator{
		Iterator: iterator,
	}
}

func newResolvedInstances(resolvedInstances []resolvedInstance) *resolvedIterator {
	instances := make([]eval.Instance, len(resolvedInstances))
	for i, ri := range resolvedInstances {
		instances[i] = ri
	}
	return newResolvedIterator(newInstanceIterator(instances))
}

type resolveFunc func(ctx context.Context, e env.Env, ruleID string, resource compliance.Resource) (resolved, error)

type resourceCheck struct {
	ruleID   string
	resource compliance.Resource

	resolve  resolveFunc
	fallback checkable

	reportedFields []string
}

func (c *resourceCheck) check(env env.Env) []*compliance.Report {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	resolved, err := c.resolve(ctx, env, c.ruleID, c.resource)
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

	var fallback checkable
	if resource.Fallback != nil {
		fallback, err = newResourceCheck(env, ruleID, resource.Fallback.Resource)
		if err != nil {
			return nil, err
		}
	}

	return &resourceCheck{
		ruleID:         ruleID,
		resource:       resource,
		resolve:        resolve,
		fallback:       fallback,
		reportedFields: reportedFields,
	}, nil
}

func resourceKindToResolverAndFields(env env.Env, ruleID string, kind compliance.ResourceKind) (resolveFunc, []string, error) {
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
	default:
		return nil, nil, ErrResourceKindNotSupported
	}
}

func newResourceCheckList(env env.Env, ruleID string, resources []compliance.Resource) (checkable, error) {
	var checks checkableList
	for _, resource := range resources {
		c, err := newResourceCheck(env, ruleID, resource)
		if err != nil {
			return nil, err
		}
		checks = append(checks, c)
	}
	return checks, nil
}
