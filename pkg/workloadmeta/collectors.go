// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import "context"

// Collector is responsible for collecting metadata about workloads.
type Collector interface {
	Pull(context.Context) error
	Start(context.Context, *Store) error
}

type collectorFactory func() Collector

var collectorCatalog = make(map[string]collectorFactory)

// RegisterCollector registers a new collector to be used by the store.
func RegisterCollector(id string, c collectorFactory) {
	collectorCatalog[id] = c
}
