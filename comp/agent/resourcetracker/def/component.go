// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package resourcetracker offers a resource (CPU/RSS/...) tracking component.
// It submits resource usage metrics for use in Fleet Automation.
package resourcetracker

import (
	"context"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

// team: fleet-automation

// Component is the component type.
type Component interface {
}

// Submitter is the interface to submit gauge metrics.
type Submitter interface {
	Gauge(name string, value float64, tags []string) error
}

type telemetrySubmitter struct{}

func (t *telemetrySubmitter) Gauge(name string, value float64, tags []string) error {
	telemetry.GetStatsTelemetryProvider().Gauge(name, value, tags)
	return nil
}

// NewTelemetryGaugeSubmitter returns a new Submitter that submits gauge metrics to the telemetry provider.
func NewTelemetryGaugeSubmitter() Submitter {
	return &telemetrySubmitter{}
}

type apiSubmitter struct {
	api pb.AgentSecureClient
}

func (a *apiSubmitter) Gauge(name string, value float64, tags []string) error {
	_, err := a.api.Gauge(context.Background(), &pb.GaugeRequest{
		Name:  name,
		Value: value,
		Tags:  tags,
	})
	return err
}

// NewAPISubmitter returns a new Submitter that submits gauge metrics to the API.
func NewAPISubmitter(api pb.AgentSecureClient) Submitter {
	return &apiSubmitter{api: api}
}
