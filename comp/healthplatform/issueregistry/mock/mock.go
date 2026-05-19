// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a test mock of the health platform issue registry component.
package mock

import (
	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	registrydef "github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/def"
	issuesmod "github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockRegistry struct {
	templates map[string]issuesmod.IssueTemplate
}

// New returns an empty mock registry. Call RegisterTemplate to add templates.
func New() registrydef.Component {
	return &mockRegistry{templates: make(map[string]issuesmod.IssueTemplate)}
}

// MockModule provides the mock registry via fx.
func MockModule() fxutil.Module {
	return fxutil.Component(fxutil.ProvideComponentConstructor(New))
}

// RegisterTemplate adds a template under issueType so that BuildIssue succeeds.
// Panics if r was not created by New() — intentional, test-only helper.
func RegisterTemplate(r registrydef.Component, issueType string, tmpl issuesmod.IssueTemplate) {
	r.(*mockRegistry).templates[issueType] = tmpl //nolint:forcetypeassert
}

func (m *mockRegistry) BuildIssue(issueType string, context map[string]string) (*healthplatformpayload.Issue, error) {
	if tmpl, ok := m.templates[issueType]; ok {
		return tmpl.BuildIssue(context)
	}
	return &healthplatformpayload.Issue{IssueName: issueType}, nil
}

func (m *mockRegistry) HasTemplate(issueType string) bool {
	_, ok := m.templates[issueType]
	return ok
}

func (m *mockRegistry) GetBuiltInPeriodicHealthChecks() []*issuesmod.BuiltInPeriodicHealthCheck {
	return nil
}

func (m *mockRegistry) GetBuiltInStartupHealthChecks() []*issuesmod.BuiltInStartupHealthCheck {
	return nil
}
