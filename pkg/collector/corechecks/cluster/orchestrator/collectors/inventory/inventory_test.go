// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package inventory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
)

// TestManifestCollectorInventoryCoversCollectorInventory checks that the manifest inventory has a collector for each
// collector of the structured metadata pipeline, selected the same way.
func TestManifestCollectorInventoryCoversCollectorInventory(t *testing.T) {
	cfg := mockconfig.New(t)
	metadataCollectors := allCollectors(NewCollectorInventory(cfg, nil, nil))
	manifestCollectors := allCollectors(NewManifestCollectorInventory(cfg, nil, nil))
	require.Len(t, manifestCollectors, len(metadataCollectors))

	for name, metadataCollector := range metadataCollectors {
		manifestCollector, found := manifestCollectors[name]
		if !assert.True(t, found, "no manifest collector for %s", name) {
			continue
		}

		expected := *metadataCollector.Metadata()
		actual := *manifestCollector.Metadata()
		assert.Equal(t, selectionMetadata(expected), selectionMetadata(actual), name)
		assert.False(t, actual.IsMetadataProducer, name)
		assert.True(t, actual.IsManifestProducer, name)
		assert.True(t, actual.SupportsManifestBuffering, name)
	}
}

func allCollectors(inventory *CollectorInventory) map[string]collectors.K8sCollector {
	all := map[string]collectors.K8sCollector{}
	for _, versions := range inventory.collectors {
		for _, collector := range versions.Collectors {
			all[collector.Metadata().FullName()] = collector
		}
	}
	return all
}

// selectionMetadata returns the metadata fields that identify and select a collector, and shape its manifests.
func selectionMetadata(metadata collectors.CollectorMetadata) collectors.CollectorMetadata {
	return collectors.CollectorMetadata{
		IsDefaultVersion:                     metadata.IsDefaultVersion,
		IsStable:                             metadata.IsStable,
		Name:                                 metadata.Name,
		NodeType:                             metadata.NodeType,
		Kind:                                 metadata.Kind,
		Group:                                metadata.Group,
		Version:                              metadata.Version,
		SupportsTerminatedResourceCollection: metadata.SupportsTerminatedResourceCollection,
	}
}
