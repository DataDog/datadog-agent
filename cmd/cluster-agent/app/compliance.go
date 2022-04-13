// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package app

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/collector/runner"
	"github.com/DataDog/datadog-agent/pkg/collector/scheduler"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/compliance/agent"
	"github.com/DataDog/datadog-agent/pkg/compliance/checks"
	"github.com/DataDog/datadog-agent/pkg/compliance/event"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	logshttp "github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

const (
	intakeTrackType = "compliance"
)

func runCompliance(ctx context.Context, apiCl *apiserver.APIClient, isLeader func() bool) error {
	stopper := startstop.NewSerialStopper()
	if err := startCompliance(stopper, apiCl, isLeader); err != nil {
		return err
	}

	<-ctx.Done()

	stopper.Stop()
	return nil
}

func newLogContext(logsConfig *config.LogsConfigKeys, endpointPrefix string) (*config.Endpoints, *client.DestinationsContext, error) {
	endpoints, err := config.BuildHTTPEndpointsWithConfig(logsConfig, endpointPrefix, intakeTrackType, logs.AgentJSONIntakeProtocol, config.DefaultIntakeOrigin)
	if err != nil {
		endpoints, err = config.BuildHTTPEndpoints(intakeTrackType, logs.AgentJSONIntakeProtocol, config.DefaultIntakeOrigin)
		if err == nil {
			httpConnectivity := logshttp.CheckConnectivity(endpoints.Main)
			endpoints, err = config.BuildEndpoints(httpConnectivity, intakeTrackType, logs.AgentJSONIntakeProtocol, config.DefaultIntakeOrigin)
		}
	}

	if err != nil {
		return nil, nil, log.Errorf("Invalid endpoints: %v", err)
	}

	for _, status := range endpoints.GetStatus() {
		log.Info(status)
	}

	destinationsCtx := client.NewDestinationsContext()
	destinationsCtx.Start()

	return endpoints, destinationsCtx, nil
}

func newLogContextCompliance() (*config.Endpoints, *client.DestinationsContext, error) {
	logsConfigComplianceKeys := config.NewLogsConfigKeys("compliance_config.endpoints.", coreconfig.Datadog)
	return newLogContext(logsConfigComplianceKeys, "cspm-intake.")
}

func startCompliance(stopper startstop.Stopper, apiCl *apiserver.APIClient, isLeader func() bool) error {
	endpoints, ctx, err := newLogContextCompliance()
	if err != nil {
		log.Error(err)
	}
	stopper.Add(ctx)

	runPath := coreconfig.Datadog.GetString("compliance_config.run_path")
	reporter, err := event.NewLogReporter(stopper, "compliance-agent", "compliance", runPath, endpoints, ctx)
	if err != nil {
		return err
	}

	runner := runner.NewRunner()
	stopper.Add(runner)

	scheduler := scheduler.NewScheduler(runner.GetChan())
	runner.SetScheduler(scheduler)

	checkInterval := coreconfig.Datadog.GetDuration("compliance_config.check_interval")
	checkMaxEvents := coreconfig.Datadog.GetInt("compliance_config.check_max_events_per_run")
	configDir := coreconfig.Datadog.GetString("compliance_config.dir")

	hostname, err := util.GetHostname(context.TODO())
	if err != nil {
		return err
	}
	agent, err := agent.New(
		reporter,
		scheduler,
		configDir,
		endpoints,
		checks.WithInterval(checkInterval),
		checks.WithMaxEvents(checkMaxEvents),
		checks.WithHostname(hostname),
		checks.WithMatchRule(func(rule *compliance.RuleCommon) bool {
			return rule.Scope.Includes(compliance.KubernetesClusterScope)
		}),
		checks.WithKubernetesClient(apiCl.DynamicCl, ""),
		checks.WithIsLeader(isLeader),
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
