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
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockRegistry struct {
	templates map[string]issuesmod.Template
}

// New returns an empty mock registry. Call RegisterTemplate to add templates.
func New() registrydef.Component {
	return &mockRegistry{templates: make(map[string]issuesmod.Template)}
}

// MockModule provides the mock registry via fx.
func MockModule() fxutil.Module {
	return fxutil.Component(fxutil.ProvideComponentConstructor(New))
}

// RegisterTemplate adds a template under issueName so that GetTemplate succeeds.
// Panics if r was not created by New() — intentional, test-only helper.
func RegisterTemplate(r registrydef.Component, issueName string, tmpl issuesmod.Template) {
	m, ok := r.(*mockRegistry)
	if !ok {
		panic("RegisterTemplate: r was not created by mock.New()")
	}
	m.templates[issueName] = tmpl
}

func (m *mockRegistry) GetTemplate(issueName string) (issuesmod.Template, bool) {
	tmpl, ok := m.templates[issueName]
	return tmpl, ok
}

func (m *mockRegistry) GetBuiltInPeriodicHealthChecks() []*runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}

func (m *mockRegistry) GetBuiltInStartupHealthChecks() []*runnerdef.BuiltInHealthCheck {
	return nil
}
