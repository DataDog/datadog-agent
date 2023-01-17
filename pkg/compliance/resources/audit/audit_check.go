// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package audit

import (
	"context"
	"fmt"
	"os"

	"github.com/elastic/go-libaudit/rule"

	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks/env"
	"github.com/DataDog/datadog-agent/pkg/compliance/eval"
	"github.com/DataDog/datadog-agent/pkg/compliance/resources"
	fileutils "github.com/DataDog/datadog-agent/pkg/compliance/utils/file"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var reportedFields = []string{
	compliance.AuditFieldPath,
	compliance.AuditFieldEnabled,
	compliance.AuditFieldPermissions,
}

func resolve(_ context.Context, e env.Env, ruleID string, res compliance.ResourceCommon, rego bool) (resources.Resolved, error) {
	if res.Audit == nil {
		return nil, fmt.Errorf("%s: expecting audit resource in audit check", ruleID)
	}

	audit := res.Audit

	client := e.AuditClient()
	if client == nil {
		return nil, fmt.Errorf("audit client not configured")
	}

	path, err := fileutils.ResolvePath(e, audit.Path)
	if err != nil {
		return nil, err
	}

	normPath := e.NormalizeToHostRoot(path)
	if _, err := os.Stat(normPath); err != nil && os.IsNotExist(err) {
		return nil, fmt.Errorf("%s: audit resource path does not exist", ruleID)
	}

	paths := []string{path}

	log.Debugf("%s: evaluating audit rules", ruleID)

	auditRules, err := client.GetFileWatchRules()
	if err != nil {
		return nil, err
	}

	var instances []resources.ResolvedInstance
	for _, auditRule := range auditRules {
		for _, path := range paths {
			if auditRule.Path != path {
				continue
			}

			log.Debugf("%s: audit check - match %s", ruleID, path)
			auditPermissions := auditPermissionsString(auditRule)
			instances = append(instances, resources.NewResolvedInstance(
				eval.NewInstance(
					eval.VarMap{
						compliance.AuditFieldPath:        path,
						compliance.AuditFieldEnabled:     true,
						compliance.AuditFieldPermissions: auditPermissions,
					},
					nil,
					eval.RegoInputMap{
						"path":        path,
						"enabled":     true,
						"permissions": auditPermissions,
					},
				),
				auditRule.Path, "audit"),
			)
		}
	}

	return resources.NewResolvedInstances(instances), nil
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

func init() {
	resources.RegisterHandler("audit", resolve, reportedFields)
}
