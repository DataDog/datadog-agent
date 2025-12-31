// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package privateactionrunnerimpl implements the privateactionrunner component interface
package privateactionrunnerimpl

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/remote-config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
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

	canSelfEnroll := reqs.Config.GetBool("privateactionrunner.self_enroll")
	if cfg.IdentityIsIncomplete() && canSelfEnroll {
		reqs.Log.Info("Identity not found and self-enrollment enabled. Self-enrolling private action runner")
		updatedCfg, err := performSelfEnrollment(reqs.Log, reqs.Config, cfg)
		if err != nil {
			return Provides{}, fmt.Errorf("self-enrollment failed: %w", err)
		}
		cfg = updatedCfg
	} else if cfg.IdentityIsIncomplete() {
		return Provides{}, errors.New("identity not found and self-enrollment disabled. Please provide a valid URN and private key")
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

// performSelfEnrollment handles the self-registration of a private action runner
func performSelfEnrollment(log log.Component, ddConfig config.Component, cfg *parconfig.Config) (*parconfig.Config, error) {
	ddSite := ddConfig.GetString("site")
	apiKey := ddConfig.GetString("api_key")
	appKey := ddConfig.GetString("app_key")

	env.DetectFeatures(ddConfig)
	runnerHostname, err := hostname.Get(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}
	now := time.Now().UTC()
	formattedTime := now.Format("20060102150405")
	runnerName := runnerHostname + "-" + formattedTime

	enrollmentResult, err := enrollment.SelfEnroll(ddSite, runnerName, apiKey, appKey)
	if err != nil {
		return nil, fmt.Errorf("enrollment API call failed: %w", err)
	}
	log.Info("Self-enrollment successful")

	cfg.Urn = enrollmentResult.URN
	cfg.PrivateKey = enrollmentResult.PrivateKey

	urnParts, err := util.ParseRunnerURN(enrollmentResult.URN)
	if err != nil {
		return nil, fmt.Errorf("failed to parse enrollment URN: %w", err)
	}
	cfg.OrgId = urnParts.OrgID
	cfg.RunnerId = urnParts.RunnerID

	return cfg, nil
}
