// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

package start

import (
	"context"
	"os"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	logshttp "github.com/DataDog/datadog-agent/pkg/logs/client/http"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

const (
	intakeTrackType = "compliance"
)

func runCompliance(ctx context.Context, senderManager sender.SenderManager, wmeta workloadmeta.Component, apiCl *apiserver.APIClient, isLeader func() bool) error {
	stopper := startstop.NewSerialStopper()
	if err := startCompliance(senderManager, wmeta, stopper, apiCl, isLeader); err != nil {
		return err
	}

	<-ctx.Done()

	stopper.Stop()
	return nil
}

func newLogContext(logsConfig *config.LogsConfigKeys, endpointPrefix string) (*config.Endpoints, *client.DestinationsContext, error) {
	endpoints, err := config.BuildHTTPEndpointsWithConfig(coreconfig.Datadog, logsConfig, endpointPrefix, intakeTrackType, config.AgentJSONIntakeProtocol, config.DefaultIntakeOrigin)
	if err != nil {
		endpoints, err = config.BuildHTTPEndpoints(coreconfig.Datadog, intakeTrackType, config.AgentJSONIntakeProtocol, config.DefaultIntakeOrigin)
		if err == nil {
			httpConnectivity := logshttp.CheckConnectivity(endpoints.Main, coreconfig.Datadog)
			endpoints, err = config.BuildEndpoints(coreconfig.Datadog, httpConnectivity, intakeTrackType, config.AgentJSONIntakeProtocol, config.DefaultIntakeOrigin)
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

func startCompliance(senderManager sender.SenderManager, wmeta workloadmeta.Component, stopper startstop.Stopper, apiCl *apiserver.APIClient, isLeader func() bool) error {
	endpoints, ctx, err := newLogContextCompliance()
	if err != nil {
		log.Error(err)
	}
	stopper.Add(ctx)

	runPath := coreconfig.Datadog.GetString("compliance_config.run_path")
	configDir := coreconfig.Datadog.GetString("compliance_config.dir")
	checkInterval := coreconfig.Datadog.GetDuration("compliance_config.check_interval")

	hname, err := hostname.Get(context.TODO())
	if err != nil {
		return err
	}

	reporter := compliance.NewLogReporter(hname, "compliance-agent", "compliance", runPath, endpoints, ctx)
	agent := compliance.NewAgent(senderManager, wmeta, compliance.AgentOptions{
		ConfigDir:     configDir,
		Reporter:      reporter,
		CheckInterval: checkInterval,
		RuleFilter: func(rule *compliance.Rule) bool {
			return rule.HasScope(compliance.KubernetesClusterScope)
		},
		ResolverOptions: compliance.ResolverOptions{
			Hostname:           hname,
			HostRoot:           os.Getenv("HOST_ROOT"),
			DockerProvider:     compliance.DefaultDockerProvider,
			LinuxAuditProvider: compliance.DefaultLinuxAuditProvider,
			KubernetesProvider: wrapKubernetesClient(apiCl, isLeader),
		},
	})
	err = agent.Start()
	if err != nil {
		return log.Errorf("Error starting compliance agent, exiting: %v", err)
	}
	stopper.Add(agent)

	log.Infof("Running compliance checks every %s", checkInterval.String())
	return nil
}

func wrapKubernetesClient(apiCl *apiserver.APIClient, isLeader func() bool) compliance.KubernetesProvider {
	return func(ctx context.Context) (dynamic.Interface, discovery.DiscoveryInterface, error) {
		if isLeader() {
			return apiCl.DynamicCl, apiCl.Cl.Discovery(), nil
		}
		return nil, nil, compliance.ErrIncompatibleEnvironment
	}
}
