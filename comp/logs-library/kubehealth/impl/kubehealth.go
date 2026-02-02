// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package kubehealthimpl provides a wrapper around the health package to allow for easier registration of health checks
package kubehealthimpl

import (
	depvalidator "github.com/DataDog/datadog-agent/comp/logs-library/depvalidator/def"
	kubehealthdef "github.com/DataDog/datadog-agent/comp/logs-library/kubehealth/def"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Requires defines the dependencies for the kubehealth component
type Requires struct {
	DepValidator depvalidator.Component
}

// Provides contains the kubehealth component
type Provides struct {
	Comp option.Option[kubehealthdef.Component]
}

// RegistrarImpl is an implementation of KubeHealthRegistrar
type RegistrarImpl struct{}

// NewRegistrar creates a new Registrar
func newRegistrar() *RegistrarImpl {
	return &RegistrarImpl{}
}

// NewProvides provides a new Registrar if logs are enabled
func NewProvides(reqs Requires) Provides {
	if err := reqs.DepValidator.ValidateIfEnabled(reqs); err != nil {
		return Provides{Comp: option.None[kubehealthdef.Component]()}
	}
	return Provides{Comp: option.New[kubehealthdef.Component](newRegistrar())}
}

// RegisterReadiness registers a readiness check with the health package
func (r *RegistrarImpl) RegisterReadiness(name string, options ...health.Option) *health.Handle {
	return health.RegisterReadiness(name, options...)
}

// RegisterLiveness registers a liveness check with the health package
func (r *RegistrarImpl) RegisterLiveness(name string, options ...health.Option) *health.Handle {
	return health.RegisterLiveness(name, options...)
}

// RegisterStartup registers a startup check with the health package
func (r *RegistrarImpl) RegisterStartup(name string, options ...health.Option) *health.Handle {
	return health.RegisterStartup(name, options...)
}

// Deregister deregisters a health check with the health package
func (r *RegistrarImpl) Deregister(handle *health.Handle) error {
	return health.Deregister(handle)
}
