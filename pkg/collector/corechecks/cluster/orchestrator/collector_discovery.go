// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package orchestrator

import (
	"context"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
func (kp *APIServerDiscoveryProvider) Discover(inventory *inventory.CollectorInventory) ([]collectors.Collector, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultAPIServerTimeout)
	defer cancel()

	client, err := apiserver.WaitForAPIClient(ctx)
	if err != nil {
		return nil, err
	}

	preferredResources, err := client.DiscoveryCl.ServerPreferredResources()
	if err != nil {
		return nil, err
	}

	_, allResources, err := client.DiscoveryCl.ServerGroupsAndResources()
	if err != nil {
		return nil, err
	}

	// First pass to enable server-preferred resources
	kp.walkAPIResources(inventory, preferredResources)

	// Second pass to enable non-preferred resources
	kp.walkAPIResources(inventory, allResources)

	return kp.result, nil
}

func (kp *APIServerDiscoveryProvider) addCollector(collector collectors.Collector) {
	if _, found := kp.seen[collector.Metadata().Name]; found {
		return
	}

	kp.result = append(kp.result, collector)
	kp.seen[collector.Metadata().Name] = struct{}{}
	log.Debugf("Discovered collector %s", collector.Metadata().FullName())
}

func (kp *APIServerDiscoveryProvider) walkAPIResources(inventory *inventory.CollectorInventory, apis []*v1.APIResourceList) {
	for _, api := range apis {
		for _, resource := range api.APIResources {
			collector, err := inventory.CollectorForVersion(resource.Name, api.GroupVersion)
			if err != nil {
				continue
			}

			// Enable the cluster collector when the node resource is discovered.
			if collector.Metadata().NodeType == orchestrator.K8sNode {
				clusterCollector, _ := inventory.CollectorForDefaultVersion("clusters")
				kp.addCollector(clusterCollector)
			}

			kp.addCollector(collector)
		}
	}
}
