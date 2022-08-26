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
	"github.com/DataDog/datadog-agent/pkg/compliance/resources"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources/audit"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources/command"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources/docker"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources/file"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources/group"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// ErrResourceKindNotSupported is returned in case resource kind is not supported by evaluator
	ErrResourceKindNotSupported = errors.New("resource kind not supported")

	// ErrResourceFailedToResolve is returned when a resource failed to resolve to any instances for evaluation
	ErrResourceFailedToResolve = errors.New("failed to resolve resource")
)

type resourceCheck struct {
	ruleID   string
	resource compliance.Resource

	resolve resources.Resolver

	reportedFields []string
}

func (c *resourceCheck) check(env env.Env) []*compliance.Report {
	ctx, cancel := context.WithTimeout(context.Background(), compliance.DefaultTimeout)
	defer cancel()

	resolved, err := c.resolve(ctx, env, c.ruleID, c.resource.ResourceCommon, false)
	if err != nil {
		return []*compliance.Report{compliance.BuildReportForError(err)}
	}

	conditionExpression, err := eval.Cache.ParseIterable(c.resource.Condition)
	if err != nil {
		return []*compliance.Report{compliance.BuildReportForError(err)}
	}

	return resolved.Evaluate(conditionExpression, env)
}

func resourceKindToResolverAndFields(env env.Env, kind compliance.ResourceKind) (resources.Resolver, []string, error) {
	switch kind {
	case compliance.KindFile:
		return file.Resolve, file.ReportedFields, nil
	case compliance.KindAudit:
		return audit.Resolve, audit.ReportedFields, nil
	case compliance.KindGroup:
		return group.Resolve, group.ReportedFields, nil
	case compliance.KindCommand:
		return command.Resolve, command.ReportedFields, nil
	case compliance.KindProcess:
		return resolveProcess, processReportedFields, nil
	case compliance.KindDocker:
		if env.DockerClient() == nil {
			return nil, nil, log.Errorf("%s: docker client not initialized")
		}
		return docker.Resolve, docker.ReportedFields, nil
	case compliance.KindKubernetes:
		if env.KubeClient() == nil {
			return nil, nil, log.Errorf("%s: kube client not initialized")
		}
		return resolveKubeapiserver, kubeResourceReportedFields, nil
	case compliance.KindConstants:
		return resolveConstants, nil, nil
	default:
		return nil, nil, ErrResourceKindNotSupported
	}
}
