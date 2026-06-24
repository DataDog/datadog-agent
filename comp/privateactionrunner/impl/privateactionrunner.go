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
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	rcclient "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/parversion"
	pkgrcclient "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/rcclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/autoconnections"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/enrollment"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/executor"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"go.uber.org/fx"
)

const (
	maxStartupWaitTimeout = 15 * time.Second
)

// isEnabled checks if the private action runner is enabled in the configuration
func isEnabled(cfg config.Component) bool {
	return cfg.GetBool(privateactionrunner.PAREnabled)
}

// Requires defines the dependencies for the privateactionrunner component
type Requires struct {
	Config        config.Component
	Log           log.Component
	Lifecycle     compdef.Lifecycle
	RcClient      rcclient.Component
	Hostname      hostname.Component
	Tagger        tagger.Component
	Traceroute    traceroute.Component
	EventPlatform eventplatform.Component
	IPC           ipc.Component
	Params        *privateactionrunner.Params `optional:"true"`
	Shutdowner    fx.Shutdowner
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
	traceroute     traceroute.Component
	eventPlatform  eventplatform.Component
	ipc            ipc.Component
	params         privateactionrunner.Params
	shutdowner     fx.Shutdowner

	workflowRunner      *runners.WorkflowRunner
	workflowTaskHandler *runners.WorkflowTaskHandler
	commonRunner        *runners.CommonRunner
	taskExecutor        executor.Executor
	executorServer      *executor.Server

	telemetry *telemetry.Telemetry

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
		reqs.Log.Flush()
		return Provides{}, privateactionrunner.ErrNotEnabled
	}

	runner, err := NewPrivateActionRunner(ctx, reqs.Config, reqs.Hostname, pkgrcclient.NewAdapter(reqs.RcClient), reqs.Log, reqs.Tagger, reqs.Traceroute, reqs.EventPlatform, reqs.IPC, reqs.Params, reqs.Shutdowner)
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
	tracerouteComp traceroute.Component,
	eventPlatform eventplatform.Component,
	ipcComp ipc.Component,
	optionalParams ...any,
) (*PrivateActionRunner, error) {
	params := privateactionrunner.Params{Mode: privateactionrunner.ModeOrchestrator}
	var shutdowner fx.Shutdowner
	for _, param := range optionalParams {
		switch v := param.(type) {
		case *privateactionrunner.Params:
			if v != nil {
				params = *v
			}
		case fx.Shutdowner:
			shutdowner = v
		}
	}
	return &PrivateActionRunner{
		coreConfig:     coreConfig,
		hostnameGetter: hostnameGetter,
		rcClient:       rcClient,
		logger:         logger,
		tagger:         taggerComp,
		traceroute:     tracerouteComp,
		eventPlatform:  eventPlatform,
		ipc:            ipcComp,
		params:         params,
		shutdowner:     shutdowner,
		startChan:      make(chan struct{}),
	}, nil
}

