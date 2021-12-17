// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator
// +build kubeapiserver,orchestrator

package collectors

import "fmt"

// CollectorInventory is used to store and retrieve available collectors.
type CollectorInventory struct {
	collectors []Collector
}

// NewCollectorInventory returns a new inventory containing all known
// collectors.
func NewCollectorInventory() *CollectorInventory {
	return &CollectorInventory{
		collectors: []Collector{
			newK8sServiceCollector(),
		},
	}
}

// CollectorByName gets a collector given its name. It returns an error if the
// name is not known.
func (ci *CollectorInventory) CollectorByName(collectorName string) (Collector, error) {
	for _, c := range ci.collectors {
		if c.Metadata().Name == collectorName {
			return c, nil
		}
	}
	return nil, fmt.Errorf("no collector found for name %s", collectorName)
}

// StableCollectors get a map of all stable collectors in the inventory indexed
// by name.
func (ci *CollectorInventory) StableCollectors() []Collector {
	var stableCollectors []Collector
	for _, c := range ci.collectors {
		if c.Metadata().IsStable {
			stableCollectors = append(stableCollectors, c)
		}
	}
	return stableCollectors
}
