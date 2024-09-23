// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package service provides a way to interact with os services
package service

import (
	"context"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/repository"
	"github.com/DataDog/datadog-agent/pkg/fleet/internal/cdn"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

const (
	datadogAgent = "datadog-agent"
)

// SetupAgent installs and starts the agent
func SetupAgent(ctx context.Context, args []string) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "setup_agent")
	defer func() {
		if err != nil {
			log.Errorf("Failed to setup agent: %s", err)
		}
		span.Finish(tracer.WithError(err))
	}()
	// Make sure there are no Agent already installed
	_ = removeProduct("Datadog Agent")
	err = msiexec("stable", datadogAgent, "/i", args)
	return err
}

// StartAgentExperiment starts the agent experiment
func StartAgentExperiment(ctx context.Context) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "start_experiment")
	defer func() {
		if err != nil {
			log.Errorf("Failed to start agent experiment: %s", err)
		}
		span.Finish(tracer.WithError(err))
	}()
	err = msiexec("experiment", datadogAgent, "/i", nil)
	return err
}

// StopAgentExperiment stops the agent experiment, i.e. removes/uninstalls it.
func StopAgentExperiment(ctx context.Context) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "stop_experiment")
	defer func() {
		if err != nil {
			log.Errorf("Failed to stop agent experiment: %s", err)
		}
		span.Finish(tracer.WithError(err))
	}()
	err = msiexec("experiment", datadogAgent, "/x", nil)
	if err != nil {
		return err
	}

	// TODO: Need args here to restore DDAGENTUSER
	err = msiexec("stable", datadogAgent, "/i", nil)
	return err
}

// PromoteAgentExperiment promotes the agent experiment
func PromoteAgentExperiment(_ context.Context) error {
	// noop
	return nil
}

// RemoveAgent stops and removes the agent
func RemoveAgent(ctx context.Context) (err error) {
	span, _ := tracer.StartSpanFromContext(ctx, "remove_agent")
	defer func() {
		if err != nil {
			log.Errorf("Failed to remove agent: %s", err)
		}
		span.Finish(tracer.WithError(err))
	}()
	err = removeProduct("Datadog Agent")
	return err
}

// ConfigureAgent noop
func ConfigureAgent(_ context.Context, _ *cdn.CDN, _ *repository.Repositories) error {
	return nil
}
