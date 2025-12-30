// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package privateactionrunnerimpl implements the privateactionrunner component interface
package privateactionrunnerimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/remote-config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
)

// IsEnabled checks if the private action runner is enabled in the configuration
func IsEnabled(cfg config.Component) bool {
	return cfg.GetBool("privateactionrunner.enabled")
}

// Requires defines the dependencies for the privateactionrunner component
type Requires struct {
	Config    config.Component
	Log       log.Component
	Lifecycle compdef.Lifecycle
	RcClient  rcclient.Component
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
	if !IsEnabled(reqs.Config) {
		// Return a no-op component when disabled
		return Provides{
			Comp: &privateactionrunnerImpl{},
		}, nil
	}
	cfg, err := parconfig.FromDDConfig(reqs.Config)
	if err != nil {
		return Provides{}, err
	}
	keysManager := remoteconfig.New(reqs.RcClient)
	taskVerifier := taskverifier.NewTaskVerifier(keysManager, cfg)
	opmsClient := opms.NewClient(cfg)

	r, err := runners.NewWorkflowRunner(cfg, keysManager, taskVerifier, opmsClient)
	if err != nil {
		return Provides{}, err
	}
	runner := &privateactionrunnerImpl{
		WorkflowRunner: r,
	}
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: runner.Start,
		OnStop:  runner.Stop,
	})
	return Provides{
		Comp: runner,
	}, nil
}

func (p *privateactionrunnerImpl) Start(_ context.Context) error {
	// Use background context to avoid inheriting any deadlines from component lifecycle which stop the PAR loop
	p.WorkflowRunner.Start(context.Background())
	return nil
}

func (p *privateactionrunnerImpl) Stop(ctx context.Context) error {
	p.WorkflowRunner.Close(ctx)
	return nil
}
