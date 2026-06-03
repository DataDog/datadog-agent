// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !jetson

package invalidsysprobeconfig

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

// IssueID is the stable Agent Health identifier for system-probe configuration-schema violations.
const IssueID = "invalid-system-probe-config"

func init() {
	issues.RegisterModuleFactory(NewModule)
}

type invalidSysprobeConfigModule struct {
	checker *checker
}

// NewModule captures the system-probe config so the once-only startup check can read it
func NewModule(deps issues.ModuleDeps) issues.Module {
	return &invalidSysprobeConfigModule{checker: newChecker(deps.SysProbeConfig)}
}

func (m *invalidSysprobeConfigModule) IssueName() string {
	return IssueID
}

func (m *invalidSysprobeConfigModule) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return InvalidSysprobeConfigIssue{}.BuildIssue(context)
}

// BuiltInPeriodicHealthCheck returns nil as schema validation runs only at startup.
func (m *invalidSysprobeConfigModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

// BuiltInStartupHealthCheck runs the system-probe schema validation once at agent startup.
func (m *invalidSysprobeConfigModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	if m.checker.cfg == nil {
		return nil
	}
	return &runnerdef.BuiltInHealthCheck{
		Source: "system-probe",
		Fn:     m.checker.Run,
	}
}
