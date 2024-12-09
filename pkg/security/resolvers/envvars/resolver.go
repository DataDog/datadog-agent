// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package envvars holds envvars related files
package envvars

import (
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// Resolver defines a resolver
type Resolver struct {
	priorityEnvs []string
}

// NewEnvVarsResolver returns a new resolver
func NewEnvVarsResolver(cfg *config.Config) *Resolver {
	var envsWithValue []string
	if cfg != nil {
		envsWithValue = cfg.EnvsWithValue
	}

	pe := make([]string, 0, len(envsWithValue)+1)
	pe = append(pe, "DD_SERVICE")
	pe = append(pe, envsWithValue...)

	return &Resolver{
		priorityEnvs: pe,
	}
}

// ResolveEnvVars resolves a pid
func (r *Resolver) ResolveEnvVars(pid uint32) ([]string, bool, error) {
	// we support the r == nil, for when env vars resolution is disabled
	if r == nil {
		// communicate the fact that it was truncated
		return nil, true, nil
	}
	return utils.EnvVars(r.priorityEnvs, pid, model.MaxArgsEnvsSize)
}
