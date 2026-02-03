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
	"github.com/DataDog/datadog-agent/pkg/config/model"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/parversion"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

// isEnabled checks if the private action runner is enabled in the configuration
func isEnabled(cfg config.Component) bool {
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
	workflowRunner *runners.WorkflowRunner
	commonRunner   *runners.CommonRunner
	drain          func()
}

// NewComponent creates a new privateactionrunner component
func NewComponent(reqs Requires) (Provides, error) {
	if !isEnabled(reqs.Config) {
		reqs.Log.Info("private-action-runner is not enabled. Set privateactionrunner.enabled: true in your datadog.yaml file or set the environment variable DD_PRIVATEACTIONRUNNER_ENABLED=true.")
		return Provides{}, privateactionrunner.ErrNotEnabled
	}
	persistedIdentity, err := enrollment.GetIdentityFromPreviousEnrollment(reqs.Config)
	if err != nil {
		return Provides{}, fmt.Errorf("self-enrollment failed: %w", err)
	}
	if persistedIdentity != nil {
		reqs.Config.Set("privateactionrunner.private_key", persistedIdentity.PrivateKey, model.SourceAgentRuntime)
		reqs.Config.Set("privateactionrunner.urn", persistedIdentity.URN, model.SourceAgentRuntime)
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
		reqs.Config.Set("privateactionrunner.private_key", updatedCfg.PrivateKey, model.SourceAgentRuntime)
		reqs.Config.Set("privateactionrunner.urn", updatedCfg.Urn, model.SourceAgentRuntime)
		cfg = updatedCfg
	} else if cfg.IdentityIsIncomplete() {
		return Provides{}, errors.New("identity not found and self-enrollment disabled. Please provide a valid URN and private key")
	}
	reqs.Log.Info("Private action runner starting")
	reqs.Log.Info("==> Version : " + parversion.RunnerVersion)
	reqs.Log.Info("==> Site : " + cfg.DatadogSite)
	reqs.Log.Info("==> URN : " + cfg.Urn)

	keysManager := taskverifier.NewKeyManager(reqs.RcClient)
	taskVerifier := taskverifier.NewTaskVerifier(keysManager, cfg)
	opmsClient := opms.NewClient(cfg)

	r, err := runners.NewWorkflowRunner(cfg, keysManager, taskVerifier, opmsClient)
	if err != nil {
		return Provides{}, err
	}
	runner := &privateactionrunnerImpl{
		workflowRunner: r,
		commonRunner:   runners.NewCommonRunner(cfg),
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
	ctx, cancel := context.WithCancel(context.Background())
	p.drain = cancel
	err := p.commonRunner.Start(ctx)
	if err != nil {
		return err
	}
	return p.workflowRunner.Start(ctx)
}

func (p *privateactionrunnerImpl) Stop(ctx context.Context) error {
	err := p.workflowRunner.Stop(ctx)
	if err != nil {
		return err
	}
	p.drain()
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

	if err := enrollment.PersistIdentity(ddConfig, enrollmentResult); err != nil {
		return nil, fmt.Errorf("failed to persist enrollment identity: %w", err)
	}

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
