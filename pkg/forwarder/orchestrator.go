// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

// +build kubeapiserver,orchestrator,kubelet

package forwarder

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	orchcfg "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	apicfg "github.com/DataDog/datadog-agent/pkg/process/util/api/config"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewOrchestratorForwarder returns an orchestratorForwarder
// if the feature is activated on the cluster-agent/cluster-check runner, nil otherwise
func NewOrchestratorForwarder() *DefaultForwarder {
	if !config.Datadog.GetBool("orchestrator_explorer.enabled") {
		return nil
	}
	if flavor.GetFlavor() == flavor.DefaultAgent && !config.IsCLCRunner() {
		return nil
	}
	orchestratorCfg := orchcfg.NewDefaultOrchestratorConfig()
	if err := orchestratorCfg.Load(); err != nil {
		log.Errorf("Error loading the orchestrator config: %s", err)
	}
	keysPerDomain := apicfg.KeysPerDomains(orchestratorCfg.OrchestratorEndpoints)
	orchestratorForwarderOpts := NewOptions(keysPerDomain)
	orchestratorForwarderOpts.DisableAPIKeyChecking = true

	return NewDefaultForwarder(orchestratorForwarderOpts)
}
