// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a test mock of the health platform issue registry component.
package mock

import (
	"testing"

	registrydef "github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/def"
	issuesmod "github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

type mockRegistry struct {
	t         testing.TB
	templates map[string]issuesmod.Template
	periodic  []*runnerdef.BuiltInPeriodicHealthCheck
	startup   []*runnerdef.BuiltInHealthCheck
}

// Option configures the mock registry returned by New.
type Option func(*mockRegistry)

// WithTemplate registers a template so that GetTemplate(issueName) succeeds.
func WithTemplate(issueName string, tmpl issuesmod.Template) Option {
	return func(m *mockRegistry) { m.templates[issueName] = tmpl }
}

// WithPeriodicCheck appends a check returned by GetBuiltInPeriodicHealthChecks.
func WithPeriodicCheck(check *runnerdef.BuiltInPeriodicHealthCheck) Option {
	return func(m *mockRegistry) { m.periodic = append(m.periodic, check) }
}

// WithStartupCheck appends a check returned by GetBuiltInStartupHealthChecks.
func WithStartupCheck(check *runnerdef.BuiltInHealthCheck) Option {
	return func(m *mockRegistry) { m.startup = append(m.startup, check) }
}

// New returns a mock registry pre-populated with the given options.
func New(t testing.TB, opts ...Option) registrydef.Component {
	m := &mockRegistry{t: t, templates: make(map[string]issuesmod.Template)}
	for _, o := range opts {
		o(m)
	}
	return m
}

func (m *mockRegistry) GetTemplate(issueName string) (issuesmod.Template, bool) {
	tmpl, ok := m.templates[issueName]
	return tmpl, ok
}

func (m *mockRegistry) GetBuiltInPeriodicHealthChecks() []*runnerdef.BuiltInPeriodicHealthCheck {
	return m.periodic
}

func (m *mockRegistry) GetBuiltInStartupHealthChecks() []*runnerdef.BuiltInHealthCheck {
	return m.startup
}
