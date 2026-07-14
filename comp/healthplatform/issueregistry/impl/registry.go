// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package issueregistryimpl implements the health platform issue registry component.
package issueregistryimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	registrydef "github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/def"
	issuesmod "github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

// Requires defines the dependencies for the registry component.
type Requires struct {
	Config         config.Component
	SysProbeConfig sysprobeconfig.Component `optional:"true"`
	Hostname       hostnameinterface.Component
}

type registryImpl struct {
	inner *issuesmod.Registry
}

// NewComponent creates the issue registry, instantiating all self-registered modules.
func NewComponent(reqs Requires) registrydef.Component {
	r := issuesmod.NewRegistry()
	deps := issuesmod.ModuleDeps{
		Config:         reqs.Config,
		SysProbeConfig: reqs.SysProbeConfig,
		Hostname:       reqs.Hostname,
	}
	for _, module := range issuesmod.GetAllModules(deps) {
		r.RegisterModule(module)
	}
	return &registryImpl{inner: r}
}

func (r *registryImpl) GetTemplate(issueName string) (issuesmod.Template, bool) {
	return r.inner.GetTemplate(issueName)
}

func (r *registryImpl) GetBuiltInPeriodicHealthChecks() []*runnerdef.BuiltInPeriodicHealthCheck {
	return r.inner.GetBuiltInPeriodicHealthChecks()
}

func (r *registryImpl) GetBuiltInStartupHealthChecks() []*runnerdef.BuiltInHealthCheck {
	return r.inner.GetBuiltInStartupHealthChecks()
}
