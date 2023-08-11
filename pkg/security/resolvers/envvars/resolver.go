// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package envvars

import (
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// Resolver exported type should have comment or be unexported
type Resolver struct {
	priorityEnvs []string
}

// NewEnvVarsResolver exported function should have comment or be unexported
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

// ResolveEnvVars exported method should have comment or be unexported
func (r *Resolver) ResolveEnvVars(pid int32) ([]string, bool, error) {
	return utils.EnvVars(r.priorityEnvs, pid)
}
