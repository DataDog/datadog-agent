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
	"regexp"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsutils "github.com/DataDog/datadog-agent/comp/core/secrets/utils"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
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

	maxStartupWaitTimeout = 15 * time.Second
)

var (
	// apiKeyRegex matches valid Datadog API keys (32 hexadecimal characters)
	apiKeyRegex = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
	// appKeyRegex matches valid Datadog application keys (40 hexadecimal characters)
	appKeyRegex = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)
)

func validateAPIKey(key string) error {
	if key == "" {
		return errors.New("api_key is required but not set")
	}
	if isEnc, _ := secretsutils.IsEnc(key); isEnc {
		return errors.New("api_key contains unresolved secret (ENC[...] format). Check secret_backend_command/secret_backend_type configuration")
	}
	if !apiKeyRegex.MatchString(key) {
		return fmt.Errorf("api_key has invalid format (expected 32 hexadecimal characters, got %d characters)", len(key))
	}
	return nil
}

func validateAppKey(key string) error {
	if key == "" {
		return errors.New("app_key is required but not set")
	}
	if isEnc, _ := secretsutils.IsEnc(key); isEnc {
		return errors.New("app_key contains unresolved secret (ENC[...] format). Check secret_backend_command/secret_backend_type configuration")
	}
	if !appKeyRegex.MatchString(key) {
		return fmt.Errorf("app_key has invalid format (expected 40 hexadecimal characters, got %d characters)", len(key))
	}
	return nil
}

// isEnabled checks if the private action runner is enabled in the configuration
func isEnabled(cfg config.Component) bool {
	return cfg.GetBool(parEnabled)
}

// Requires defines the dependencies for the privateactionrunner component
type Requires struct {
	Config        config.Component
	Log           log.Component
	Lifecycle     compdef.Lifecycle
	RcClient      rcclient.Component
	Hostname      hostname.Component
	Tagger        tagger.Component
	Wmeta         workloadmeta.Component
	Traceroute    traceroute.Component
	EventPlatform eventplatform.Component
}

// Provides defines the output of the privateactionrunner component
type Provides struct {
	Comp privateactionrunner.Component
}

type PrivateActionRunner struct {
	coreConfig     model.ReaderWriter
	hostnameGetter hostnameinterface.Component
	rcClient       pkgrcclient.Client
	logger         log.Component
	tagger         tagger.Component
	wmeta          workloadmeta.Component
	traceroute     traceroute.Component
	eventPlatform  eventplatform.Component

	workflowRunner *runners.WorkflowRunner
	commonRunner   *runners.CommonRunner

	started     bool
	startOnce   sync.Once
	startChan   chan struct{}
	cancelStart context.CancelFunc
}

// NewComponent creates a new privateactionrunner component
func NewComponent(reqs Requires) (Provides, error) {
	ctx := context.Background()
	if !isEnabled(reqs.Config) {
		reqs.Log.Info("private-action-runner is not enabled. Set private_action_runner.enabled: true in your datadog.yaml file or set the environment variable DD_PRIVATE_ACTION_RUNNER_ENABLED=true.")
		return Provides{}, privateactionrunner.ErrNotEnabled
	}

	runner, err := NewPrivateActionRunner(ctx, reqs.Config, reqs.Hostname, pkgrcclient.NewAdapter(reqs.RcClient), reqs.Log, reqs.Tagger, reqs.Wmeta, reqs.Traceroute, reqs.EventPlatform)
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
	_ context.Context,
	coreConfig model.ReaderWriter,
	hostnameGetter hostnameinterface.Component,
	rcClient pkgrcclient.Client,
	logger log.Component,
	taggerComp tagger.Component,
	wmeta workloadmeta.Component,
	tracerouteComp traceroute.Component,
	eventPlatform eventplatform.Component,
) (*PrivateActionRunner, error) {
	return &PrivateActionRunner{
		coreConfig:     coreConfig,
		hostnameGetter: hostnameGetter,
		rcClient:       rcClient,
		logger:         logger,
		tagger:         taggerComp,
		wmeta:          wmeta,
		traceroute:     tracerouteComp,
		eventPlatform:  eventPlatform,
		startChan:      make(chan struct{}),
	}, nil
}

func (p *PrivateActionRunner) getRunnerConfig(ctx context.Context) (*parconfig.Config, error) {
	persistedIdentity, err := enrollment.GetIdentityFromPreviousEnrollment(ctx, p.coreConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get identity: %w", err)
	}
	if persistedIdentity != nil {
		p.coreConfig.Set(parPrivateKey, persistedIdentity.PrivateKey, model.SourceAgentRuntime)
		p.coreConfig.Set(parUrn, persistedIdentity.URN, model.SourceAgentRuntime)
	}

	cfg, err := parconfig.FromDDConfig(p.coreConfig)
	if err != nil {
		return nil, err
	}

	canSelfEnroll := p.coreConfig.GetBool(parSelfEnroll)
	if cfg.IdentityIsIncomplete() && canSelfEnroll {
		p.logger.Info("Identity not found and self-enrollment enabled. Self-enrolling private action runner")
		updatedCfg, err := p.performSelfEnrollment(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("self-enrollment failed: %w", err)
		}
		p.coreConfig.Set(parPrivateKey, updatedCfg.PrivateKey, model.SourceAgentRuntime)
		p.coreConfig.Set(parUrn, updatedCfg.Urn, model.SourceAgentRuntime)
		cfg = updatedCfg
	} else if cfg.IdentityIsIncomplete() {
		return nil, errors.New("identity not found and self-enrollment disabled. Please provide a valid URN and private key")
	}
	return cfg, nil
}

