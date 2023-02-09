// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp
// +build otlp

package otlp

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/otelcol"

	"github.com/DataDog/datadog-agent/pkg/config"
	coreOtlp "github.com/DataDog/datadog-agent/pkg/otlp"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type ServerlessOTLPAgent struct {
	pipeline *coreOtlp.Pipeline
}

func NewServerlessOTLPAgent(serializer serializer.MetricSerializer) *ServerlessOTLPAgent {
	pipeline, err := coreOtlp.NewPipelineFromAgentConfig(config.Datadog, serializer)
	if err != nil {
		log.Error("Error creating new otlp pipeline:", err)
		return nil
	}
	return &ServerlessOTLPAgent{
		pipeline: pipeline,
	}
}

func (o *ServerlessOTLPAgent) Start() {
	go o.pipeline.Run(context.Background())
}

func (o *ServerlessOTLPAgent) Stop() {
	if o == nil {
		return
	}
	o.pipeline.Stop()
	if err := o.waitForState(collectorStateClosed, time.Second); err != nil {
		log.Error("Error stopping OTLP endpints:", err)
	}
}

func IsEnabled() bool {
	return coreOtlp.IsEnabled(config.Datadog)
}

var (
	collectorStateRunning = otelcol.StateRunning.String()
	collectorStateClosed  = otelcol.StateClosed.String()
)

func (o *ServerlessOTLPAgent) state() string {
	return coreOtlp.GetCollectorStatus(o.pipeline).Status
}

func (o *ServerlessOTLPAgent) Wait(timeout time.Duration) error {
	return o.waitForState(collectorStateRunning, timeout)
}

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
