// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package privateactionrunnerimpl implements the privateactionrunner component interface
package privateactionrunnerimpl

import (
	"context"
	"crypto/ecdsa"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/helpers"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/remoteconfig"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-go/v5/statsd"
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
	log            log.Component
	config         config.Component
	started        bool
	keysManager    remoteconfig.KeysManager
	TaskVerifier   *taskverifier.TaskVerifier
	WorkflowRunner *runners.WorkflowRunner
}

// NewComponent creates a new privateactionrunner component
func NewComponent(reqs Requires) (Provides, error) {

	//ctx := context.Background()
	cfg, err := getParConfig(reqs.Config)
	if err != nil {
		return Provides{}, err
	}
	opmsClient := opms.NewClient(cfg)
	keysManager := remoteconfig.New(reqs.RcClient)
	verifier := taskverifier.NewTaskVerifier(keysManager, cfg)

	runner := &runnerImpl{
		log:            reqs.Logger,
		config:         reqs.Config,
		keysManager:    keysManager,
		TaskVerifier:   verifier,
		WorkflowRunner: runners.NewWorkflowRunner(cfg, keysManager, verifier, opmsClient),
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: runner.Start,
		OnStop:  runner.Stop,
	})
	return Provides{
		Comp: runner,
	}, nil
}

const (
	maxBackoff                   = 3 * time.Minute
	minBackoff                   = 1 * time.Second
	maxAttempts                  = 20
	waitBeforeRetry              = 5 * time.Minute
	loopInterval                 = 1 * time.Second
	opmsRequestTimeout           = 30_000
	runnerPoolSize               = 1
	defaultHealthCheckEndpoint   = "/healthz"
	healthCheckInterval          = 30_000
	defaultHttpServerReadTimeout = 10_000
	// defaultHttpServerWriteTimeout defines how long a request is allowed to run for after the HTTP connection is established. If actions are timing out often, `httpServerWriteTimeout` can be adjusted in config.yaml to override this value. See the Golang docs under `WriteTimeout` for more information about how the server uses this value - https://pkg.go.dev/net/http#Server
	defaultHttpServerWriteTimeout = 60_000
	runnerAccessTokenHeader       = "X-Datadog-Apps-On-Prem-Runner-Access-Token"
	runnerAccessTokenIdHeader     = "X-Datadog-Apps-On-Prem-Runner-Access-Token-ID"
	defaultPort                   = 9016
	defaultJwtRefreshInterval     = 15 * time.Second
	heartbeatInterval             = 20 * time.Second
)

func getParConfig(component config.Component) (*parconfig.Config, error) {
	ddSite := component.GetString("site")
	encodedPrivateKey := component.GetString("privateactionrunner.private_key")
	urn := component.GetString("privateactionrunner.urn")

	privateKey, err := helpers.Base64ToJWK(encodedPrivateKey)
	if err != nil {
		return nil, err
	}

	return &parconfig.Config{
		MaxBackoff:                maxBackoff,
		MinBackoff:                minBackoff,
		MaxAttempts:               maxAttempts,
		WaitBeforeRetry:           waitBeforeRetry,
		LoopInterval:              loopInterval,
		OpmsRequestTimeout:        opmsRequestTimeout,
		RunnerPoolSize:            runnerPoolSize,
		HealthCheckInterval:       healthCheckInterval,
		HttpServerReadTimeout:     defaultHttpServerReadTimeout,
		HttpServerWriteTimeout:    defaultHttpServerWriteTimeout,
		RunnerAccessTokenHeader:   runnerAccessTokenHeader,
		RunnerAccessTokenIdHeader: runnerAccessTokenIdHeader,
		Port:                      defaultPort,
		JWTRefreshInterval:        defaultJwtRefreshInterval,
		HealthCheckEndpoint:       defaultHealthCheckEndpoint,
		HeartbeatInterval:         heartbeatInterval,
		Version:                   "1.0.0-agent",
		MetricsClient:             &statsd.NoOpClient{},
		ActionsAllowlist:          make(map[string][]string),
		Allowlist:                 strings.Split(component.GetString("privateactionrunner.allowlist"), ","),
		AllowIMDSEndpoint:         component.GetBool("privateactionrunner.allow_imds_endpoint"),
		DDHost:                    strings.Join([]string{"api", ddSite}, "."),
		Modes:                     strings.Split(component.GetString("privateactionrunner.modes"), ","),
		OrgId:                     component.GetInt64("privateactionrunner.org_id"),
		PrivateKey:                privateKey.Key.(*ecdsa.PrivateKey),
		RunnerId:                  component.GetString("privateactionrunner.runner_id"),
		Urn:                       urn,
		DatadogSite:               ddSite,
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
	r.WorkflowRunner.Start(ctx)
	return nil
}

func (r *runnerImpl) Stop(ctx context.Context) error {
	if !r.started {
		return nil
	}
	r.log.Info("Stopping private action runner")
	r.WorkflowRunner.Close(ctx)
	return nil
}
