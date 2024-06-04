// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package kubeapiserver implements the kubeapiserver Workloadmeta collector.
package kubeapiserver

import (
	"context"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	collectorID   = "kubeapiserver"
	componentName = "workloadmeta-kubeapiserver"
	noResync      = time.Duration(0)
)

// storeGenerator returns a new store specific to a given resource
type storeGenerator func(context.Context, workloadmeta.Component, kubernetes.Interface) (*cache.Reflector, *reflectorStore)

func storeGenerators(cfg model.Reader) []storeGenerator {
	generators := []storeGenerator{newNodeStore}

	if cfg.GetBool("cluster_agent.collect_kubernetes_tags") || cfg.GetBool("autoscaling.workload.enabled") {
		generators = append(generators, newPodStore)
	}

	if cfg.GetBool("language_detection.enabled") && cfg.GetBool("language_detection.reporting.enabled") {
		generators = append(generators, newDeploymentStore)
	}

	addNamespaceStore := len(cfg.GetStringMapString("kubernetes_namespace_labels_as_tags")) > 0
	// TODO: Remove this once we migrate references to the namespace store to use generic collection
	if cfg.GetBool("cluster_agent.kube_metadata_collection.enabled") {
		resources := cfg.GetStringSlice("cluster_agent.kube_metadata_collection.resources")
		if slices.Contains(resources, "namespaces") {
			addNamespaceStore = true
		}
	}

	if addNamespaceStore {
		generators = append(generators, newNamespaceStore)
	}

	return generators
}

func metadataCollectionGVRs(cfg model.Reader, discoveryClient discovery.DiscoveryInterface) ([]schema.GroupVersionResource, error) {
	if !cfg.GetBool("cluster_agent.kube_metadata_collection.enabled") {
		return []schema.GroupVersionResource{}, nil
	}

	requestedResources := cfg.GetStringSlice("cluster_agent.kube_metadata_collection.resources")

	// TODO: Remove this after implementing collector factory which specifies which collector should be registered for each specific resource type
	// Adding this now as a quick work around to avoid having 2 collectors collecting the same data
	excludedResources := []string{"namespaces"}

	discoveredResourcesGVs, err := discoverGVRs(discoveryClient, requestedResources, excludedResources)
	return discoveredResourcesGVs, err
}

type collector struct {
	id      string
	catalog workloadmeta.AgentType
}

// NewCollector returns a kubeapiserver CollectorProvider that instantiates its colletor
func NewCollector() (workloadmeta.CollectorProvider, error) {
	return workloadmeta.CollectorProvider{
		Collector: &collector{
			id:      collectorID,
			catalog: workloadmeta.ClusterAgent,
		},
	}, nil
}

// GetFxOptions returns the FX framework options for the collector
func GetFxOptions() fx.Option {
	return fx.Provide(NewCollector)
}

func (c *collector) Start(ctx context.Context, wlmetaStore workloadmeta.Component) error {
	var objectStores []*reflectorStore

	apiserverClient, err := apiserver.GetAPIClient()
	if err != nil {
		return err
	}
	client := apiserverClient.InformerCl

	metadataclient, err := apiserverClient.MetadataClient()
	if err != nil {
		return err
	}

	// Initialize metadata collection informers
	// TODO(components): do not use the config.Datadog reference, use a component instead
	gvrs, err := metadataCollectionGVRs(config.Datadog(), client.Discovery())

	if err != nil {
		log.Errorf("failed to discover Group and Version of requested resources: %v", err)
	} else {
		for _, gvr := range gvrs {
			reflector, store := newMetadataStore(ctx, wlmetaStore, metadataclient, gvr)
			objectStores = append(objectStores, store)
			go reflector.Run(ctx.Done())
		}
	}

	// TODO(components): do not use the config.Datadog reference, use a component instead
	for _, storeBuilder := range storeGenerators(config.Datadog()) {
		reflector, store := storeBuilder(ctx, wlmetaStore, client)
		objectStores = append(objectStores, store)
		go reflector.Run(ctx.Done())
	}
	go startReadiness(ctx, objectStores)
	return nil
}

func (c *collector) Pull(_ context.Context) error {
	return nil
}

func (c *collector) GetID() string {
	return c.id
}

func (c *collector) GetTargetCatalog() workloadmeta.AgentType {
	return c.catalog
}

func startReadiness(ctx context.Context, stores []*reflectorStore) {
	log.Infof("Starting readiness waiting for %d k8s reflectors to sync", len(stores))

	// There is no way to ensure liveness correctly as it would need to be plugged inside the
	// inner loop of Reflector.
	// However we add Readiness when we got at least some data.
	health := health.RegisterReadiness(componentName)
	defer func() {
		err := health.Deregister()
		if err != nil {
			log.Criticalf("Unable to deregister component: %s, readiness will likely fail until POD is replaced err: %v", componentName, err)
		}
	}()

	// Checked synced, in its own scope to cleanly unreference the syncTimer
	{
		syncTimer := time.NewTicker(time.Second)
	OUTER:
		for {
			select {
			case <-ctx.Done():
				break OUTER

			case <-syncTimer.C:
				allSynced := true
				for _, store := range stores {
					allSynced = allSynced && store.HasSynced()
				}

				if allSynced {
					break OUTER
				}
			}
		}
		syncTimer.Stop()
	}

	// Once synced, start answering to readiness probe
	log.Infof("All (%d) K8S reflectors synced to workloadmeta", len(stores))
	for {
		select {
		case <-ctx.Done():
			return

		case <-health.C:
		}
	}
}
