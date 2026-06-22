// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a test mock of the health platform issue registry component.
package mock

import (
	registrydef "github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/def"
	issuesmod "github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

// Mock is a test implementation of issueregistry.Component.
// Use RegisterTemplate, RegisterBuiltInPeriodicHealthCheck, and
// RegisterBuiltInStartupHealthCheck to populate it before the test runs.
type Mock struct {
	templates map[string]issuesmod.Template
	periodic  []*runnerdef.BuiltInPeriodicHealthCheck
	startup   []*runnerdef.BuiltInHealthCheck
}

// New returns an empty mock registry.
func New() *Mock {
	return &Mock{templates: make(map[string]issuesmod.Template)}
}

// RegisterTemplate adds a template so that GetTemplate(issueName) succeeds.
func (m *Mock) RegisterTemplate(issueName string, tmpl issuesmod.Template) {
	m.templates[issueName] = tmpl
}

// RegisterBuiltInPeriodicHealthCheck adds a check returned by GetBuiltInPeriodicHealthChecks.
func (m *Mock) RegisterBuiltInPeriodicHealthCheck(check *runnerdef.BuiltInPeriodicHealthCheck) {
	m.periodic = append(m.periodic, check)
}

// RegisterBuiltInStartupHealthCheck adds a check returned by GetBuiltInStartupHealthChecks.
func (m *Mock) RegisterBuiltInStartupHealthCheck(check *runnerdef.BuiltInHealthCheck) {
	m.startup = append(m.startup, check)
}

func (m *Mock) GetTemplate(issueName string) (issuesmod.Template, bool) {
	tmpl, ok := m.templates[issueName]
	return tmpl, ok
}

func (m *Mock) GetBuiltInPeriodicHealthChecks() []*runnerdef.BuiltInPeriodicHealthCheck {
	return m.periodic
}

func (m *Mock) GetBuiltInStartupHealthChecks() []*runnerdef.BuiltInHealthCheck {
	return m.startup
}

// ensure Mock satisfies the interface at compile time.
var _ registrydef.Component = (*Mock)(nil)
