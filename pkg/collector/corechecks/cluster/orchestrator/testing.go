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
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/inventory"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/discovery"
	utilTypes "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/util"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// TestDependencies replace the services that the check reaches outside the process.
type TestDependencies struct {
	WaitForAPIClient         func(ctx context.Context) (*apiserver.APIClient, error)
	RunLeaderElection        func() (leader string, err error)
	ServerGroupsAndResources discovery.ServerGroupsAndResourcesFunc
	// IsCLCRunner runs the check as a cluster check runner.
	IsCLCRunner bool
	// OnDeletion is called after the check handles the deletion of an object, queued or not.
	OnDeletion func(obj interface{})
}

// NewCheckForTest returns an unconfigured check that uses the test dependencies. It panics when a service is missing,
// so that a test cannot reach the API server of the host kubeconfig.
func NewCheckForTest(cfg configcomp.Component, store workloadmeta.Component, tagger tagger.Component, deps TestDependencies) *OrchestratorCheck {
	if deps.WaitForAPIClient == nil || deps.RunLeaderElection == nil || deps.ServerGroupsAndResources == nil {
		panic("the test dependencies miss WaitForAPIClient, RunLeaderElection or ServerGroupsAndResources")
	}
	chk := newOrchestratorCheck(core.NewCheckBase(CheckName), &OrchestratorInstance{}, cfg, store, tagger)
	chk.isCLCRunner = deps.IsCLCRunner
	chk.deps = checkDependencies{
		waitForAPIClient:         deps.WaitForAPIClient,
		runLeaderElection:        deps.RunLeaderElection,
		serverGroupsAndResources: deps.ServerGroupsAndResources,
		onDeletion:               deps.OnDeletion,
	}
	return chk
}

// WatchesPodDeletionsForTest reports whether the check initialized the terminated pod collector, whose pod deletion
// watcher lists and then watches the pods in the background.
func (o *OrchestratorCheck) WatchesPodDeletionsForTest() bool {
	if o.collectorBundle == nil {
		return false
	}
	for _, collector := range o.collectorBundle.collectors {
		if collector.Metadata().Name != utilTypes.TerminatedPodName {
			continue
		}
		if _, found := o.collectorBundle.initializedCollectors[collector.Metadata().FullName()]; found {
			return true
		}
	}
	return false
}

// CollectorMetadataForTest returns the metadata of each version of each collector of the inventory.
func CollectorMetadataForTest(cfg configcomp.Component, store workloadmeta.Component, tagger tagger.Component) []*collectors.CollectorMetadata {
	var metadata []*collectors.CollectorMetadata
	for _, collector := range inventory.NewCollectorInventory(cfg, store, tagger).CollectorsForTest() {
		metadata = append(metadata, collector.Metadata())
	}
	return metadata
}

// BuiltinCustomResourcesForTest returns the full name of each version of each built-in custom resource. A group-wide
// custom resource has no resource name.
func BuiltinCustomResourcesForTest() []string {
	var names []string
	for _, cr := range newBuiltinCRDConfigs() {
		for _, version := range append([]string{cr.preferredVersion}, cr.fallbackVersions...) {
			names = append(names, cr.group+"/"+version+"/"+cr.kind)
		}
	}
	return names
}
