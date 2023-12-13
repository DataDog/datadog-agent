// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp

package otlp

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/otelcol"

	coreOtlp "github.com/DataDog/datadog-agent/comp/otelcol/otlp"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ServerlessOTLPAgent represents an OTLP agent in a serverless context
type ServerlessOTLPAgent struct {
	pipeline *coreOtlp.Pipeline
}

// NewServerlessOTLPAgent creates a new ServerlessOTLPAgent with the correct
// otel pipeline.
func NewServerlessOTLPAgent(serializer serializer.MetricSerializer) *ServerlessOTLPAgent {
	pipeline, err := coreOtlp.NewPipelineFromAgentConfig(config.Datadog, serializer, nil)
	if err != nil {
		log.Error("Error creating new otlp pipeline:", err)
		return nil
	}
	return &ServerlessOTLPAgent{
		pipeline: pipeline,
	}
}

// Start starts the OTLP agent listening for traces and metrics
func (o *ServerlessOTLPAgent) Start() {
	go func() {
		if err := o.pipeline.Run(context.Background()); err != nil {
			log.Errorf("Error running the OTLP pipeline: %s", err)
		}
	}()
}

// Stop stops the OTLP agent.
func (o *ServerlessOTLPAgent) Stop() {
	if o == nil {
		return
	}
	o.pipeline.Stop()
	if err := o.waitForState(collectorStateClosed, time.Second); err != nil {
		log.Error("Error stopping OTLP endpints:", err)
	}
}

// IsEnabled returns true if the OTLP endpoint should be enabled.
func IsEnabled() bool {
	return coreOtlp.IsEnabled(config.Datadog)
}

var (
	collectorStateRunning = otelcol.StateRunning.String()
	collectorStateClosed  = otelcol.StateClosed.String()
)

// state returns the current state of the underlying otel collector.
func (o *ServerlessOTLPAgent) state() string {
	return o.pipeline.GetCollectorStatus().Status
}

// Wait waits until the OTLP agent is running.
func (o *ServerlessOTLPAgent) Wait(timeout time.Duration) error {
	return o.waitForState(collectorStateRunning, timeout)
}

// waitForState waits until the underlying otel collector is in a given state.
func (o *ServerlessOTLPAgent) waitForState(state string, timeout time.Duration) error {
	after := time.After(timeout)
	for {
		if o.state() == state {
			return nil
		}
		select {
		case <-after:
			return fmt.Errorf("timeout waiting for otlp agent state %s", state)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
