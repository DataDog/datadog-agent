// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package scenario

import (
	"context"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

// Action is a named post-deploy operation on a running scenario. Run receives the
// fully-hydrated typed environment (same clients a test gets from s.Env()).
type Action[Env any] struct {
	Description string
	NewParams   func() any // returns a pointer to this action's tagged params struct
	Run         func(ctx context.Context, env *Env, params any) error
}

// Scenario is the single, authoritative definition of a scenario.
type Scenario[Env any] struct {
	Name        string
	Description string
	// NewParams returns a pointer to the canonical tagged params struct.
	NewParams func() any
	// Provisioner builds a provisioner from decoded params (adapter to existing
	// framework provisioners, e.g. awshost.Provisioner).
	Provisioner func(params any) (provisioners.TypedProvisioner[Env], error)
	Actions     map[string]Action[Env]
}
