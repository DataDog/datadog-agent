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

type invalidConfigModule struct {
	checker *checker
}

// NewModule captures the config so the once-only startup check can read it.
func NewModule(cfg config.Component) issues.Module {
	return &invalidConfigModule{checker: newChecker(cfg)}
}

func (m *invalidConfigModule) IssueID() string {
	return healthplatformdef.InvalidConfigIssueID
}

func (m *invalidConfigModule) IssueTemplate() issues.IssueTemplate {
	return InvalidConfigIssue{}
}

// BuiltInHealthCheck runs the schema validation once at agent startup. The
// agent does not hot-reload datadog.yaml, so the verdict can't change at
// runtime — Once: true tells the platform to skip periodic re-invocation.
func (m *invalidConfigModule) BuiltInHealthCheck() *issues.BuiltInHealthCheck {
	return &issues.BuiltInHealthCheck{
		ID:      healthplatformdef.InvalidConfigCheckID,
		Name:    healthplatformdef.InvalidConfigCheckName,
		CheckFn: m.checker.Run,
		Once:    true,
	}
}
