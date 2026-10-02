// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package issueregistryimpl implements the health platform issue registry component.
package issueregistryimpl

import (
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	registrydef "github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/def"
	issuesmod "github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Requires defines the dependencies for the registry component.
type Requires struct {
	compdef.In
	Log     log.Component
	Modules []issuesmod.Module `group:"healthplatform_issue"`
}

type registryImpl struct {
	inner *issuesmod.Registry
}

// NewComponent creates the issue registry from the injected issue modules.
func NewComponent(reqs Requires) registrydef.Component {
	r := issuesmod.NewRegistry()
	for _, m := range fxutil.GetAndFilterGroup(reqs.Modules) {
		if _, dup := r.GetTemplate(m.IssueName()); dup {
			reqs.Log.Warnf("duplicate health platform issue module for %q; ignoring", m.IssueName())
			continue
		}
		r.RegisterModule(m)
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
