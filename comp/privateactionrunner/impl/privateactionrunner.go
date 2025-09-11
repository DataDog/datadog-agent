// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package privateactionrunnerimpl implements the privateactionrunner component interface
package privateactionrunnerimpl

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient"
	parconfig "github.com/DataDog/datadog-agent/pkg/privateactionrunner/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/remoteconfig"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/runners"
	taskverifier "github.com/DataDog/datadog-agent/pkg/privateactionrunner/task-verifier"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/utils"
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
	enabled := reqs.Config.GetBool("privateactionrunner.enabled")
	if !enabled {
		// Return a no-op component when disabled
		runner := &runnerImpl{
			log:    reqs.Logger,
			config: reqs.Config,
		}
		return Provides{
			Comp: runner,
		}, nil
	}

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

	if encodedPrivateKey == "" {
		return nil, fmt.Errorf("private action runner not configured: either run enrollment or provide privateactionrunner.private_key")
	}
	privateKey, err := utils.Base64ToJWK(encodedPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode privateactionrunner.private_key: %w", err)
	}

	if urn == "" {
		return nil, fmt.Errorf("private action runner not configured: URN is required")
	}

	orgId, runnerId, err := parseURN(urn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URN: %w", err)
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
		OrgId:                     orgId,
		PrivateKey:                privateKey.Key.(*ecdsa.PrivateKey),
		RunnerId:                  runnerId,
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

// parseURN parses a URN in the format urn:dd:apps:on-prem-runner:{region}:{org_id}:{runner_id}
// and returns the org_id and runner_id
func parseURN(urn string) (int64, string, error) {
	parts := strings.Split(urn, ":")
	if len(parts) != 7 {
		return 0, "", fmt.Errorf("invalid URN format: expected 6 parts separated by ':', got %d", len(parts))
	}

	if parts[0] != "urn" || parts[1] != "dd" || parts[2] != "apps" || parts[3] != "on-prem-runner" {
		return 0, "", fmt.Errorf("invalid URN format: expected 'urn:dd:apps:on-prem-runner', got '%s:%s:%s:%s'", parts[0], parts[1], parts[2], parts[3])
	}

	orgId, err := strconv.ParseInt(parts[5], 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid org_id in URN: %w", err)
	}

	runnerId := parts[6]
	if runnerId == "" {
		return 0, "", fmt.Errorf("runner_id cannot be empty in URN")
	}

	return orgId, runnerId, nil
}
