// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

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

type resolveFunc func(ctx context.Context, e env.Env, ruleID string, resource compliance.Resource) (interface{}, error)

type resourceCheck struct {
	ruleID   string
	resource compliance.Resource

	resolve  resolveFunc
	fallback checkable

	reportedFields []string
}

func (c *resourceCheck) check(env env.Env) (*compliance.Report, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	resolved, err := c.resolve(ctx, env, c.ruleID, c.resource)
	if err != nil {
		return nil, err
	}

	return c.evaluate(env, resolved)
}

func (c *resourceCheck) evaluate(env env.Env, resolved interface{}) (*compliance.Report, error) {
	conditionExpression, err := eval.Cache.ParseIterable(c.resource.Condition)
	if err != nil {
		return nil, err
	}

	switch resolved := resolved.(type) {
	case *eval.Instance:
		if c.resource.Fallback != nil {
			if c.fallback == nil {
				return nil, ErrResourceFallbackMissing
			}

			fallbackExpression, err := eval.Cache.ParseExpression(c.resource.Fallback.Condition)
			if err != nil {
				return nil, err
			}

			useFallback, err := fallbackExpression.BoolEvaluate(resolved)
			if err != nil {
				return nil, err
			}
			if useFallback {
				return c.fallback.check(env)
			}
		}

		passed, err := conditionExpression.Evaluate(resolved)
		if err != nil {
			return nil, err
		}
		return instanceToReport(resolved, passed, c.reportedFields), nil

	case eval.Iterator:
		if c.resource.Fallback != nil {
			return nil, ErrResourceCannotUseFallback
		}

		result, err := conditionExpression.EvaluateIterator(resolved, globalInstance)
		if err != nil {
			return nil, err
		}
		return instanceResultToReport(result, c.reportedFields), nil
	default:
		return nil, ErrResourceFailedToResolve
	}
}

func newResourceCheck(env env.Env, ruleID string, resource compliance.Resource) (checkable, error) {
	// TODO: validate resource here
	kind := resource.Kind()

	switch kind {
	case compliance.KindCustom:
		return newCustomCheck(ruleID, resource)
	case compliance.KindAudit:
		if env.AuditClient() == nil {
			return nil, log.Errorf("%s: audit client not initialized", ruleID)
		}
	case compliance.KindDocker:
		if env.DockerClient() == nil {
			return nil, log.Errorf("%s: docker client not initialized", ruleID)
		}
	case compliance.KindKubernetes:
		if env.KubeClient() == nil {
			return nil, log.Errorf("%s: kube client not initialized", ruleID)
		}
	}

	resolve, reportedFields, err := resourceKindToResolverAndFields(kind)
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

func resourceKindToResolverAndFields(kind compliance.ResourceKind) (resolveFunc, []string, error) {
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
		return resolveDocker, dockerReportedFields, nil
	case compliance.KindKubernetes:
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
