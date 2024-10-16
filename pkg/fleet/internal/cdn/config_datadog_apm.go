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
	"github.com/vmihailenco/msgpack/v5"
)

const (
	injectorConfigFilename = "injector.msgpack"
)

// apmConfig represents the injector configuration from the CDN.
type apmConfig struct {
	version        string
	injectorConfig []byte
}

// apmConfigLayer is a config layer that can be merged with other layers into a config.
type apmConfigLayer struct {
	ID             string                 `json:"name"`
	InjectorConfig map[string]interface{} `json:"apm_ssi_config"`
}

// Version returns the version (hash) of the agent configuration.
func (i *apmConfig) Version() string {
	return i.version
}

func newAPMConfig(hostTags []string, configOrder *orderConfig, rawLayers ...[]byte) (*apmConfig, error) {
	if configOrder == nil {
		return nil, fmt.Errorf("order config is nil")
	}

	// Unmarshal layers
	layers := map[string]*apmConfigLayer{}
	for _, rawLayer := range rawLayers {
		layer := &apmConfigLayer{}
		if err := json.Unmarshal(rawLayer, layer); err != nil {
			log.Warnf("Failed to unmarshal layer: %v", err)
			continue
		}

		if layer.InjectorConfig != nil {
			// Only add layers that have at least one config that matches
			layers[layer.ID] = layer
		}
	}

	// Compile ordered layers into a single config
	// TODO: maybe we don't want that and we should reject if there are more than one config?
	compiledLayer := &apmConfigLayer{
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

	hash := sha256.New()
	version, err := json.Marshal(compiledLayer)
	if err != nil {
		return nil, err
	}
	hash.Write(version)

	// Add host tags AFTER compiling the version -- we don't want to trigger noop updates
	compiledLayer.InjectorConfig["host_tags"] = hostTags

	// Marshal into msgpack configs
	injectorConfig, err := msgpack.Marshal(compiledLayer.InjectorConfig)
	if err != nil {
		return nil, err
	}

	return &apmConfig{
		version:        fmt.Sprintf("%x", hash.Sum(nil)),
		injectorConfig: injectorConfig,
	}, nil
}

// Write writes the agent configuration to the given directory.
func (i *apmConfig) Write(dir string) error {
	if i.injectorConfig != nil {
		err := os.WriteFile(filepath.Join(dir, injectorConfigFilename), []byte(i.injectorConfig), 0644) // Must be world readable
		if err != nil {
			return fmt.Errorf("could not write datadog.yaml: %w", err)
		}
	}
	return nil
}
