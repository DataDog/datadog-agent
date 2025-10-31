// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package privateactionrunnerimpl implements the privateactionrunner component interface
package privateactionrunnerimpl

import (
	"context"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
)

// Requires defines the dependencies for the privateactionrunner component
type Requires struct {
	// Remove this field if the component has no lifecycle hooks
	Lifecycle compdef.Lifecycle
}

// Provides defines the output of the privateactionrunner component
type Provides struct {
	Comp privateactionrunner.Component
}

type privateactionrunnerImpl struct {
	WorkflowRunner *runners.WorkflowRunner
}

// NewComponent creates a new privateactionrunner component
func NewComponent(reqs Requires) (Provides, error) {
	runner := &privateactionrunnerImpl{
		WorkflowRunner: runners.NewWorkflowRunner(),
	}
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: runner.Start,
		OnStop:  runner.Stop,
	})
	return Provides{
		Comp: runner,
	}, nil
}

func (p *privateactionrunnerImpl) Start(ctx context.Context) error {
	return p.WorkflowRunner.Start(ctx)
}

func (p *privateactionrunnerImpl) Stop(ctx context.Context) error {
	return p.WorkflowRunner.Close(ctx)
}
