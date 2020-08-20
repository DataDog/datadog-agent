// +build linux

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package app

import (
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func newRuntimeReporter(stopper restart.Stopper, sourceName, sourceType string, endpoints *config.Endpoints, context *client.DestinationsContext) (event.Reporter, error) {
	health := health.RegisterLiveness("runtime-security")

	// setup the auditor
	auditor := auditor.New(coreconfig.Datadog.GetString("runtime_security_config.run_path"), "runtime-security-registry.json", health)
	auditor.Start()
	stopper.Add(auditor)

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(config.NumberOfPipelines, auditor, nil, endpoints, context)
	pipelineProvider.Start()
	stopper.Add(pipelineProvider)

	logSource := config.NewLogSource(
		sourceName,
		&config.LogsConfig{
			Type:    sourceType,
			Service: sourceName,
			Source:  sourceName,
		},
	)
	return event.NewReporter(logSource, pipelineProvider.NextPipelineChan()), nil
}

func startRuntimeSecurity(hostname string, endpoints *config.Endpoints, context *client.DestinationsContext, stopper restart.Stopper) (*secagent.RuntimeSecurityAgent, error) {
	enabled := coreconfig.Datadog.GetBool("runtime_security_config.enabled")
	if !enabled {
		log.Info("Datadog runtime security agent disabled by config")
		return nil, nil
	}

	reporter, err := newRuntimeReporter(stopper, "runtime-security-agent", "runtime-security", endpoints, context)
	if err != nil {
		return nil, err
	}

	agent, err := secagent.NewRuntimeSecurityAgent(hostname, reporter)
	if err != nil {
		return nil, errors.Wrap(err, "unable to create a runtime security agent instance")
	}
	agent.Start()

	stopper.Add(agent)

	log.Info("Datadog runtime security agent is now running")

	return agent, nil
}
