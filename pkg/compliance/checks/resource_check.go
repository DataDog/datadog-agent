// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// ErrResourceKindNotSupported is returned in case resource kind is not supported by evaluator
	ErrResourceKindNotSupported = errors.New("resource kind not supported")

	// ErrResourceUseFallback is returned when a resource cannot provide check result and relies on fallback
	ErrResourceUseFallback = errors.New("resource check uses fallback")

	// ErrResourceFallbackMissing is returned when a resource relies on fallback but no fallback is provided
	ErrResourceFallbackMissing = errors.New("resource fallback missing")
)

type checkFunc func(e env.Env, ruleID string, resource compliance.Resource) (*compliance.Report, error)

type resourceCheck struct {
	ruleID   string
	resource compliance.Resource

	checkFn  checkFunc
	fallback checkable
}

func (c *resourceCheck) check(env env.Env) (*compliance.Report, error) {
	report, err := c.checkFn(env, c.ruleID, c.resource)

	if err == ErrResourceUseFallback {
		if c.fallback != nil {
			return c.fallback.check(env)
		}
		return nil, ErrResourceFallbackMissing
	}

	return report, err
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

	checkFn, err := checkFuncForKind(kind)
	if err != nil {
		return nil, log.Errorf("%s: failed to resolve check handler for kind: %s", ruleID, kind)
	}

	var fallback checkable
	if resource.Fallback != nil {
		fallback, err = newResourceCheck(env, ruleID, resource.Fallback.Resource)
		if err != nil {
			return nil, err
		}
	}

	return &resourceCheck{
		ruleID:   ruleID,
		resource: resource,
		checkFn:  checkFn,
		fallback: fallback,
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
	case compliance.KindCustom:
		return checkCustom, nil
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
