// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package invalidconfig

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

// invalidConfigModule implements issues.Module
type invalidConfigModule struct {
	template *InvalidConfigIssue
	checker  *checker
}

// NewModule creates a new invalid-config issue module. The config component
// is captured so the periodic check can read ConfigFileUsed at run time.
func NewModule(cfg config.Component) issues.Module {
	return &invalidConfigModule{
		template: NewInvalidConfigIssue(),
		checker:  newChecker(cfg),
	}
}

// IssueID returns the unique identifier for this issue type.
func (m *invalidConfigModule) IssueID() string {
	return healthplatformdef.InvalidConfigIssueID
}

// IssueTemplate returns the template for building complete issues.
func (m *invalidConfigModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns the periodic schema-validation check. Interval is
// left at zero so the platform applies its default (15 minutes), matching
// the forwarder cadence.
func (m *invalidConfigModule) BuiltInCheck() *issues.BuiltInCheck {
	return &issues.BuiltInCheck{
		ID:      healthplatformdef.InvalidConfigCheckID,
		Name:    healthplatformdef.InvalidConfigCheckName,
		CheckFn: m.checker.Run,
	}
}
