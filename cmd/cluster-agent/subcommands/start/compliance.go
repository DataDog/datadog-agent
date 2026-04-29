// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

package start

import (
	"context"
	"os"

	"k8s.io/client-go/dynamic"

	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	seccommon "github.com/DataDog/datadog-agent/pkg/security/common"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/startstop"
)

func runCompliance(ctx context.Context, senderManager sender.SenderManager, wmeta workloadmeta.Component, filterStore workloadfilter.Component, apiCl *apiserver.APIClient, compression logscompression.Component, isLeader func() bool, secretsComp secrets.Component) error {
	stopper := startstop.NewSerialStopper()
	if err := startCompliance(ctx, senderManager, wmeta, filterStore, stopper, apiCl, isLeader, compression, secretsComp); err != nil {
		return err
	}

	<-ctx.Done()

	stopper.Stop()
	return nil
}

func startCompliance(ctx context.Context, senderManager sender.SenderManager, wmeta workloadmeta.Component, filterStore workloadfilter.Component, stopper startstop.Stopper, apiCl *apiserver.APIClient, isLeader func() bool, compression logscompression.Component, secretsComp secrets.Component) error {
	endpoints, destinationsCtx, err := seccommon.NewLogContextCompliance()
	if err != nil {
		log.Error(err)
	}
	stopper.Add(destinationsCtx)

	configDir := pkgconfigsetup.Datadog().GetString("compliance_config.dir")
	checkInterval := pkgconfigsetup.Datadog().GetDuration("compliance_config.check_interval")

	hname, err := hostname.Get(ctx)
	if err != nil {
		return err
	}

	reporter := compliance.NewLogReporter(hname, "compliance-agent", "compliance", endpoints, destinationsCtx, compression, secretsComp)
	statsdClient, err := simpleTelemetrySenderFromSenderManager(senderManager)
	if err != nil {
		return err
	}

	reflectorStore := compliance.NewReflectorStore(apiCl.Cl)
	reflectorStore.Run(ctx.Done())

	agent := compliance.NewAgent(statsdClient, wmeta, filterStore, hname, compliance.AgentOptions{
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
			ReflectorStore:     reflectorStore,
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
	return func(_ context.Context) (dynamic.Interface, compliance.KubernetesGroupsAndResourcesProvider, error) {
		if isLeader() {
			discoveryCl := apiCl.Cl.Discovery()
			return apiCl.DynamicCl, discoveryCl.ServerGroupsAndResources, nil
		}
		return nil, nil, compliance.ErrIncompatibleEnvironment
	}
}
