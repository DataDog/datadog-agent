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
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/parversion"
	pkgrcclient "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/rcclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/autoconnections"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

// Configuration keys for the private action runner.
// These mirror the constants in pkg/config/setup but are defined here
// because comp/ packages cannot import pkg/config/setup (depguard rule).
const (
	parEnabled    = "private_action_runner.enabled"
	parSelfEnroll = "private_action_runner.self_enroll"
	parPrivateKey = "private_action_runner.private_key"
	parUrn        = "private_action_runner.urn"
)

// isEnabled checks if the private action runner is enabled in the configuration
func isEnabled(cfg config.Component) bool {
	return cfg.GetBool(parEnabled)
}

// Requires defines the dependencies for the privateactionrunner component
type Requires struct {
	Config    config.Component
	Log       log.Component
	Lifecycle compdef.Lifecycle
	RcClient  rcclient.Component
	Hostname  hostname.Component
}

// Provides defines the output of the privateactionrunner component
type Provides struct {
	Comp privateactionrunner.Component
}

type PrivateActionRunner struct {
	workflowRunner *runners.WorkflowRunner
	commonRunner   *runners.CommonRunner
	drain          func()
}

// NewComponent creates a new privateactionrunner component
func NewComponent(reqs Requires) (Provides, error) {
	ctx := context.Background()
	if !isEnabled(reqs.Config) {
		reqs.Log.Info("private-action-runner is not enabled. Set privateactionrunner.enabled: true in your datadog.yaml file or set the environment variable DD_PRIVATEACTIONRUNNER_ENABLED=true.")
		return Provides{}, privateactionrunner.ErrNotEnabled
	}

	runner, err := NewPrivateActionRunner(ctx, reqs.Config, reqs.Hostname, pkgrcclient.NewAdapter(reqs.RcClient), reqs.Log)
	if err != nil {
		return Provides{}, err
	}
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: runner.Start,
		OnStop:  runner.Stop,
	})
	return Provides{Comp: runner}, nil
}

func NewPrivateActionRunner(
	ctx context.Context,
	coreConfig model.ReaderWriter,
	hostnameGetter hostnameinterface.Component,
	rcClient pkgrcclient.Client,
	logger log.Component,
) (*PrivateActionRunner, error) {
	persistedIdentity, err := enrollment.GetIdentityFromPreviousEnrollment(coreConfig)
	if err != nil {
		return nil, fmt.Errorf("self-enrollment failed: %w", err)
	}
	if persistedIdentity != nil {
		coreConfig.Set(parPrivateKey, persistedIdentity.PrivateKey, model.SourceAgentRuntime)
		coreConfig.Set(parUrn, persistedIdentity.URN, model.SourceAgentRuntime)
	}

	cfg, err := parconfig.FromDDConfig(coreConfig)
	if err != nil {
		return nil, err
	}

	canSelfEnroll := coreConfig.GetBool(parSelfEnroll)
	if cfg.IdentityIsIncomplete() && canSelfEnroll {
		logger.Info("Identity not found and self-enrollment enabled. Self-enrolling private action runner")
		updatedCfg, err := performSelfEnrollment(ctx, logger, coreConfig, hostnameGetter, cfg)
		if err != nil {
			return nil, fmt.Errorf("self-enrollment failed: %w", err)
		}
		coreConfig.Set(parPrivateKey, updatedCfg.PrivateKey, model.SourceAgentRuntime)
		coreConfig.Set(parUrn, updatedCfg.Urn, model.SourceAgentRuntime)
		cfg = updatedCfg
	} else if cfg.IdentityIsIncomplete() {
		return nil, errors.New("identity not found and self-enrollment disabled. Please provide a valid URN and private key")
	}
	logger.Info("Private action runner starting")
	logger.Info("==> Version : " + parversion.RunnerVersion)
	logger.Info("==> Site : " + cfg.DatadogSite)
	logger.Info("==> URN : " + cfg.Urn)

	keysManager := taskverifier.NewKeyManager(rcClient)
	taskVerifier := taskverifier.NewTaskVerifier(keysManager, cfg)
	opmsClient := opms.NewClient(cfg)

	r, err := runners.NewWorkflowRunner(cfg, keysManager, taskVerifier, opmsClient)
	if err != nil {
		return nil, err
	}
	runner := &PrivateActionRunner{
		workflowRunner: r,
		commonRunner:   runners.NewCommonRunner(cfg),
	}
	return runner, nil
}

func (p *PrivateActionRunner) Start(_ context.Context) error {
	// Use background context to avoid inheriting any deadlines from component lifecycle which stop the PAR loop
	ctx, cancel := context.WithCancel(context.Background())
	p.drain = cancel
	err := p.commonRunner.Start(ctx)
	if err != nil {
		return err
	}
	return p.workflowRunner.Start(ctx)
}

func (p *PrivateActionRunner) Stop(ctx context.Context) error {
	err := p.workflowRunner.Stop(ctx)
	if err != nil {
		return err
	}
	p.drain()
	return nil
}

// performSelfEnrollment handles the self-registration of a private action runner
func performSelfEnrollment(ctx context.Context, log log.Component, ddConfig config.Component, hostnameComp hostnameinterface.Component, cfg *parconfig.Config) (*parconfig.Config, error) {
	ddSite := ddConfig.GetString("site")
	apiKey := ddConfig.GetString("api_key")
	appKey := ddConfig.GetString("app_key")

	runnerHostname, err := hostnameComp.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}
	now := time.Now().UTC()
	formattedTime := now.Format("20060102150405")
	runnerName := runnerHostname + "-" + formattedTime

	enrollmentResult, err := enrollment.SelfEnroll(ctx, ddSite, runnerName, apiKey, appKey)
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

	// Auto-create connections for enrolled runner
	var actionsAllowlist = make([]string, 0)
	for _, set := range cfg.ActionsAllowlist {
		for action := range set {
			actionsAllowlist = append(actionsAllowlist, action)
		}
	}

	if len(actionsAllowlist) > 0 {
		client, err := autoconnections.NewConnectionsAPIClient(ddConfig, ddSite, apiKey, appKey)
		if err != nil {
			log.Warnf("Failed to create connections API client: %v", err)
		} else {
			creator := autoconnections.NewConnectionsCreator(*client)

			if err := creator.AutoCreateConnections(context.Background(), urnParts.RunnerID, runnerName, actionsAllowlist); err != nil {
				log.Warnf("Failed to auto-create connections: %v", err)
			}
		}
	}

	return cfg, nil
}
