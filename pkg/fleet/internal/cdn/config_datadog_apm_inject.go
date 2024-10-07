// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	configInjectorYAML = "apm-inject.yaml"
)

// injectorConfig represents the injector configuration from the CDN.
type injectorConfig struct {
	version string
	config  []byte
}

// injectorConfigLayer is a config layer that can be merged with other layers into a config.
type injectorConfigLayer struct {
	ID             string                 `json:"name"`
	InjectorConfig map[string]interface{} `json:"injector"`
}

// Version returns the version (hash) of the agent configuration.
func (i *injectorConfig) Version() string {
	return i.version
}

func newInjectorConfig(configOrder *orderConfig, rawLayers ...[]byte) (*injectorConfig, error) {
	if configOrder == nil {
		return nil, fmt.Errorf("order config is nil")
	}

	// Unmarshal layers
	layers := map[string]*injectorConfigLayer{}
	for _, rawLayer := range rawLayers {
		layer := &injectorConfigLayer{}
		if err := json.Unmarshal(rawLayer, layer); err != nil {
			log.Warnf("Failed to unmarshal layer: %v", err)
			continue
		}
		if layer.InjectorConfig != nil {
			// Only add layers that have at least one config that matches the agent
			layers[layer.ID] = layer
		}
	}

	// Compile ordered layers into a single config
	compiledLayer := &injectorConfigLayer{
		InjectorConfig: map[string]interface{}{},
	}
	for i := len(configOrder.Order) - 1; i >= 0; i-- {
		layerID := configOrder.Order[i]
		layer, ok := layers[layerID]
		if !ok {
			continue
		}

		if layer.InjectorConfig != nil {
			agentConfig, err := merge(compiledLayer.InjectorConfig, layer.InjectorConfig)
			if err != nil {
				return nil, err
			}
			compiledLayer.InjectorConfig = agentConfig.(map[string]interface{})
		}
	}

	// Marshal into YAML configs
	config, err := marshalAgentConfig(compiledLayer.InjectorConfig) // TODO: do we need to marshal in yaml?
	if err != nil {
		return nil, err
	}

	hash := sha256.New()
	version, err := json.Marshal(compiledLayer)
	if err != nil {
		return nil, err
	}
	hash.Write(version)

	return &injectorConfig{
		version: fmt.Sprintf("%x", hash.Sum(nil)),
		config:  config,
	}, nil
}

// Write writes the agent configuration to the given directory.
func (i *injectorConfig) Write(dir string) error {
	if i.config != nil {
		err := os.WriteFile(filepath.Join(dir, configInjectorYAML), []byte(i.config), 0644) // Must be world readable
		if err != nil {
			return fmt.Errorf("could not write datadog.yaml: %w", err)
		}
	}
	return nil
}
