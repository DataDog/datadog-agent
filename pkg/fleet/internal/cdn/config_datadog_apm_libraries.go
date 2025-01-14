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
)

const (
	apmLibrariesConfigPath = "libraries_config.yaml"
)

// apmLibrariesConfig represents the injector configuration from the CDN.
// The contents are YAML and are ready to be written on disk.
type apmLibrariesConfig struct {
	version string
	policy  string

	apmLibrariesConfig []byte
}

// apmLibrariesConfigRaw represents the raw injector configuration from the CDN.
type apmLibrariesConfigRaw struct {
	ID                 string                 `json:"name"`
	APMLibrariesConfig map[string]interface{} `json:"apm_libraries_config"`
}

// State returns the APM configs state
func (i *apmLibrariesConfig) State() *pbgo.PoliciesState {
	return &pbgo.PoliciesState{
		MatchedPolicies: []string{i.policy},
		Version:         i.version,
	}
}

func newAPMLibrariesConfig(hostTags []string, rawConfig []byte) (*apmLibrariesConfig, error) {
	if len(rawConfig) == 0 {
		return &apmLibrariesConfig{
			version: fmt.Sprintf("%x", sha256.Sum256([]byte{})),
			policy:  "default",
		}, nil
	}
	var config configLayer
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return nil, err
	}
	apmLibsConfig := &apmLibrariesConfigRaw{}
	if err := json.Unmarshal(config.Configs, apmLibsConfig); err != nil {
		return nil, err
	}
	if len(apmLibsConfig.APMLibrariesConfig) == 0 {
		return &apmLibrariesConfig{
			version: fmt.Sprintf("%x", sha256.Sum256([]byte{})),
			policy:  "default",
		}, nil
	}

	apmLibsConfig.APMLibrariesConfig["host_tags"] = hostTags

	// Marshal into YAML configs
	yamlCfg, err := marshalYAMLConfig(apmLibsConfig.APMLibrariesConfig)
	if err != nil {
		return nil, err
	}

	hash := sha256.New()
	version, err := json.Marshal(apmLibsConfig)
	if err != nil {
		return nil, err
	}
	hash.Write(version)

	return &apmLibrariesConfig{
		version: fmt.Sprintf("%x", hash.Sum(nil)),
		policy:  config.ID,

		apmLibrariesConfig: yamlCfg,
	}, nil
}

// Write writes the agent configuration to the given directory.
func (i *apmLibrariesConfig) Write(dir string) error {
	if i == nil {
		return nil
	}
	if i.apmLibrariesConfig != nil {
		err := os.WriteFile(filepath.Join(dir, apmLibrariesConfigPath), []byte(i.apmLibrariesConfig), 0644) // Must be world readable
		if err != nil {
			return fmt.Errorf("could not write %s: %w", apmLibrariesConfigPath, err)
		}
	}
	return writePolicyMetadata(i, dir)
}
