// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package kubehealthimpl provides a wrapper around the health package to allow for easier registration of health checks
package kubehealthimpl

import (
	kubehealthdef "github.com/DataDog/datadog-agent/comp/logs-library/kubehealth/def"
	validatordef "github.com/DataDog/datadog-agent/comp/logs-library/validator/def"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type Dependencies struct {
	Validator validatordef.Component
}

// Provides contains the kubehealth component
type Provides struct {
	Comp option.Option[kubehealthdef.Component]
}

// RegistrarImpl is an implementation of KubeHealthRegistrar
type RegistrarImpl struct{}

// NewRegistrar creates a new Registrar
func newRegistrar(_ Dependencies) kubehealthdef.Component {
	return &RegistrarImpl{}
}

// NewProvides provides a new Registrar
func NewProvides(deps Dependencies) Provides {
	return Provides{
		Comp: validatordef.GenOption(deps.Validator, deps, newRegistrar),
	}
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