func (p *PrivateActionRunner) getRunnerConfig(ctx context.Context) (*parconfig.Config, error) {
	agentIdentifier, err := enrollment.GetAgentIdentifier(ctx, p.hostnameGetter)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent identifier: %w", err)
	}

	persistedIdentity, err := enrollment.GetIdentityFromPreviousEnrollment(ctx, p.coreConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get identity: %w", err)
	}
	if enrollment.ShouldReenroll(agentIdentifier, persistedIdentity) {
		persistedIdentity = nil
	}
	if persistedIdentity != nil {
		p.coreConfig.Set(privateactionrunner.PARPrivateKey, persistedIdentity.PrivateKey, model.SourceAgentRuntime)
		p.coreConfig.Set(privateactionrunner.PARUrn, persistedIdentity.URN, model.SourceAgentRuntime)
	}

	cfg, err := parconfig.FromDDConfig(p.coreConfig)
	if err != nil {
		return nil, err
	}

	canSelfEnroll := p.coreConfig.GetBool(privateactionrunner.PARSelfEnroll)
	if cfg.IdentityIsIncomplete() && canSelfEnroll {
		p.logger.Info("Identity not found and self-enrollment enabled. Self-enrolling private action runner")
		updatedCfg, err := p.performSelfEnrollment(ctx, cfg, agentIdentifier)
		if err != nil {
			p.logger.Errorf("Self-enrollment failed: %v", err)
			return nil, fmt.Errorf("self-enrollment failed: %w", err)
		}
		p.coreConfig.Set(privateactionrunner.PARPrivateKey, updatedCfg.PrivateKey, model.SourceAgentRuntime)
		p.coreConfig.Set(privateactionrunner.PARUrn, updatedCfg.Urn, model.SourceAgentRuntime)
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
	defer p.logger.Flush()
	cfg, err := p.getRunnerConfig(ctx)
	if err != nil {
		p.logger.Errorf("Private action runner failed to start: %v", err)
		return err
	}
	commonTags := observability.CommonTags{
		RunnerId:      cfg.RunnerId,
		RunnerVersion: cfg.Version,
		Modes:         cfg.Modes,
		ExtraTags:     cfg.Tags,
		Component:     string(p.params.Mode),
	}
	ctx = observability.AddCommonTagsToLogs(ctx, commonTags)

	p.telemetry = telemetry.NewTelemetry(
		&http.Client{Transport: httputils.CreateHTTPTransport(p.coreConfig)},
		configutils.SanitizeAPIKey(p.coreConfig.GetString("api_key")),
		cfg.DatadogSite,
		observability.ParService,
	)

	p.logger.Info("Private action runner starting")
	p.logger.Info("==> Version : " + parversion.RunnerVersion)
	p.logger.Info("==> Site : " + cfg.DatadogSite)
	p.logger.Info("==> API Host : " + cfg.DDApiHost)
	p.logger.Info("==> URN : " + cfg.Urn)

	opmsClient := opms.NewClient(cfg)
	socketPath := p.params.ExecutorSocketPath
	if socketPath == "" {
		socketPath = executor.SocketPath(p.coreConfig)
	}
	executorMode := p.coreConfig.GetString(privateactionrunner.PARExecutorMode)
	if p.params.Mode == privateactionrunner.ModeExecutor || executorMode == "" || executorMode == executor.ModeInProcess {
		keysManager := taskverifier.NewKeyManager(p.rcClient)
		taskVerifier := taskverifier.NewTaskVerifier(keysManager, cfg)
		p.workflowTaskHandler, err = runners.NewWorkflowTaskHandler(cfg, keysManager, taskVerifier, opmsClient, p.traceroute, p.eventPlatform, p.ipc.GetClient())
		if err != nil {
			return err
		}
	}

	if p.params.Mode != privateactionrunner.ModeExecutor {
		p.logger.Infof("==> Executor mode : %s", executorMode)
		p.taskExecutor, err = executor.NewExecutor(executor.Params{
			Mode:           executorMode,
			SocketPath:     socketPath,
			ConfPath:       p.params.ConfPath,
			ExtraConfFiles: p.params.ExtraConfFiles,
			Capacity:       cfg.RunnerPoolSize,
			AuthToken:      p.ipc.GetAuthToken(),
			Version:        cfg.Version,
			OnShutdown: executor.LogShutdown(ctx, func() {
				if p.shutdowner != nil {
					_ = p.shutdowner.Shutdown()
				}
			}),
		})
		if err != nil {
			return err
		}
	}

	if p.params.Mode == privateactionrunner.ModeExecutor {
		return p.startExecutor(ctx, cfg, socketPath)
	}
	p.workflowRunner, err = runners.NewWorkflowRunner(cfg, opmsClient, p.taskExecutor)
	if err != nil {
		return err
	}
	p.commonRunner = runners.NewCommonRunner(cfg)
	// Bring the executor up before the orchestrator loop so it is ready by the time
	// the loop starts submitting tasks. Each executor implementation readies its
	// handler as needed (e.g. the in-process one loads verification keys here).
	var taskHandler executor.TaskHandler
	if p.workflowTaskHandler != nil {
		taskHandler = p.workflowTaskHandler
	}
	if err := p.taskExecutor.Start(ctx, taskHandler); err != nil {
		return err
	}
	if err := p.workflowRunner.Start(ctx); err != nil {
		return err
	}
	return p.commonRunner.Start(ctx)
}

