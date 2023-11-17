// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

// Package kubeapiserver implements the kubeapiserver Workloadmeta collector.
package kubeapiserver

import (
	"context"
	"time"

	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/status/health"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"k8s.io/client-go/kubernetes"
)

const (
	collectorID   = "kubeapiserver"
	componentName = "workloadmeta-kubeapiserver"
	noResync      = time.Duration(0)
)

// storeGenerator returns a new store specific to a given resource
type storeGenerator func(context.Context, workloadmeta.Store, kubernetes.Interface) (*cache.Reflector, *reflectorStore)

func storeGenerators(cfg config.ConfigReader) []storeGenerator {
	generators := []storeGenerator{newNodeStore}

	if cfg.GetBool("cluster_agent.collect_kubernetes_tags") {
		generators = append(generators, newPodStore)
	}

	if cfg.GetBool("language_detection.enabled") {
		generators = append(generators, newDeploymentStore)
	}

	return generators
}

type collector struct {
	id string
}

func NewCollector() collectors.CollectorProvider {
	return collectors.CollectorProvider{
		Collector: &collector{
			id: collectorID,
		},
	}
}

func (c *collector) Start(ctx context.Context, wlmetaStore workloadmeta.Component) error {
	var objectStores []*reflectorStore

	apiserverClient, err := apiserver.GetAPIClient()
	if err != nil {
		return err
	}
	client := apiserverClient.Cl

	// TODO(components): do not use the config.Datadog reference, use a component instead
	for _, storeBuilder := range storeGenerators(config.Datadog) {
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

func startReadiness(ctx context.Context, stores []*reflectorStore) {
	log.Infof("Starting readiness waiting for %d k8s reflectors to sync", len(stores))

	// There is no way to ensure liveness correctly as it would need to be plugged inside the
	// inner loop of Reflector.
	// However we add Readiness when we got at least some data.
	health := health.RegisterReadiness(componentName)
	defer func() {
		err := health.Deregister()
		log.Criticalf("Unable to deregister component: %s, readiness will likely fail until POD is replaced err: %v", componentName, err)
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
