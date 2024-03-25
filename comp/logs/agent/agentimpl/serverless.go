// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentimpl

import (
	"context"
	flareController "github.com/DataDog/datadog-agent/comp/logs/agent/flare"

	logComponent "github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/logs/agent"
	pkgConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/service"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/logs/tailers"
	"go.uber.org/atomic"
)

// NewServerlessLogsAgent creates a new instance of the logs agent for serverless
func NewServerlessLogsAgent() agent.ServerlessLogsAgent {
	logsAgent := &logAgent{
		log:     logComponent.NewTemporaryLoggerWithoutInit(),
		config:  pkgConfig.Datadog,
		started: atomic.NewBool(false),

		sources:         sources.NewLogSources(),
		services:        service.NewServices(),
		tracker:         tailers.NewTailerTracker(),
		flarecontroller: flareController.NewFlareController(),
	}
	return logsAgent
}

func (a *logAgent) Start() error {
	return a.start(context.TODO())
}

func (a *logAgent) Stop() {
	_ = a.stop(context.TODO())
}

// Flush flushes synchronously the running instance of the Logs Agent.
// Use a WithTimeout context in order to have a flush that can be cancelled.
func (a *logAgent) Flush(ctx context.Context) {
	a.log.Info("Triggering a flush in the logs-agent")
	a.pipelineProvider.Flush(ctx)
	a.log.Debug("Flush in the logs-agent done.")
}
