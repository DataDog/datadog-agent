// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package privateactionrunnerimpl implements the privateactionrunner component interface
package privateactionrunnerimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/remoteconfig"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
)

// Requires defines the dependencies for the privateactionrunner component
type Requires struct {
	compdef.In
	Lifecycle compdef.Lifecycle
	Logger    log.Component
	Config    config.Component
	RcClient  rcclient.Component
}

// Provides defines the output of the privateactionrunner component
type Provides struct {
	Comp privateactionrunner.Component
}

type runnerImpl struct {
	log          log.Component
	config       config.Component
	started      bool
	keysManager  remoteconfig.KeysManager
	TaskVerifier *taskverifier.TaskVerifier
}

// NewComponent creates a new privateactionrunner component
func NewComponent(reqs Requires) (Provides, error) {

	//ctx := context.Background()
	cfg := parconfig.Config{}
	opmsClient := opms.NewClient(&cfg)
	println(opmsClient)
	keysManager := remoteconfig.New(reqs.RcClient)

	runner := &runnerImpl{
		log:          reqs.Logger,
		config:       reqs.Config,
		keysManager:  keysManager,
		TaskVerifier: taskverifier.NewTaskVerifier(keysManager, &cfg),
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: runner.Start,
		OnStop:  runner.Stop,
	})
	return Provides{
		Comp: runner,
	}, nil
}

func (r *runnerImpl) Start(ctx context.Context) error {
	enabled := r.config.GetBool("privateactionrunner.enabled")
	if !enabled {
		r.log.Debug("privateactionrunner disabled")
		return nil
	}
	r.log.Info("Starting private action runner")
	r.started = true
	r.keysManager.Start(ctx)
	return nil
}

func (r *runnerImpl) Stop(ctx context.Context) error {
	if !r.started {
		return nil
	}
	r.log.Info("Stopping private action runner")
	return nil
}
