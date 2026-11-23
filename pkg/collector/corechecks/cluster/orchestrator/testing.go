// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package orchestrator

import (
	"context"

	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/discovery"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// TestDependencies replace the services that the check reaches outside the process.
type TestDependencies struct {
	WaitForAPIClient         func(ctx context.Context) (*apiserver.APIClient, error)
	RunLeaderElection        func() (leader string, err error)
	ServerGroupsAndResources discovery.ServerGroupsAndResourcesFunc
	// IsCLCRunner runs the check as a cluster check runner.
	IsCLCRunner bool
}

// NewCheckForTest returns an unconfigured check that uses the test dependencies.
func NewCheckForTest(cfg configcomp.Component, store workloadmeta.Component, tagger tagger.Component, deps TestDependencies) *OrchestratorCheck {
	chk := newOrchestratorCheck(core.NewCheckBase(CheckName), &OrchestratorInstance{}, cfg, store, tagger)
	chk.isCLCRunner = deps.IsCLCRunner
	chk.deps = checkDependencies{
		waitForAPIClient:         deps.WaitForAPIClient,
		runLeaderElection:        deps.RunLeaderElection,
		serverGroupsAndResources: deps.ServerGroupsAndResources,
	}
	return chk
}

// CollectorsForTest returns the collectors with the full name in every bundle of the check.
func (o *OrchestratorCheck) CollectorsForTest(fullName string) []collectors.K8sCollector {
	var found []collectors.K8sCollector
	for _, bundle := range o.bundles() {
		for _, collector := range bundle.collectors {
			if collector.Metadata().FullName() == fullName {
				found = append(found, collector)
			}
		}
	}
	return found
}
