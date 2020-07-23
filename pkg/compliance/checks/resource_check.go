// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// ErrResourceKindNotSupported is returned in case resource kind is not supported by evaluator
	ErrResourceKindNotSupported = errors.New("resource kind not supported")
)

type checkFunc func(e env.Env, ruleID string, res compliance.Resource, expr *eval.IterableExpression) (*report, error)

type resourceCheck struct {
	ruleID     string
	resource   compliance.Resource
	expression *eval.IterableExpression
	checkFn    checkFunc
}

func (c *resourceCheck) check(env env.Env) (*report, error) {
	return c.checkFn(env, c.ruleID, c.resource, c.expression)
}

func newResourceCheck(env env.Env, ruleID string, resource compliance.Resource) (checkable, error) {
	// TODO: validate resource here
	kind := resource.Kind()

	switch kind {
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

	expression, err := eval.ParseIterable(resource.Condition)
	if err != nil {
		return nil, log.Errorf("%s: failed to parse condition: %s", ruleID, err)
	}

	checkFn, err := checkFuncForKind(kind)
	if err != nil {
		return nil, log.Errorf("%s: failed to resolve evaluator for kind: %s", ruleID, kind)
	}

	return &resourceCheck{
		ruleID:     ruleID,
		resource:   resource,
		expression: expression,
		checkFn:    checkFn,
	}, nil
}

func checkFuncForKind(kind compliance.ResourceKind) (checkFunc, error) {
	switch kind {
	case compliance.KindFile:
		return checkFile, nil
	case compliance.KindAudit:
		return checkAudit, nil
	case compliance.KindGroup:
		return checkGroup, nil
	case compliance.KindCommand:
		return checkCommand, nil
	case compliance.KindProcess:
		return checkProcess, nil
	case compliance.KindDocker:
		return checkDocker, nil
	case compliance.KindKubernetes:
		return checkKubeapiserver, nil
	default:
		return nil, ErrResourceKindNotSupported
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
