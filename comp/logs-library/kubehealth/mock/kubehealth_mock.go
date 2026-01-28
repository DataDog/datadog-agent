// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package mock provides a mock implementation of the KubeHealthRegistrar
package mock

import (
	"go.uber.org/fx"

	kubehealthdef "github.com/DataDog/datadog-agent/comp/logs-library/kubehealth/def"
	"github.com/DataDog/datadog-agent/pkg/status/health"
)

// Registrar is a mock implementation of KubeHealthRegistrar
type Registrar struct {
	registeredChecks map[string]int
}

// Provides is the mock component output
type Provides struct {
	fx.Out

	Comp kubehealthdef.Component
}

// NewProvides provides a new MockRegistrar
func NewProvides() Provides {
	return Provides{
		Comp: NewMockRegistrar(),
	}
}

// NewMockRegistrar creates a new Registrar
func NewMockRegistrar() *Registrar {
	return &Registrar{
		registeredChecks: make(map[string]int),
	}
}

// RegisterReadiness registers a readiness check with the health package
func (r *Registrar) RegisterReadiness(name string, _ ...health.Option) *health.Handle {
	r.registeredChecks[name]++
	return &health.Handle{}
}

// RegisterLiveness registers a liveness check with the health package
func (r *Registrar) RegisterLiveness(name string, _ ...health.Option) *health.Handle {
	r.registeredChecks[name]++
	return &health.Handle{}
}

// RegisterStartup registers a startup check with the health package
func (r *Registrar) RegisterStartup(name string, _ ...health.Option) *health.Handle {
	r.registeredChecks[name]++
	return &health.Handle{}
}

// Deregister deregisters a health check with the health package
func (r *Registrar) Deregister(_ *health.Handle) error {
	return nil
}

// CountRegistered checks how many times a health check has been registered
func (r *Registrar) CountRegistered(name string) int {
	return r.registeredChecks[name]
}
