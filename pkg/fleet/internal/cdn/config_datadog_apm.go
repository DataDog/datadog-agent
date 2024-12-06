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

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	injectorConfigFilename = "injector.msgpack"
)

// apmConfig represents the injector configuration from the CDN.
type apmConfig struct {
	version   string
	policyIDs []string

	injectorConfig []byte
}

// apmConfigLayer is a config layer that can be merged with other layers into a config.
type apmConfigLayer struct {
	ID             string                 `json:"name"`
	InjectorConfig map[string]interface{} `json:"apm_ssi_config"`
}

// State returns the APM configs state
func (i *apmConfig) State() *pbgo.PoliciesState {
	return &pbgo.PoliciesState{
		MatchedPolicies: i.policyIDs,
		Version:         i.version,
	}
}

func newAPMConfig(hostTags []string, orderedLayers ...[]byte) (*apmConfig, error) {
	// Compile ordered layers into a single config
	// TODO: maybe we don't want that and we should reject if there are more than one config?
	policyIDs := []string{}
	compiledLayer := &apmConfigLayer{
		InjectorConfig: map[string]interface{}{},
	}
	for _, rawLayer := range orderedLayers {
		layer := &apmConfigLayer{}
		if err := json.Unmarshal(rawLayer, layer); err != nil {
			log.Warnf("Failed to unmarshal layer: %v", err)
			continue
		}

		// Only add layers that match the injector
		if layer.InjectorConfig != nil {
			injectorConfig, err := merge(compiledLayer.InjectorConfig, layer.InjectorConfig)
			if err != nil {
				return nil, err
			}
			compiledLayer.InjectorConfig = injectorConfig.(map[string]interface{})
			policyIDs = append(policyIDs, layer.ID)
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
		version:   fmt.Sprintf("%x", hash.Sum(nil)),
		policyIDs: policyIDs,

		injectorConfig: injectorConfig,
	}, nil
}

// Write writes the agent configuration to the given directory.
func (i *apmConfig) Write(dir string) error {
	if i.injectorConfig != nil {
		err := os.WriteFile(filepath.Join(dir, injectorConfigFilename), []byte(i.injectorConfig), 0644) // Must be world readable
		if err != nil {
			return fmt.Errorf("could not write %s: %w", injectorConfigFilename, err)
		}
	}
	return writePolicyMetadata(i, dir)
}
