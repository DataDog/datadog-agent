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

// CollectorsForTest returns the collectors of the check with the full name.
func (o *OrchestratorCheck) CollectorsForTest(fullName string) []collectors.K8sCollector {
	if o.collectorBundle == nil {
		return nil
	}
	var found []collectors.K8sCollector
	for _, collector := range o.collectorBundle.collectors {
		if collector.Metadata().FullName() == fullName {
			found = append(found, collector)
		}
	}
	return found
}

// DatadogAPIGroupForTest is the API group of the built-in Datadog custom resources.
const DatadogAPIGroupForTest = datadogAPIGroup

// CollectorOrderForTest returns the full names of the collectors of the check, in the order in which the check starts
// them.
func (o *OrchestratorCheck) CollectorOrderForTest() []string {
	if o.collectorBundle == nil {
		return nil
	}
	names := make([]string, 0, len(o.collectorBundle.collectors))
	for _, collector := range o.collectorBundle.collectors {
		names = append(names, collector.Metadata().FullName())
	}
	return names
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

// PendingDeletionsForTest returns the number of deleted resources that the check queued and has not sent yet, and
// whether it queues deletions.
func (o *OrchestratorCheck) PendingDeletionsForTest() (pending int, enabled bool) {
	if o.collectorBundle == nil {
		return 0, false
	}
	tb := o.collectorBundle.terminatedResourceBundle
	tb.mu.Lock()
	defer tb.mu.Unlock()
	for _, resources := range tb.terminatedResources {
		pending += len(resources)
	}
	return pending, tb.enabled
}
