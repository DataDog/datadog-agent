// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package app provides the core private action runner application that can be
// embedded in different agents (standalone, cluster-agent, etc.)
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/parversion"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/rcclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// App represents a private action runner application instance.
type App struct {
	coreConfig model.ReaderWriter
	rcClient   rcclient.Client
	drain      func(ctx context.Context) error
}

// ErrNotEnabled is returned when the private action runner is not enabled in config.
var ErrNotEnabled = errors.New("private action runner is not enabled")

// NewApp creates a new private action runner application.
// It handles enrollment, configuration, and sets up the runners on startup.
func NewApp(coreConfig model.ReaderWriter, rcClient rcclient.Client) (*App, error) {
	if rcClient == nil {
		return nil, errors.New("rcClient cannot be nil")
	}
	return &App{
		coreConfig: coreConfig,
		rcClient:   rcClient,
	}, nil
}

// Start starts the private action runner.
// It returns immediately after starting the runners in background goroutines.
func (a *App) Start(_ context.Context) error {
	if !a.coreConfig.GetBool("privateactionrunner.enabled") {
		pkglog.Info("private-action-runner is not enabled. Set privateactionrunner.enabled: true in your datadog.yaml file or set the environment variable DD_PRIVATEACTIONRUNNER_ENABLED=true.")
		return ErrNotEnabled
	}
	cfg, err := a.getRunnerConfig()
	if err != nil {
		return err
	}

	pkglog.Info("Private action runner starting")
	pkglog.Infof("==> Version : %s", parversion.RunnerVersion)
	pkglog.Infof("==> Site : %s", cfg.DatadogSite)
	pkglog.Infof("==> URN : %s", cfg.Urn)

	keysManager := taskverifier.NewKeyManager(a.rcClient)
	taskVerifier := taskverifier.NewTaskVerifier(keysManager, cfg)
	opmsClient := opms.NewClient(cfg)

	workflowRunner, err := runners.NewWorkflowRunner(cfg, keysManager, taskVerifier, opmsClient)
	if err != nil {
		return err
	}
	commonRunner := runners.NewCommonRunner(cfg)

	// Use background context to avoid inheriting any deadlines from component lifecycle which stop the PAR loop
	ctx, mainCtxCancel := context.WithCancel(context.Background())

	a.drain = func(ctx context.Context) error {
		if err := workflowRunner.Stop(ctx); err != nil {
			return err
		}
		if err := commonRunner.Stop(ctx); err != nil {
			return err
		}
		mainCtxCancel()
		return nil
	}

	if err := commonRunner.Start(ctx); err != nil {
		return err
	}
	return workflowRunner.Start(ctx)
}

func (a *App) getRunnerConfig() (*parconfig.Config, error) {
	persistedIdentity, err := enrollment.GetIdentityFromPreviousEnrollment(a.coreConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get persisted identity: %w", err)
	}
	if persistedIdentity != nil {
		a.coreConfig.Set("privateactionrunner.private_key", persistedIdentity.PrivateKey, model.SourceAgentRuntime)
		a.coreConfig.Set("privateactionrunner.urn", persistedIdentity.URN, model.SourceAgentRuntime)
	}

	cfg, err := parconfig.FromDDConfig(a.coreConfig)
	if err != nil {
		return nil, err
	}

	canSelfEnroll := a.coreConfig.GetBool("privateactionrunner.self_enroll")
	if cfg.IdentityIsIncomplete() && canSelfEnroll {
		pkglog.Info("Identity not found and self-enrollment enabled. Self-enrolling private action runner")
		updatedCfg, err := performSelfEnrollment(a.coreConfig, cfg)
		if err != nil {
			return nil, fmt.Errorf("self-enrollment failed: %w", err)
		}
		a.coreConfig.Set("privateactionrunner.private_key", updatedCfg.PrivateKey, model.SourceAgentRuntime)
		a.coreConfig.Set("privateactionrunner.urn", updatedCfg.Urn, model.SourceAgentRuntime)
		cfg = updatedCfg
	} else if cfg.IdentityIsIncomplete() {
		return nil, errors.New("identity not found and self-enrollment disabled. Please provide a valid URN and private key")
	}
	return cfg, nil
}

// Stop stops the private action runner gracefully.
func (a *App) Stop(ctx context.Context) error {
	return a.drain(ctx)
}

// performSelfEnrollment handles the self-registration of a private action runner
func performSelfEnrollment(coreConfig model.ReaderWriter, cfg *parconfig.Config) (*parconfig.Config, error) {
	ddSite := coreConfig.GetString("site")
	apiKey := coreConfig.GetString("api_key")
	appKey := coreConfig.GetString("app_key")

	env.DetectFeatures(coreConfig)
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
	pkglog.Info("Self-enrollment successful")

	if err := enrollment.PersistIdentity(coreConfig, enrollmentResult); err != nil {
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
