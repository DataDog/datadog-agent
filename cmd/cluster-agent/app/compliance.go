// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package app

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/agent"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/auditor"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/restart"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func runCompliance(ctx context.Context) error {
	apiCl, err := apiserver.WaitForAPIClient(ctx)
	if err != nil {
		return err
	}

	stopper := restart.NewSerialStopper()
	if err := startCompliance(stopper, apiCl); err != nil {
		return err
	}

	<-ctx.Done()

	stopper.Stop()
	return nil
}

// TODO: Factorize code with pkg/compliance
func startCompliance(stopper restart.Stopper, apiCl *apiserver.APIClient) error {
	httpConnectivity := config.HTTPConnectivityFailure
	if endpoints, err := config.BuildHTTPEndpoints(); err == nil {
		httpConnectivity = http.CheckConnectivity(endpoints.Main)
	}

	endpoints, err := config.BuildEndpoints(httpConnectivity)
	if err != nil {
		return log.Errorf("Invalid endpoints: %v", err)
	}

	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()
	stopper.Add(destinationsCtx)

	health := health.RegisterLiveness("compliance")

	// setup the auditor
	auditor := auditor.New(coreconfig.Datadog.GetString("compliance_config.run_path"), health)
	auditor.Start()
	stopper.Add(auditor)

	// setup the pipeline provider that provides pairs of processor and sender
	pipelineProvider := pipeline.NewProvider(config.NumberOfPipelines, auditor, nil, endpoints, destinationsCtx)
	pipelineProvider.Start()
	stopper.Add(pipelineProvider)

	logSource := config.NewLogSource("compliance-agent", &config.LogsConfig{
		Type:    "compliance",
		Service: "compliance-agent",
		Source:  "compliance-agent",
	})

	reporter := compliance.NewReporter(logSource, pipelineProvider.NextPipelineChan())

	runner := runner.NewRunner()
	stopper.Add(runner)

	scheduler := scheduler.NewScheduler(runner.GetChan())
	runner.SetScheduler(scheduler)

	checkInterval := coreconfig.Datadog.GetDuration("compliance_config.check_interval")
	configDir := coreconfig.Datadog.GetString("compliance_config.dir")

	hostname, err := util.GetHostname()
	if err != nil {
		return err
	}
	agent, err := agent.New(
		reporter,
		scheduler,
		configDir,
		checks.WithInterval(checkInterval),
		checks.WithHostname(hostname),
		checks.WithMatchRule(func(rule *compliance.Rule) bool {
			return rule.Scope.KubernetesCluster
		}),
		checks.WithKubernetesClient(apiCl.DynamicCl),
	)
	if err != nil {
		return err
	}
	err = agent.Run()
	if err != nil {
		return log.Errorf("Error starting compliance agent, exiting: %v", err)
	}
	stopper.Add(agent)

	log.Infof("Running compliance checks every %s", checkInterval.String())
	return nil
}
