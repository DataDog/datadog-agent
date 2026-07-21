// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package privateactionrunnerimpl implements the privateactionrunner component interface
package privateactionrunnerimpl

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	statsdcomp "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	helmactions "github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/def"
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
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/observability"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	statsdclient "github.com/DataDog/datadog-go/v5/statsd"
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
	Statsd        statsdcomp.Component
	HelmActions   helmactions.Component
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
	// metricsClient is the resolved metrics sink: a DogStatsD client built from
	// config (standalone runner) or an in-process adapter (Cluster Agent).
	metricsClient     statsdclient.ClientInterface
	ownsMetricsClient bool
	ha                helmactions.Component

	workflowRunner *runners.WorkflowRunner
	commonRunner   *runners.CommonRunner

	executorServer  *executor.Server
	encryptionStore *encryptioncontext.Store
	executorDone    chan struct{}

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

	// The standalone runner sends metrics over a DogStatsD socket/UDP, built from
	// the Agent's configured endpoint (it runs alongside a node Agent listener).
	metricsClient, err := parconfig.NewMetricsClient(reqs.Config, reqs.Statsd)
	if err != nil {
		reqs.Log.Errorf("Private action runner metrics disabled: %v", err)
	}
	runner, err := NewPrivateActionRunner(ctx, reqs.Config, reqs.Hostname, pkgrcclient.NewAdapter(reqs.RcClient), reqs.Log, reqs.Tagger, reqs.Traceroute, reqs.EventPlatform, reqs.IPC, metricsClient, reqs.HelmActions)
	if err != nil {
		return Provides{}, err
	}
	runner.ownsMetricsClient = true
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: runner.Start,
		OnStop:  runner.Stop,
	})
	return Provides{Comp: runner}, nil
}