func (p *PrivateActionRunner) Start(ctx context.Context) error {
	var err error
	p.started = true
	p.startOnce.Do(func() {
		defer close(p.startChan)
		err = p.start(ctx)
	})
	return err
}

func (p *PrivateActionRunner) StartAsync(ctx context.Context) <-chan error {
	errChan := make(chan error, 1)
	go func() {
		errChan <- p.Start(ctx)
		close(errChan)
	}()
	return errChan
}

func (p *PrivateActionRunner) start(ctx context.Context) error {
	// Keep the parent context's deadline for the startup phase (config, enrollment, etc.)
	// but allow Stop() to cancel as well.
	ctx, p.cancelStart = context.WithCancel(ctx)
	cfg, err := p.getRunnerConfig(ctx)
	if err != nil {
		return err
	}
	p.logger.Info("Private action runner starting")
	p.logger.Info("==> Version : " + parversion.RunnerVersion)
	p.logger.Info("==> Site : " + cfg.DatadogSite)
	p.logger.Info("==> URN : " + cfg.Urn)

	keysManager := taskverifier.NewKeyManager(p.rcClient)
	taskVerifier := taskverifier.NewTaskVerifier(keysManager, cfg)
	opmsClient := opms.NewClient(cfg)

	p.workflowRunner, err = runners.NewWorkflowRunner(cfg, keysManager, taskVerifier, opmsClient, p.wmeta, p.traceroute, p.eventPlatform)
	if err != nil {
		return err
	}
	p.commonRunner = runners.NewCommonRunner(cfg)
	err = p.workflowRunner.Start(ctx)
	if err != nil {
		return err
	}
	return p.commonRunner.Start(ctx)
}

func (p *PrivateActionRunner) Stop(ctx context.Context) error {
	if !p.started {
		return nil // Never started, nothing to stop
	}

	p.cancelStart()
	waitCtx, cancelWaitCtx := context.WithTimeout(ctx, maxStartupWaitTimeout)
	defer cancelWaitCtx()
	err := p.waitForStartup(waitCtx)
	if err != nil {
		p.logger.Warn("PAR startup did not complete in time, forcing cleanup")
		// Don't return - continue to cleanup what we can
	}

	if p.workflowRunner != nil {
		err := p.workflowRunner.Stop(ctx)
		if err != nil {
			return err
		}
	}
	if p.commonRunner != nil {
		err := p.commonRunner.Stop(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *PrivateActionRunner) waitForStartup(ctx context.Context) error {
	select {
	case <-p.startChan:
		// Startup completed normally
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// performSelfEnrollment handles the self-registration of a private action runner
func (p *PrivateActionRunner) performSelfEnrollment(ctx context.Context, cfg *parconfig.Config) (*parconfig.Config, error) {
	ddSite := cfg.DatadogSite
	apiKey := p.coreConfig.GetString("api_key")
	appKey := p.coreConfig.GetString("app_key")

	if err := validateAPIKey(apiKey); err != nil {
		return nil, fmt.Errorf("invalid api_key: %w", err)
	}

	if err := validateAppKey(appKey); err != nil {
		return nil, fmt.Errorf("invalid app_key: %w", err)
	}

	runnerHostname, err := p.hostnameGetter.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}

	enrollmentResult, err := enrollment.SelfEnroll(ctx, ddSite, runnerHostname, apiKey, appKey)
	if err != nil {
		return nil, fmt.Errorf("enrollment API call failed: %w", err)
	}
	p.logger.Info("Self-enrollment successful")

	if err := enrollment.PersistIdentity(ctx, p.coreConfig, enrollmentResult); err != nil {
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
	var actionsAllowlist = make([]string, 0, len(cfg.ActionsAllowlist))
	for fqnPrefix := range cfg.ActionsAllowlist {
		actionsAllowlist = append(actionsAllowlist, fqnPrefix)
	}

	if len(actionsAllowlist) > 0 {
		client, err := autoconnections.NewConnectionsAPIClient(p.coreConfig, ddSite, apiKey, appKey)
		if err != nil {
			p.logger.Warnf("Failed to create connections API client: %v", err)
		} else {
			tagsProvider := autoconnections.NewTagsProvider(p.tagger)
			creator := autoconnections.NewConnectionsCreator(*client, tagsProvider)

			if err := creator.AutoCreateConnections(ctx, urnParts.RunnerID, enrollmentResult, actionsAllowlist); err != nil {
				p.logger.Warnf("Failed to auto-create connections: %v", err)
			}
		}
	}

	return cfg, nil
}
