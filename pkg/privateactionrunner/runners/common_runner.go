// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package runners

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/opms"
)

type CommonRunner struct {
	opmsClient opms.Client
	config     *config.Config
}

func NewCommonRunner(
	configuration *config.Config,
) *CommonRunner {
	return &CommonRunner{
		opmsClient: opms.NewClient(configuration),
		config:     configuration,
	}
}

func (n *CommonRunner) Start(ctx context.Context) error {
	log.FromContext(ctx).Info("Starting Common runner")
	go n.healthCheckLoop(ctx)
	return nil
}

func (n *CommonRunner) Stop(ctx context.Context) error {
	log.FromContext(ctx).Info("Stopping Common runner")
	// Common runner does not have any resource to clean up
	return nil
}

func (n *CommonRunner) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Millisecond * time.Duration(n.config.HealthCheckInterval))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logger := log.FromContext(ctx)
			healthResponse, err := n.opmsClient.HealthCheck(ctx)
			if healthResponse != nil && healthResponse.ServerTime != nil {
				logger = logger.With(log.String("server-time", healthResponse.ServerTime.UTC().Format(time.RFC3339)))
			}
			if err != nil {
				logger.Error("health check failed", log.ErrorField(err))
			} else {
				logger.Info("health check succeeded")
			}
		}
	}
}
