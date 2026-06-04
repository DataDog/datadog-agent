// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !jetson

package invalidconfig

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

// IssueID is the stable Agent Health identifier for configuration-schema violations
const IssueID = "invalid-config"

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

func (m *invalidConfigModule) IssueName() string {
	return IssueID
}

func (m *invalidConfigModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return InvalidConfigIssue{}.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil as schema validation runs only at startup
func (m *invalidConfigModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck runs the schema validation once at agent startup.
func (m *invalidConfigModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return &runnerdef.BuiltInHealthCheck{
		Source: "agent",
		Fn:     m.checker.Run,
	}
}
