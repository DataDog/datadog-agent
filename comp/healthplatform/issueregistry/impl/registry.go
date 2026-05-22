// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package issueregistryimpl implements the health platform issue registry component.
package issueregistryimpl

import (
	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	registrydef "github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/def"
	issuesmod "github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

// Requires defines the dependencies for the registry component.
type Requires struct {
	Config config.Component
}

type registryImpl struct {
	inner *issuesmod.Registry
}

// New creates the issue registry, instantiating all self-registered modules.
func New(reqs Requires) registrydef.Component {
	r := issuesmod.NewRegistry()
	for _, module := range issuesmod.GetAllModules(reqs.Config) {
		r.RegisterModule(module)
	}
	return &registryImpl{inner: r}
}

func (r *registryImpl) BuildIssue(issueName string, context map[string]string) (*healthplatformpayload.Issue, error) {
	return r.inner.BuildIssue(issueName, context)
}

func (r *registryImpl) HasTemplate(issueName string) bool {
	_, ok := r.inner.GetTemplate(issueName)
	return ok
}

func (r *registryImpl) GetBuiltInPeriodicHealthChecks() []*runnerdef.BuiltInPeriodicHealthCheck {
	return r.inner.GetBuiltInPeriodicHealthChecks()
}

func (r *registryImpl) GetBuiltInStartupHealthChecks() []*runnerdef.BuiltInHealthCheck {
	return r.inner.GetBuiltInStartupHealthChecks()
}
