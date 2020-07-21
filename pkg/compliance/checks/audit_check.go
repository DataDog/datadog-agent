// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package checks

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/elastic/go-libaudit/rule"
)

const (
	auditFieldPath        = "audit.path"
	auditFieldEnabled     = "audit.enabled"
	auditFieldPermissions = "audit.permissions"
)

var auditReportedFields = []string{
	auditFieldPath,
	auditFieldEnabled,
	auditFieldPermissions,
}

func checkAudit(e env.Env, ruleID string, res compliance.Resource, expr *eval.IterableExpression) (*report, error) {
	if res.Audit == nil {
		return nil, fmt.Errorf("%s: expecting audit resource in audit check", ruleID)
	}

	audit := res.Audit

	client := e.AuditClient()
	if client == nil {
		return nil, fmt.Errorf("audit client not configured")
	}

	path, err := resolvePath(e, audit.Path)
	if err != nil {
		return nil, err
	}

	paths := []string{path}

	log.Debugf("%s: evaluating audit rules", ruleID)

	auditRules, err := client.GetFileWatchRules()
	if err != nil {
		return nil, err
	}

	var instances []*eval.Instance
	for _, auditRule := range auditRules {
		for _, path := range paths {
			if auditRule.Path != path {
				continue
			}

			log.Debugf("%s: audit check - match %s", ruleID, path)
			instances = append(instances, &eval.Instance{
				Vars: eval.VarMap{
					auditFieldPath:        path,
					auditFieldEnabled:     true,
					auditFieldPermissions: auditPermissionsString(auditRule),
				},
			})
		}
	}

	it := &instanceIterator{
		instances: instances,
	}

	result, err := expr.EvaluateIterator(it, globalInstance)
	if err != nil {
		return nil, err
	}

	return instanceResultToReport(result, auditReportedFields), nil
}

func resolvePath(e env.Env, path string) (string, error) {

	pathExpr, err := eval.ParsePath(path)
	if err != nil {
		return "", err
	}

	if pathExpr.Path != nil {
		return *pathExpr.Path, nil
	}

	v, err := e.EvaluateFromCache(pathExpr)
	if err != nil {
		return "", err
	}

	path, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("resource path expression not resolved to string: %s", path)
	}

	return path, nil
}

func auditPermissionsString(r *rule.FileWatchRule) string {
	permissions := ""
	for _, p := range r.Permissions {
		switch p {
		case rule.ReadAccessType:
			permissions += "r"
		case rule.WriteAccessType:
			permissions += "w"
		case rule.ExecuteAccessType:
			permissions += "e"
		case rule.AttributeChangeAccessType:
			permissions += "a"
		}
	}
	return permissions
}
