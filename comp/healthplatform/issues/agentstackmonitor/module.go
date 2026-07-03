// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package agentstackmonitor provides healthplatform issue templates for the
// agent-stack self-monitoring component. Detection lives in
// comp/agentstackmonitor; this package only builds the proto Issues.
package agentstackmonitor

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

// Source is the reporting label on every IssueReport this component emits,
// and the key used with scheduler.Schedule for resolution diffing.
const Source = "agentstackmonitor"

const (
	IssueNameMemoryPressure     = "AgentStackMonitor: Memory Pressure"
	IssueNameContainerRestart   = "AgentStackMonitor: Container Restart"
	IssueNameContainerOOMKilled = "AgentStackMonitor: Container OOMKilled"
	IssueNameCrashLoopBackOff   = "AgentStackMonitor: CrashLoopBackOff"
)

// AllIssueNames lets the component look up persisted issues at startup so
// the scheduler can resolve any that no longer reproduce.
var AllIssueNames = []string{
	IssueNameMemoryPressure,
	IssueNameContainerRestart,
	IssueNameContainerOOMKilled,
	IssueNameCrashLoopBackOff,
}

func init() {
	issues.RegisterModuleFactory(newMemoryPressureModule)
	issues.RegisterModuleFactory(newContainerRestartModule)
	issues.RegisterModuleFactory(newContainerOOMKilledModule)
	issues.RegisterModuleFactory(newCrashLoopBackOffModule)
}

// templateModule provides only the remediation side; detection is driven
// externally, so both BuiltIn*HealthCheck accessors return nil.
type templateModule struct {
	issueName string
	build     func(map[string]string) (*healthplatform.Issue, error)
}

func (m *templateModule) IssueName() string { return m.issueName }

func (m *templateModule) BuildIssue(ctx map[string]string) (*healthplatform.Issue, error) {
	return m.build(ctx)
}

func (m *templateModule) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

func (m *templateModule) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}

func newMemoryPressureModule(config.Component) issues.Module {
	return &templateModule{issueName: IssueNameMemoryPressure, build: buildMemoryPressureIssue}
}

func newContainerRestartModule(config.Component) issues.Module {
	return &templateModule{issueName: IssueNameContainerRestart, build: buildContainerRestartIssue}
}

func newContainerOOMKilledModule(config.Component) issues.Module {
	return &templateModule{issueName: IssueNameContainerOOMKilled, build: buildContainerOOMKilledIssue}
}

func newCrashLoopBackOffModule(config.Component) issues.Module {
	return &templateModule{issueName: IssueNameCrashLoopBackOff, build: buildCrashLoopBackOffIssue}
}
