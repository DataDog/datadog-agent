// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package workloadmetastore defines a resolver that reads data from the global workloadmeta store
package workloadmetastore

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// DefaultResolver represents a default resolver based directly on the underlying tagger
type DefaultResolver struct {
	workloadmetaStore workloadmeta.Store
}

// Resolver represents a cache resolver
type Resolver interface {
	Start(ctx context.Context) error
	Resolve(containerID string) *workloadmeta.Container
}

// Resolve returns the tags for the given id
func (w *DefaultResolver) Resolve(containerID string) *workloadmeta.Container {
	containers := workloadmeta.GetGlobalStore().ListContainersWithFilter(func(container *workloadmeta.Container) bool { return container.ID == containerID })
	if len(containers) == 0 {
		return nil
	}
	if len(containers) > 0 {
		seclog.Errorf("more than 1 container had container ID %s. returning the first one", containerID)
	}
	return containers[0]
}

// Start the resolver
func (w *DefaultResolver) Start(ctx context.Context) error {
	go func() {
		w.workloadmetaStore.Start(ctx) // TODO: Should there be a workloadmeta store stop function?
	}()

	return nil
}

// NewResolver returns a new workloadmeta store resolver
func NewResolver(config *config.Config) Resolver {
	// We only allow the system probe workloadmeta store to be populated via GRPC call to the core agent's workloadmeta store.
	// We do not want to instantiate additional collectors.
	if config.RemoteWorkloadmetaStoreEnabled {
		workloadmeta.CreateGlobalStore(workloadmeta.RemoteCatalog)
	}

	return &DefaultResolver{
		workloadmetaStore: workloadmeta.GetGlobalStore(),
	}
}
