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
)

const (
	apmLibrariesConfigPath = "libraries_config.yaml"
)

// apmLibrariesConfig represents the injector configuration from the CDN.
type apmLibrariesConfig struct {
	version   string
	policyIDs []string

	apmLibrariesConfig []byte
}

// apmLibrariesConfigLayer is a config layer that can be merged with other layers into a config.
type apmLibrariesConfigLayer struct {
	ID                 string                 `json:"name"`
	APMLibrariesConfig map[string]interface{} `json:"apm_libraries_config"`
}

// State returns the APM configs state
func (i *apmLibrariesConfig) State() *pbgo.PoliciesState {
	return &pbgo.PoliciesState{
		MatchedPolicies: i.policyIDs,
		Version:         i.version,
	}
}

func newAPMLibrariesConfig(hostTags []string, orderedLayers ...[]byte) (*apmLibrariesConfig, error) {
	// Compile ordered layers into a single config
	policyIDs := []string{}
	compiledLayer := &apmLibrariesConfigLayer{
		APMLibrariesConfig: map[string]interface{}{},
	}
	for _, rawLayer := range orderedLayers {
		layer := &apmLibrariesConfigLayer{}
		if err := json.Unmarshal(rawLayer, layer); err != nil {
			log.Warnf("Failed to unmarshal layer: %v", err)
			continue
		}

		if layer.APMLibrariesConfig != nil {
			cfg, err := merge(compiledLayer.APMLibrariesConfig, layer.APMLibrariesConfig)
			if err != nil {
				return nil, err
			}
			compiledLayer.APMLibrariesConfig = cfg.(map[string]interface{})
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
	compiledLayer.APMLibrariesConfig["host_tags"] = hostTags

	// Marshal into msgpack configs
	yamlCfg, err := marshalYAMLConfig(compiledLayer.APMLibrariesConfig)
	if err != nil {
		return nil, err
	}

	return &apmLibrariesConfig{
		version:   fmt.Sprintf("%x", hash.Sum(nil)),
		policyIDs: policyIDs,

		apmLibrariesConfig: yamlCfg,
	}, nil
}

// Write writes the agent configuration to the given directory.
func (i *apmLibrariesConfig) Write(dir string) error {
	if i.apmLibrariesConfig != nil {
		err := os.WriteFile(filepath.Join(dir, apmLibrariesConfigPath), []byte(i.apmLibrariesConfig), 0644) // Must be world readable
		if err != nil {
			return fmt.Errorf("could not write %s: %w", apmLibrariesConfigPath, err)
		}
	}
	return writePolicyMetadata(i, dir)
}
