// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package discovery

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/inventory"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultAPIServerTimeout = 20 * time.Second
)

// APIServerDiscoveryProvider is a discovery provider that uses the Kubernetes
// API Server as its data source.
type APIServerDiscoveryProvider struct {
	result []collectors.Collector
	seen   map[string]struct{}
}

// NewAPIServerDiscoveryProvider returns a new instance of the APIServer
// discovery provider.
func NewAPIServerDiscoveryProvider() *APIServerDiscoveryProvider {
	return &APIServerDiscoveryProvider{
		seen: make(map[string]struct{}),
	}
}

// Discover returns collectors to enable based on information exposed by the API server.
func (p *APIServerDiscoveryProvider) Discover(inventory *inventory.CollectorInventory) ([]collectors.Collector, error) {
	groups, resources, err := GetServerGroupsAndResources()

	if err != nil {
		return nil, err
	}

	if len(resources) == 0 {
		return nil, fmt.Errorf("failed to discover resources from API groups")
	}

	preferredResources, otherResources := identifyResources(groups, resources)

	// First pass to enable server-preferred resources
	p.walkAPIResources(inventory, preferredResources)

	// Second pass to enable other resources
	p.walkAPIResources(inventory, otherResources)

	return p.result, nil
}

// GetServerGroupsAndResources accesses the api server to retrieve the registered groups and resources
func GetServerGroupsAndResources() ([]*v1.APIGroup, []*v1.APIResourceList, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultAPIServerTimeout)
	defer cancel()

	client, err := apiserver.WaitForAPIClient(ctx)
	if err != nil {
		return nil, nil, err
	}

	groups, resources, err := client.DiscoveryCl.ServerGroupsAndResources()
	if err != nil {
		if !discovery.IsGroupDiscoveryFailedError(err) {
			return nil, nil, err
		}
		// We don't handle API group errors here because we assume API groups used
		// by collectors in the orchestrator check will always be part of the result
		// even though it might be incomplete due to discovery failures on other
		// groups.
		for group, apiGroupErr := range err.(*discovery.ErrGroupDiscoveryFailed).Groups {
			log.Warnf("Resources for API group version %s could not be discovered: %s", group, apiGroupErr)
		}
	}
	return groups, resources, nil
}

func (p *APIServerDiscoveryProvider) addCollector(collector collectors.Collector) {
	// Make sure resource collectors are added at most once
	if _, found := p.seen[collector.Metadata().Name]; found {
		return
	}

	p.result = append(p.result, collector)
	p.seen[collector.Metadata().Name] = struct{}{}
	log.Debugf("Discovered collector %s", collector.Metadata().FullName())
}

func (p *APIServerDiscoveryProvider) walkAPIResources(inventory *inventory.CollectorInventory, resources []*v1.APIResourceList) {
	for _, list := range resources {
		for _, resource := range list.APIResources {
			collector, err := inventory.CollectorForVersion(resource.Name, list.GroupVersion)
			if err != nil {
				continue
			}

			// Ignore unstable collectors.
			if !collector.Metadata().IsStable {
				continue
			}

			// Enable the cluster collector when the node resource is discovered.
			if collector.Metadata().NodeType == orchestrator.K8sNode {
				clusterCollector, _ := inventory.CollectorForDefaultVersion("clusters")
				p.addCollector(clusterCollector)
			}

			p.addCollector(collector)
		}
	}
}

// identifyResources is used to arrange resources into two groups: those that
// belong to preferred API group versions and those that don't.
func identifyResources(groups []*v1.APIGroup, resources []*v1.APIResourceList) (preferred []*v1.APIResourceList, others []*v1.APIResourceList) {
	preferredGroupVersions := make(map[string]struct{})

	// Identify preferred group versions
	for _, group := range groups {
		preferredGroupVersions[group.PreferredVersion.GroupVersion] = struct{}{}
	}

	// Triage resources
	for _, list := range resources {
		if _, found := preferredGroupVersions[list.GroupVersion]; found {
			preferred = append(preferred, list)
		} else {
			others = append(others, list)
		}
	}

	return preferred, others
}