func (p *PrivateActionRunner) startExecutor(ctx context.Context, cfg *parconfig.Config, socketPath string) error {
	p.logger.Infof("Starting Private Action Runner executor on %s", socketPath)
	if p.workflowTaskHandler == nil {
		return fmt.Errorf("private action runner executor requires a workflow task handler")
	}
	if err := p.workflowTaskHandler.Prepare(ctx); err != nil {
		return err
	}
	listener, err := executor.Listen(socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on executor socket: %w", err)
	}
	p.executorServer = executor.NewServer(
		p.workflowTaskHandler,
		cfg.Version,
		cfg.ExecutorIdleTimeout,
		p.ipc.GetAuthToken(),
		executor.LogShutdown(ctx, func() {
			if p.shutdowner != nil {
				_ = p.shutdowner.Shutdown()
			}
		}),
	)
	go func() {
		if err := p.executorServer.Serve(ctx, listener); err != nil {
			p.logger.Errorf("Private action runner executor server stopped with error: %v", err)
			if p.shutdowner != nil {
				_ = p.shutdowner.Shutdown()
			}
		}
	}()
	return nil
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
	// taskExecutor (orchestrator side) tears down an in-process server; no-op for
	// binary mode.
	if p.taskExecutor != nil {
		err := p.taskExecutor.Stop(ctx)
		if err != nil {
			return err
		}
	}
	// executorServer is only set when this process is the dedicated executor.
	if p.executorServer != nil {
		err := p.executorServer.Stop(ctx)
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
	if p.telemetry != nil {
		p.telemetry.Stop()
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

// performSelfEnrollment handles the self-registration of a private action runner.
// The enrollment mode is controlled by the api_key_only_enrollment flag:
//   - true:  enroll with API key only (app key ignored, no auto-connections)
//   - false: enroll with API key + app key (app key required, auto-connections created)
func (p *PrivateActionRunner) performSelfEnrollment(ctx context.Context, cfg *parconfig.Config, agentIdentifier *enrollment.AgentIdentifier) (*parconfig.Config, error) {
	ddSite := cfg.DatadogSite
	apiKey := p.coreConfig.GetString("api_key")
	apiKeyOnlyEnrollment := p.coreConfig.GetBool(privateactionrunner.PARApiKeyOnlyEnrollment)

	if apiKeyOK, err := util.ValidateAPIKey(apiKey); err != nil {
		return nil, fmt.Errorf("invalid api_key: %w", err)
	} else if !apiKeyOK {
		p.logger.Warnf("api_key does not match the expected format; enrollment may fail")
	}

	var appKey string
	if apiKeyOnlyEnrollment {
		p.logger.Info("API-key-only enrollment enabled")
	} else {
		appKey = p.coreConfig.GetString("app_key")
		if appKey == "" {
			return nil, errors.New("app_key is required when api_key_only_enrollment is disabled")
		}
		if appKeyOK, err := util.ValidateAppKey(appKey); err != nil {
			return nil, fmt.Errorf("invalid app_key: %w", err)
		} else if !appKeyOK {
			p.logger.Warnf("app_key does not match the expected format; enrollment may fail")
		}
	}

	// For cluster agent, use cluster name instead of hostname for better identification.
	runnerNamePrefix := agentIdentifier.Hostname
	if flavor.GetFlavor() == flavor.ClusterAgent {
		clusterName := clustername.GetClusterName(ctx, agentIdentifier.Hostname)
		if clusterName != "" {
			runnerNamePrefix = clusterName
		} else {
			p.logger.Warnf("Cluster name not found, falling back to hostname '%s' for cluster agent enrollment", agentIdentifier.Hostname)
		}
	}

	var (
		enrollmentResult *enrollment.Result
		err              error
	)
	if apiKeyOnlyEnrollment {
		enrollmentResult, err = enrollment.SelfEnrollApiKeyOnly(ctx, ddSite, runnerNamePrefix, apiKey, agentIdentifier, cfg.OpmsExtraHeaders)
	} else {
		enrollmentResult, err = enrollment.SelfEnroll(ctx, ddSite, runnerNamePrefix, apiKey, appKey, agentIdentifier, cfg.OpmsExtraHeaders)
	}
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

	// Auto-create connections when using app-key enrollment mode and skip_connection_creation is false.
	if !apiKeyOnlyEnrollment && !p.coreConfig.GetBool(privateactionrunner.PARSkipConnectionCreation) {
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
	}

	return cfg, nil
}
