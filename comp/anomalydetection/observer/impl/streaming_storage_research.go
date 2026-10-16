// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build anomalydetectiontestbench

package observerimpl

// TestbenchStreamingStorageConfig uses the live Agent's detector-derived
// retention policy. This research harness helper does not change detectors.
func TestbenchStreamingStorageConfig(settings ComponentSettings) StorageConfig {
	detectors, _, _, _, _ := defaultCatalog().Instantiate(settings)
	return storageConfigFromAgentConfig(nil, detectors)
}
