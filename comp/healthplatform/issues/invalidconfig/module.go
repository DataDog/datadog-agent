// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !jetson

package invalidconfig

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
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

func (m *invalidConfigModule) IssueType() string {
	return storedef.InvalidConfigIssueID
}

func (m *invalidConfigModule) IssueTemplate() issues.IssueTemplate {
	return InvalidConfigIssue{}
}

// BuiltInHealthCheck runs the schema validation once at agent startup
func (m *invalidConfigModule) BuiltInPeriodicHealthCheck() *issues.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck runs the schema validation once at agent startup.
func (m *invalidConfigModule) BuiltInStartupHealthCheck() *issues.BuiltInStartupHealthCheck {
	return &issues.BuiltInStartupHealthCheck{
		Source: "agent",
		Fn:     m.checker.Run,
	}
}
