// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-Present Datadog, Inc.

package collectors

import (
	"github.com/DataDog/datadog-agent/pkg/util/cloudfoundry"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

const (
	cfCollectorName = "cloudfoundry"
)

// CFCollector lists containers from local rep, and fetch performance metrics from garden
type CFCollector struct {
	gardenUtil *cloudfoundry.GardenUtil
}

// Detect tries to connect to local garden, and returns success
func (c *CFCollector) Detect() error {
	cfu, err := cloudfoundry.GetGardenUtil()
	if err != nil {
		return err
	}

	c.gardenUtil = cfu
	return nil
}

// List gets all running containers
func (c *CFCollector) List() ([]*containers.Container, error) {
	return c.gardenUtil.ListContainers()
}

// UpdateMetrics updates metrics on an existing list of containers
func (c *CFCollector) UpdateMetrics(cList []*containers.Container) error {
	return c.gardenUtil.UpdateContainerMetrics(cList)
}

func cfFactory() Collector {
	return &CFCollector{}
}

func init() {
	registerCollector(cfCollectorName, cfFactory, NodeRuntime)
}
