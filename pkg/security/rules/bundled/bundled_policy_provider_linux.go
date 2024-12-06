// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bundled contains bundled rules
package bundled

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func newBundledPolicyRules(cfg *config.RuntimeSecurityConfig) []*rules.RuleDefinition {
	if cfg.EBPFLessEnabled {
		return []*rules.RuleDefinition{}
	}

	ruleDefinitions := []*rules.RuleDefinition{{
		ID:         RefreshUserCacheRuleID,
		Expression: `rename.file.destination.path in ["/etc/passwd", "/etc/group"]`,
		Silent:     true,
	}}

	if cfg.SBOMResolverEnabled {
		ruleDefinitions = append(ruleDefinitions, &rules.RuleDefinition{
			ID:         NeedRefreshSBOMRuleID,
			Expression: `open.file.path in [~"/lib/rpm/*", ~"/lib/dpkg/*", ~"/var/lib/rpm/*", ~"/var/lib/dpkg/*", ~"/lib/apk/*"] && (open.flags & (O_CREAT | O_RDWR | O_WRONLY)) > 0`,
			Actions: []*rules.ActionDefinition{{
				Set: &rules.SetDefinition{
					Name:  needRefreshSBOMVariableName,
					Scope: needRefreshSBOMVariableScope,
					Value: true,
				},
			}},
			Silent: true,
		}, &rules.RuleDefinition{
			ID:         RefreshSBOMRuleID,
			Expression: fmt.Sprintf("exit.cause == EXITED && ${%s.%s}", needRefreshSBOMVariableScope, needRefreshSBOMVariableName),
			Silent:     true,
		})
	}

	return ruleDefinitions
}
