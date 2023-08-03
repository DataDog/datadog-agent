// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import "context"

// Collector is responsible for collecting metadata about workloads.
type Collector interface {
	// Start starts a collector. The collector should run until the context
	// is done. It also gets a reference to the store that started it so it
	// can use Notify, or get access to other entities in the store.
	Start(context.Context, Component) error

	// Pull triggers an entity collection. To be used by collectors that
	// don't have streaming functionality, and called periodically by the
	// store.
	Pull(context.Context) error

	// Returns the identifier for the respective component.
	GetID() string
}

type CollectorList []Collector

// TODO: Factory Catalogs will be removed in the component implementation.
//
// var (
// 	// NodeAgentCatalog is a catalog of collectors that runs in the node
// 	// agents
// 	NodeAgentCatalog = make(CollectorCatalog)
//
// 	// ClusterAgentCatalog is a catalog of collectors that runs in the
// 	// cluster agent, and the cluster checks runner agents
// 	ClusterAgentCatalog = make(CollectorCatalog)
//
// 	// RemoteCatalog collectors to run when workloadmeta is configured as remote
// 	RemoteCatalog = make(CollectorCatalog)
// )
