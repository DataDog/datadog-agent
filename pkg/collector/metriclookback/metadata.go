// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metriclookback contains helpers for 1Hz check metric lookback.
package metriclookback

import (
	yaml "go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

const (
	// ExecutionMetadataKey is the internal instance-config namespace used for
	// Datadog-owned execution metadata.
	ExecutionMetadataKey = "_datadog"
	// ExecutionModeKey identifies the execution mode under ExecutionMetadataKey.
	ExecutionModeKey = "execution_mode"
	// ShadowExecutionMode marks a copied check instance as a shadow execution.
	ShadowExecutionMode = "shadow"
)

// WithShadowExecutionMode returns a copied instance config marked for shadow
// execution without mutating the source instance bytes.
func WithShadowExecutionMode(instance integration.Data) (integration.Data, error) {
	rawConfig := integration.RawMap{}
	if err := yaml.Unmarshal(cloneData(instance), &rawConfig); err != nil {
		return nil, err
	}
	if rawConfig == nil {
		rawConfig = integration.RawMap{}
	}

	var metadata map[interface{}]interface{}
	switch typedMetadata := rawConfig[ExecutionMetadataKey].(type) {
	case integration.RawMap:
		metadata = typedMetadata
	case map[interface{}]interface{}:
		metadata = typedMetadata
	default:
		metadata = map[interface{}]interface{}{}
	}
	metadata[ExecutionModeKey] = ShadowExecutionMode
	rawConfig[ExecutionMetadataKey] = metadata

	out, err := yaml.Marshal(&rawConfig)
	if err != nil {
		return nil, err
	}
	return integration.Data(out), nil
}

func cloneData(data integration.Data) integration.Data {
	return append(integration.Data(nil), data...)
}