// NewExecutorComponent creates a privateactionrunner component in on-demand executor mode.
func NewExecutorComponent(reqs Requires) (Provides, error) {
	ctx := context.Background()
	if !isEnabled(reqs.Config) {
		reqs.Log.Info("private-action-runner is not enabled. Set private_action_runner.enabled: true in your datadog.yaml file or set the environment variable DD_PRIVATE_ACTION_RUNNER_ENABLED=true.")
		reqs.Log.Flush()
		return Provides{}, privateactionrunner.ErrNotEnabled
	}

	metricsClient, err := parconfig.NewMetricsClient(reqs.Config, reqs.Statsd)
	if err != nil {
		reqs.Log.Errorf("Private action runner metrics disabled: %v", err)
	}
	runner, err := NewPrivateActionRunner(ctx, reqs.Config, reqs.Hostname, pkgrcclient.NewAdapter(reqs.RcClient), reqs.Log, reqs.Tagger, reqs.Traceroute, reqs.EventPlatform, reqs.IPC, metricsClient)
	if err != nil {
		return Provides{}, err
	}
	runner.ownsMetricsClient = true
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: runner.StartExecutor,
		OnStop:  runner.StopExecutor,
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
	metricsClient statsdclient.ClientInterface,
	ha helmactions.Component,
) (*PrivateActionRunner, error) {
	return &PrivateActionRunner{
		coreConfig:     coreConfig,
		hostnameGetter: hostnameGetter,
		rcClient:       rcClient,
		logger:         logger,
		tagger:         taggerComp,
		traceroute:     tracerouteComp,
		eventPlatform:  eventPlatform,
		ipc:            ipcComp,
		metricsClient:  metricsClient,
		startChan:      make(chan struct{}),
		ha:             ha,
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

	cfg, err := parconfig.FromDDConfig(p.coreConfig, p.metricsClient)
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

// StartExecutor starts the on-demand executor gRPC server. Idempotent via startOnce.
func (p *PrivateActionRunner) StartExecutor(ctx context.Context) error {
	var err error
	p.started = true
	p.startOnce.Do(func() {
		defer close(p.startChan)
		err = p.startExecutor(ctx)
	})
	return err
}

func (p *PrivateActionRunner) startExecutor(ctx context.Context) error {
	// Detached from ctx's deadline: the server must run until Stop(), not until the fx start timeout.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	p.cancelStart = cancel
	defer p.logger.Flush()

	cfg, err := p.getRunnerConfig(ctx)
	if err != nil {
		p.logger.Errorf("Private action runner executor failed to start: %v", err)
		return err
	}
	commonTags := observability.CommonTags{
		RunnerId:      cfg.RunnerId,
		RunnerVersion: cfg.Version,
		Modes:         cfg.Modes,
		ExtraTags:     cfg.Tags,
	}
	runCtx = observability.AddCommonTagsToLogs(runCtx, commonTags)
	cfg.MetricsClient = observability.NewTaggedMetricsClient(cfg.MetricsClient, commonTags.AsMetricTags())

	p.logger.Info("Private action runner executor starting")
	p.logger.Info("==> Version : " + parversion.RunnerVersion)
	p.logger.Info("==> URN : " + cfg.Urn)

	keysManager := taskverifier.NewKeyManager(p.rcClient)
	taskVerifier := taskverifier.NewTaskVerifier(keysManager, cfg)
	p.encryptionStore = encryptioncontext.NewStore()
	taskExecutor := runners.NewWorkflowTaskExecutor(cfg, taskVerifier, p.traceroute, p.eventPlatform, p.ipc.GetClient(), p.encryptionStore)

	p.executorServer = executor.NewServer(taskExecutor, parversion.RunnerVersion)

	go p.encryptionStore.Start()
	keysManager.Start(runCtx)
	go func() {
		keysManager.WaitForReady()
		p.executorServer.SetReady(true)
		p.logger.Info("Private action runner executor ready to accept actions")
	}()

	socketPath := p.coreConfig.GetString(privateactionrunner.PARExecutorSocketPath)
	lis, err := executor.Listen(socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on executor socket %q: %w", socketPath, err)
	}
	p.logger.Info("Private action runner executor listening on " + socketPath)

	p.executorDone = make(chan struct{})
	// Drain bounded by the task timeout: that's the longest an in-flight action can run.
	drainTimeout := 60 * time.Second
	if cfg.TaskTimeoutSeconds != nil {
		drainTimeout = time.Duration(*cfg.TaskTimeoutSeconds) * time.Second
	}
	serveOpts := executor.ServeOptions{
		DrainTimeout: drainTimeout,
	}
	// mTLS via the agent IPC cert: only a client with a CA-signed cert can dispatch.
	tlsConfig := p.ipc.GetTLSServerConfig()
	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	creds := grpc.Creds(credentials.NewTLS(tlsConfig))

	go func() {
		defer close(p.executorDone)
		if serveErr := executor.Serve(runCtx, lis, p.executorServer, serveOpts, creds); serveErr != nil {
			p.logger.Errorf("Private action runner executor server stopped with error: %v", serveErr)
		}
	}()
	return nil
}

// StopExecutor gracefully stops the executor gRPC server and releases resources.
func (p *PrivateActionRunner) StopExecutor(ctx context.Context) error {
	if !p.started {
		return nil // Never started, nothing to stop
	}

	p.cancelStart()
	waitCtx, cancelWaitCtx := context.WithTimeout(ctx, maxStartupWaitTimeout)
	defer cancelWaitCtx()
	if err := p.waitForStartup(waitCtx); err != nil {
		p.logger.Warn("PAR executor startup did not complete in time, forcing cleanup")
	}

	if p.executorDone != nil {
		select {
		case <-p.executorDone:
			p.logger.Info("Private action runner executor stopped gracefully")
		case <-ctx.Done():
			p.logger.Warn("Private action runner executor did not stop in time")
		}
	}

	var stopErr error
	if p.encryptionStore != nil {
		p.encryptionStore.Stop()
	}
	if p.ownsMetricsClient && p.metricsClient != nil {
		if err := p.metricsClient.Flush(); err != nil {
			stopErr = errors.Join(stopErr, fmt.Errorf("failed to flush metrics client: %w", err))
		}
		if err := p.metricsClient.Close(); err != nil {
			stopErr = errors.Join(stopErr, fmt.Errorf("failed to close metrics client: %w", err))
		}
	}
	return stopErr
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
	}
	ctx = observability.AddCommonTagsToLogs(ctx, commonTags)
	// Stamp runner identity (runner_id, runner_version, modes) on every PAR metric so
	// executions are attributable to the runner that produced them. Done here because
	// runner_id is only finalized after enrollment.
	cfg.MetricsClient = observability.NewTaggedMetricsClient(cfg.MetricsClient, commonTags.AsMetricTags())

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

	keysManager := taskverifier.NewKeyManager(p.rcClient)
	taskVerifier := taskverifier.NewTaskVerifier(keysManager, cfg)
	opmsClient := opms.NewClient(p.coreConfig, cfg)

	p.workflowRunner, err = runners.NewWorkflowRunner(cfg, keysManager, taskVerifier, opmsClient, p.traceroute, p.eventPlatform, p.ipc.GetClient(), p.ha)
	if err != nil {
		return err
	}
	p.commonRunner = runners.NewCommonRunner(p.coreConfig, cfg)
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

	var stopErr error
	if p.workflowRunner != nil {
		err := p.workflowRunner.Stop(ctx)
		if err != nil {
			err = fmt.Errorf("failed to stop workflow runner: %w", err)
		}
		stopErr = errors.Join(stopErr, err)
	}
	if p.commonRunner != nil {
		err := p.commonRunner.Stop(ctx)
		if err != nil {
			err = fmt.Errorf("failed to stop common runner: %w", err)
		}
		stopErr = errors.Join(stopErr, err)
	}
	if p.telemetry != nil {
		p.telemetry.Stop()
	}
	if p.ownsMetricsClient && p.metricsClient != nil {
		if err := p.metricsClient.Flush(); err != nil {
			stopErr = errors.Join(stopErr, fmt.Errorf("failed to flush metrics client: %w", err))
		}
		if err := p.metricsClient.Close(); err != nil {
			stopErr = errors.Join(stopErr, fmt.Errorf("failed to close metrics client: %w", err))
		}
	}
	return stopErr
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

	enrollmentResult, err := enrollment.Enroll(ctx, p.coreConfig, agentIdentifier)
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

	autoconnections.CreateConnectionsIfEnabled(
		ctx, p.coreConfig, cfg, apiKey, appKey, urnParts.RunnerID,
		enrollmentResult, autoconnections.NewTagsProvider(p.tagger),
	)

	return cfg, nil
}
