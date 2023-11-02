// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

var bundledPolicyRules = []*rules.RuleDefinition{{
	ID:         events.RefreshUserCacheRuleID,
	Expression: `rename.file.destination.path in [ "/etc/passwd", "/etc/group" ]`,
	Actions: []rules.ActionDefinition{{
		InternalCallbackDefinition: &rules.InternalCallbackDefinition{},
	}},
	Silent: true,
}}
