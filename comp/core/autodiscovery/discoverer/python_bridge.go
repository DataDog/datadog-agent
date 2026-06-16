// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package discoverer

import "github.com/DataDog/datadog-agent/pkg/collector/python"

type pythonBridge struct{}

// NewPythonBridge returns a ConfigDiscoverer backed by the Python rtloader.
func NewPythonBridge() ConfigDiscoverer {
	return &pythonBridge{}
}

func (b *pythonBridge) DiscoverConfig(integrationName, serviceJSON string) (string, error) {
	return python.DiscoverConfig(integrationName, serviceJSON)
}
