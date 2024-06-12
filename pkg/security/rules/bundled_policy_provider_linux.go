// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

func newBundledPolicyRules(cfg *config.RuntimeSecurityConfig) []*rules.RuleDefinition {
	if cfg.EBPFLessEnabled {
		return []*rules.RuleDefinition{}
	}
	return []*rules.RuleDefinition{{
		ID:         events.RefreshUserCacheRuleID,
		Expression: `rename.file.destination.path in ["/etc/passwd", "/etc/group"]`,
		Actions: []*rules.ActionDefinition{{
			InternalCallback: &rules.InternalCallbackDefinition{},
		}},
		Silent: true,
	}, {
		ID:         events.NeedRefreshSBOMRuleID,
		Expression: `open.file.path in [~"/lib/rpm/*", ~"/lib/dpkg/*", ~"/var/lib/rpm/*", ~"/var/lib/dpkg/*", ~"/lib/apk/*"] && (open.flags & (O_CREAT | O_RDWR | O_WRONLY)) > 0`,
		Actions: []*rules.ActionDefinition{{
			InternalCallback: &rules.InternalCallbackDefinition{},
			Set: &rules.SetDefinition{
				Name:  "pkg_db_modified",
				Scope: "process",
				Value: true,
			},
		}},
		Silent: true,
	}, {
		ID:         events.RefreshSBOMRuleID,
		Expression: `exit.cause == EXITED && ${process.pkg_db_modified}`,
		Actions: []*rules.ActionDefinition{{
			InternalCallback: &rules.InternalCallbackDefinition{},
		}},
		Silent: true,
	}}
}
